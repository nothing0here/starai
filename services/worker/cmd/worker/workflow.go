package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "golang.org/x/image/webp"
)

type workflowNode struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	ModelCode      string  `json:"model_code"`
	PromptTemplate string  `json:"prompt_template"`
	Cost           float64 `json:"cost"`
}

func processWorkflowTask(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload) error {
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lockConn.Release()
	var locked bool
	if err := lockConn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, p.ProjectID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		log.Printf("Workflow project %d is already being processed; duplicate delivery ignored", p.ProjectID)
		return nil
	}
	defer func() { _, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, p.ProjectID) }()

	var workflowID int64
	var inputsRaw []byte
	var estimated float64
	var publicID string
	var projectStatus string
	err = pool.QueryRow(ctx,
		`SELECT workflow_id, inputs, estimated_cost, public_id, status FROM workflow_projects WHERE id=$1`,
		p.ProjectID).Scan(&workflowID, &inputsRaw, &estimated, &publicID, &projectStatus)
	if err != nil {
		return err
	}
	if projectStatus != "pending" && projectStatus != "running" {
		log.Printf("Workflow project %s has terminal/non-runnable status %s; delivery ignored", publicID, projectStatus)
		return nil
	}
	if projectStatus == "pending" {
		tag, claimErr := pool.Exec(ctx, `UPDATE workflow_projects SET status='running', started_at=COALESCE(started_at,now()), updated_at=now() WHERE id=$1 AND status='pending'`, p.ProjectID)
		if claimErr != nil {
			return claimErr
		}
		if tag.RowsAffected() == 0 {
			log.Printf("Workflow project %s changed state before claim; delivery ignored", publicID)
			return nil
		}
	}

	var nodesRaw, runtimeRaw []byte
	var category string
	if err := pool.QueryRow(ctx, `SELECT nodes, category, runtime_config FROM workflow_definitions WHERE id=$1`, workflowID).
		Scan(&nodesRaw, &category, &runtimeRaw); err != nil {
		return failWorkflow(ctx, pool, p, publicID, estimated, "工作流定义缺失")
	}

	var inputs map[string]interface{}
	_ = json.Unmarshal(inputsRaw, &inputs)
	if inputs == nil {
		inputs = map[string]interface{}{}
	}

	runtimeCfg := map[string]interface{}{}
	_ = json.Unmarshal(runtimeRaw, &runtimeCfg)
	if stringAny(runtimeCfg["agent_mode"]) == "comic_drama" {
		return processComicDramaWorkflow(ctx, pool, baseURL, token, p, publicID, workflowID, category, estimated, inputs, runtimeCfg)
	}
	if stringAny(runtimeCfg["agent_mode"]) == "video_upscale" {
		return processVideoUpscaleWorkflow(ctx, pool, baseURL, token, p, publicID, estimated, inputs, runtimeCfg)
	}
	if stringAny(runtimeCfg["agent_mode"]) == "video_redraw" {
		return processVideoRedrawWorkflow(ctx, pool, baseURL, token, p, publicID, estimated, inputs, runtimeCfg)
	}
	if stringAny(runtimeCfg["agent_mode"]) == "subtitle_remove" {
		return processSubtitleRemovalWorkflow(ctx, pool, baseURL, token, p, publicID, estimated, inputs, runtimeCfg)
	}
	if stringAny(runtimeCfg["agent_mode"]) == "simple_pipeline" {
		return processSimpleAgentWorkflow(ctx, pool, baseURL, token, p, publicID, workflowID, category, estimated, inputs, runtimeCfg)
	}

	var nodes []workflowNode
	_ = json.Unmarshal(nodesRaw, &nodes)
	return processCustomWorkflow(ctx, pool, baseURL, token, p, publicID, category, estimated, inputs, nodes)
}

func processVideoRedrawWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, estimated float64, inputs, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	if _, done := outputs["media_tasks"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}
	sourceVideo := firstNonEmpty(firstWorkerURL(inputs["video_url"]), firstWorkerURL(inputs["source_video_url"]), firstWorkerURL(inputs["reference_videos"]))
	if sourceVideo == "" {
		return failWorkflow(ctx, pool, p, publicID, estimated, "请先上传或从资产库选择源视频")
	}
	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "redraw", "一键转绘", "video", map[string]interface{}{
		"source_video_url": sourceVideo,
		"model_code":       stringAny(runtimeCfg["generation_model_code"]),
	}, 0)
	start := time.Now()
	taskInputs := copyMap(inputs)
	taskInputs["count"] = 1
	taskInputs["n"] = 1
	taskInputs["video_url"] = sourceVideo
	taskInputs["source_video_url"] = sourceVideo
	taskInputs["reference_videos"] = []string{sourceVideo}
	taskInputs["operation"] = firstNonEmpty(stringAny(runtimeCfg["redraw_operation"]), "video_redraw")
	taskInputs["style_strength"] = clampFloat(firstPositiveFloat(floatAny(inputs["style_strength"]), floatAny(runtimeCfg["default_style_strength"]), 0.65), 0.05, 1)
	taskInputs["preserve_motion"] = boolDefault(inputs["preserve_motion"], boolDefault(runtimeCfg["preserve_motion"], true))
	taskInputs["preserve_identity"] = boolDefault(inputs["preserve_identity"], boolDefault(runtimeCfg["preserve_identity"], true))
	taskInputs["preserve_audio"] = boolDefault(inputs["preserve_audio"], boolDefault(runtimeCfg["preserve_audio"], true))
	prompt := joinWorkflowInstruction(
		firstNonEmpty(
			stringAny(runtimeCfg["redraw_prompt"]),
			"Redraw the source video in the requested visual style. Preserve timing, camera motion, action continuity, composition and character identity. Avoid flicker, frame inconsistency, warped faces, extra limbs, subtitles and watermarks.",
		),
		stringAny(inputs["prompt"]),
	)
	videoRuntime := copyMap(runtimeCfg)
	videoRuntime["generation_type"] = "video"
	mediaTasks, errMsg := runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, videoRuntime, taskInputs, prompt)
	return finishVideoTransformWorkflow(ctx, pool, p, publicID, estimated, outputs, nodeRunID, start, mediaTasks, errMsg, map[string]interface{}{
		"source_video_url": sourceVideo,
		"style_strength":   taskInputs["style_strength"],
	}, "视频转绘失败：")
}

func processSubtitleRemovalWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, estimated float64, inputs, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	if _, done := outputs["media_tasks"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}
	sourceVideo := firstNonEmpty(firstWorkerURL(inputs["video_url"]), firstWorkerURL(inputs["source_video_url"]), firstWorkerURL(inputs["reference_videos"]))
	if sourceVideo == "" {
		return failWorkflow(ctx, pool, p, publicID, estimated, "请先上传或从资产库选择源视频")
	}
	removeMode := firstNonEmpty(stringAny(inputs["subtitle_mode"]), stringAny(runtimeCfg["default_subtitle_mode"]), "auto")
	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "remove_subtitle", "一键去字幕", "video", map[string]interface{}{
		"source_video_url": sourceVideo,
		"subtitle_mode":    removeMode,
	}, 0)
	start := time.Now()

	if removeMode == "auto" || removeMode == "soft_track" {
		localTask, hadTrack, localErr := removeEmbeddedSubtitleTracks(ctx, pool, p.ProjectID, p.UserID, publicID, sourceVideo)
		if localErr == nil && hadTrack {
			return finishVideoTransformWorkflow(ctx, pool, p, publicID, estimated, outputs, nodeRunID, start, []map[string]interface{}{localTask}, "", map[string]interface{}{
				"source_video_url":   sourceVideo,
				"subtitle_mode":      "soft_track",
				"had_subtitle_track": hadTrack,
			}, "")
		}
		if removeMode == "soft_track" {
			message := "未检测到可移除的独立字幕轨"
			if localErr != nil {
				message = localErr.Error()
			}
			return finishVideoTransformWorkflow(ctx, pool, p, publicID, estimated, outputs, nodeRunID, start, nil, message, map[string]interface{}{"source_video_url": sourceVideo}, "字幕轨移除失败：")
		}
	}

	if stringAny(runtimeCfg["generation_model_code"]) == "" {
		return finishVideoTransformWorkflow(ctx, pool, p, publicID, estimated, outputs, nodeRunID, start, nil, "未检测到可移除的独立字幕轨，且后台未配置硬字幕 AI 修复模型", map[string]interface{}{"source_video_url": sourceVideo}, "")
	}
	taskInputs := copyMap(inputs)
	taskInputs["count"] = 1
	taskInputs["n"] = 1
	taskInputs["video_url"] = sourceVideo
	taskInputs["source_video_url"] = sourceVideo
	taskInputs["reference_videos"] = []string{sourceVideo}
	taskInputs["operation"] = firstNonEmpty(stringAny(runtimeCfg["subtitle_remove_operation"]), "subtitle_remove")
	taskInputs["subtitle_region"] = firstNonEmpty(stringAny(inputs["subtitle_region"]), stringAny(runtimeCfg["default_subtitle_region"]), "bottom_25")
	taskInputs["protect_watermark"] = boolDefault(inputs["protect_watermark"], boolDefault(runtimeCfg["protect_watermark"], true))
	taskInputs["preserve_audio"] = true
	prompt := joinWorkflowInstruction(
		firstNonEmpty(
			stringAny(runtimeCfg["subtitle_remove_prompt"]),
			"Remove only the burned-in subtitles from the specified area of the source video. Reconstruct the background naturally across every frame, preserve people, objects, logos, watermarks outside the subtitle area, motion, timing and original audio, and avoid blur, flicker or ghosting.",
		),
		stringAny(inputs["prompt"]),
	)
	videoRuntime := copyMap(runtimeCfg)
	videoRuntime["generation_type"] = "video"
	mediaTasks, errMsg := runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, videoRuntime, taskInputs, prompt)
	return finishVideoTransformWorkflow(ctx, pool, p, publicID, estimated, outputs, nodeRunID, start, mediaTasks, errMsg, map[string]interface{}{
		"source_video_url": sourceVideo,
		"subtitle_mode":    "hardcoded_ai",
		"subtitle_region":  taskInputs["subtitle_region"],
	}, "硬字幕 AI 修复失败：")
}

func finishVideoTransformWorkflow(ctx context.Context, pool *pgxpool.Pool, p WorkflowTaskPayload, publicID string, estimated float64, outputs map[string]interface{}, nodeRunID int64, started time.Time, mediaTasks []map[string]interface{}, errMsg string, metadata map[string]interface{}, errPrefix string) error {
	duration := int(time.Since(started).Milliseconds())
	actual := sumAgentMediaTaskCost(mediaTasks)
	out := map[string]interface{}{"media_tasks": mediaTasks, "cost": actual}
	for key, value := range metadata {
		out[key] = value
		outputs[key] = value
	}
	if errMsg != "" {
		outputs["current_step"] = "failed"
		outputs["last_error"] = errMsg
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
		pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, error=$2, duration_ms=$3 WHERE id=$4`, mustJSON(out), errMsg, duration, nodeRunID)
		return failWorkflow(ctx, pool, p, publicID, estimated, errPrefix+errMsg)
	}
	delete(outputs, "last_error")
	outputs["media_tasks"] = mediaTasks
	outputs["current_step"] = "result"
	saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	updateNodeRunSuccess(ctx, pool, nodeRunID, out, actual, duration)
	return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func joinWorkflowInstruction(systemInstruction, userInstruction string) string {
	systemInstruction = strings.TrimSpace(systemInstruction)
	userInstruction = strings.TrimSpace(userInstruction)
	if userInstruction == "" {
		return systemInstruction
	}
	if systemInstruction == "" {
		return userInstruction
	}
	return systemInstruction + "\n\nUser requirements:\n" + userInstruction
}

func processVideoUpscaleWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, estimated float64, inputs, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	if _, done := outputs["media_tasks"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}
	sourceVideo := firstWorkerURL(inputs["video_url"])
	if sourceVideo == "" {
		sourceVideo = firstWorkerURL(inputs["source_video_url"])
	}
	if sourceVideo == "" {
		sourceVideo = firstWorkerURL(inputs["reference_videos"])
	}
	if sourceVideo == "" {
		return failWorkflow(ctx, pool, p, publicID, estimated, "请先上传或从资产库选择源视频")
	}
	targetResolution := normalizeUpscaleResolution(firstNonEmpty(stringAny(inputs["target_resolution"]), stringAny(runtimeCfg["default_target_resolution"])))
	if targetResolution == "" || !upscaleResolutionAllowed(targetResolution, runtimeCfg["supported_resolutions"]) {
		return failWorkflow(ctx, pool, p, publicID, estimated, "目标清晰度不受支持")
	}

	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "upscale", "AI 视频高清", "video", map[string]interface{}{
		"source_video_url":  sourceVideo,
		"target_resolution": targetResolution,
		"model_code":        stringAny(runtimeCfg["generation_model_code"]),
	}, 0)
	start := time.Now()
	taskInputs := copyMap(inputs)
	taskInputs["count"] = 1
	taskInputs["n"] = 1
	taskInputs["video_url"] = sourceVideo
	taskInputs["source_video_url"] = sourceVideo
	taskInputs["reference_videos"] = []string{sourceVideo}
	taskInputs["target_resolution"] = targetResolution
	taskInputs["resolution"] = targetResolution
	taskInputs["operation"] = firstNonEmpty(stringAny(runtimeCfg["upscale_operation"]), "upscale")
	taskInputs["preserve_audio"] = boolDefault(taskInputs["preserve_audio"], boolDefault(runtimeCfg["preserve_audio"], true))
	taskInputs["enhancement_mode"] = firstNonEmpty(stringAny(taskInputs["enhancement_mode"]), stringAny(runtimeCfg["default_enhancement_mode"]), "balanced")
	prompt := firstNonEmpty(
		stringAny(inputs["prompt"]),
		stringAny(runtimeCfg["upscale_prompt"]),
		"Enhance the source video to the requested resolution. Preserve the original content, timing, composition, identity, motion and audio. Reduce compression artifacts and noise, recover natural detail, and avoid changing the scene.",
	)
	videoRuntime := copyMap(runtimeCfg)
	videoRuntime["generation_type"] = "video"
	mediaTasks, errMsg := runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, videoRuntime, taskInputs, prompt)
	duration := int(time.Since(start).Milliseconds())
	actual := sumAgentMediaTaskCost(mediaTasks)
	out := map[string]interface{}{
		"media_tasks":       mediaTasks,
		"source_video_url":  sourceVideo,
		"target_resolution": targetResolution,
		"cost":              actual,
	}
	outputs["media_tasks"] = mediaTasks
	outputs["source_video_url"] = sourceVideo
	outputs["target_resolution"] = targetResolution
	outputs["current_step"] = "result"
	saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	if errMsg != "" {
		pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, error=$2, duration_ms=$3 WHERE id=$4`, mustJSON(out), errMsg, duration, nodeRunID)
		return failWorkflow(ctx, pool, p, publicID, estimated, "视频高清处理失败："+errMsg)
	}
	updateNodeRunSuccess(ctx, pool, nodeRunID, out, actual, duration)
	return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
}

func firstWorkerURL(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		if len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
	case []interface{}:
		if len(v) > 0 {
			return firstWorkerURL(v[0])
		}
	case map[string]interface{}:
		return firstNonEmpty(stringAny(v["url"]), stringAny(v["video_url"]))
	}
	return ""
}

func normalizeUpscaleResolution(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "720P", "1280X720":
		return "720P"
	case "1K", "1080P", "1920X1080":
		return "1K"
	case "2K", "1440P", "2560X1440":
		return "2K"
	default:
		return ""
	}
}

func upscaleResolutionAllowed(value string, raw interface{}) bool {
	allowed := map[string]bool{}
	switch items := raw.(type) {
	case []interface{}:
		for _, item := range items {
			if normalized := normalizeUpscaleResolution(stringAny(item)); normalized != "" {
				allowed[normalized] = true
			}
		}
	case []string:
		for _, item := range items {
			if normalized := normalizeUpscaleResolution(item); normalized != "" {
				allowed[normalized] = true
			}
		}
	}
	if len(allowed) == 0 {
		return value == "720P" || value == "1K" || value == "2K"
	}
	return allowed[value]
}

func boolDefault(value interface{}, fallback bool) bool {
	if value == nil {
		return fallback
	}
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fallback
}

func removeEmbeddedSubtitleTracks(ctx context.Context, pool *pgxpool.Pool, projectID, userID int64, publicID, sourceURL string) (map[string]interface{}, bool, error) {
	if objectStore == nil {
		return nil, false, errors.New("对象存储未配置，无法保存去字幕结果")
	}
	if _, err := ffmpegBinaryPath(); err != nil {
		return nil, false, err
	}
	data, _, err := downloadAuthenticatedMedia(ctx, connectionConfig{}, sourceURL, 1024<<20)
	if err != nil {
		return nil, false, fmt.Errorf("下载源视频失败：%w", err)
	}
	tmpDir, err := os.MkdirTemp("", "starai-subtitle-remove-*")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(tmpDir)
	inputPath := filepath.Join(tmpDir, "source.mp4")
	if err := os.WriteFile(inputPath, data, 0600); err != nil {
		return nil, false, err
	}
	hadTrack, probeErr := mediaHasSubtitle(ctx, inputPath)
	if probeErr != nil {
		return nil, false, fmt.Errorf("检测字幕轨失败：%w", probeErr)
	}
	if !hadTrack {
		return nil, false, nil
	}
	taskNo := newWorkflowTaskNo(0)
	taskInput := map[string]interface{}{"video_url": sourceURL, "operation": "remove_subtitle_track", "_skip_billing": true, "_workflow_project": publicID}
	inputJSON, _ := json.Marshal(taskInput)
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (task_no,user_id,model_id,type,status,input,estimated_cost,started_at) VALUES ($1,$2,NULL,'video','running',$3,0,now())`, taskNo, userID, inputJSON); err != nil {
		return nil, hadTrack, err
	}
	appendWorkflowMediaTask(ctx, pool, projectID, map[string]interface{}{"task_no": taskNo, "type": "video", "status": "running", "progress": 20, "output": map[string]interface{}{}})
	outputPath := filepath.Join(tmpDir, "result.mp4")
	if err := runFFmpeg(ctx, "-y", "-i", inputPath, "-map", "0:v?", "-map", "0:a?", "-map", "0:d?", "-c", "copy", "-sn", "-movflags", "+faststart", outputPath); err != nil {
		pool.Exec(ctx, `UPDATE tasks SET status='failed',error_code='SUBTITLE_REMOVE_FAILED',error_message=$1,finished_at=now(),updated_at=now() WHERE task_no=$2`, err.Error(), taskNo)
		return nil, hadTrack, err
	}
	outputData, err := os.ReadFile(outputPath)
	if err != nil || len(outputData) == 0 {
		pool.Exec(ctx, `UPDATE tasks SET status='failed',error_code='SUBTITLE_REMOVE_FAILED',error_message='读取去字幕结果失败',finished_at=now(),updated_at=now() WHERE task_no=$1`, taskNo)
		return nil, hadTrack, errors.New("读取去字幕结果失败")
	}
	objectName := fmt.Sprintf("works/subtitle-remove/%s/result_%d.mp4", publicID, time.Now().UnixNano())
	publicURL, err := objectStore.Upload(ctx, objectName, "video/mp4", bytes.NewReader(outputData), int64(len(outputData)))
	if err != nil {
		pool.Exec(ctx, `UPDATE tasks SET status='failed',error_code='SUBTITLE_REMOVE_FAILED',error_message=$1,finished_at=now(),updated_at=now() WHERE task_no=$2`, err.Error(), taskNo)
		return nil, hadTrack, fmt.Errorf("上传去字幕结果失败：%w", err)
	}
	output := map[string]interface{}{
		"video_url":          publicURL,
		"url":                publicURL,
		"subtitle_mode":      "soft_track",
		"had_subtitle_track": hadTrack,
	}
	outputJSON, _ := json.Marshal(output)
	var taskID int64
	if err := pool.QueryRow(ctx, `UPDATE tasks SET status='succeeded',output=$1,actual_cost=0,error_code=NULL,error_message=NULL,finished_at=now(),updated_at=now() WHERE task_no=$2 RETURNING id`, outputJSON, taskNo).Scan(&taskID); err != nil {
		return nil, hadTrack, err
	}
	workID := fmt.Sprintf("work_%d", time.Now().UnixNano())
	pool.Exec(ctx, `INSERT INTO works (public_id,user_id,task_id,model_id,type,prompt,thumbnail_url,metadata) VALUES ($1,$2,$3,NULL,'video',$4,$5,$6)`,
		workID, userID, taskID, "移除视频独立字幕轨", publicURL, outputJSON)
	pool.Exec(ctx, `INSERT INTO task_events (task_id,event_type,payload) VALUES ($1,'completed',$2)`, taskID, outputJSON)
	item := map[string]interface{}{"task_no": taskNo, "type": "video", "status": "succeeded", "progress": 100, "output": output, "estimated_cost": 0, "actual_cost": 0}
	appendWorkflowMediaTask(ctx, pool, projectID, item)
	return item, hadTrack, nil
}

func mediaHasSubtitle(ctx context.Context, path string) (bool, error) {
	ffprobePath := "ffprobe"
	if ffmpegPath, err := ffmpegBinaryPath(); err == nil {
		candidate := filepath.Join(filepath.Dir(ffmpegPath), ffprobeExecutableName())
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			ffprobePath = candidate
		}
	}
	cmd := exec.CommandContext(ctx, ffprobePath, "-v", "error", "-select_streams", "s", "-show_entries", "stream=index", "-of", "csv=p=0", path)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func processCustomWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, category string, estimated float64, inputs map[string]interface{}, nodes []workflowNode) error {
	vars := map[string]string{}
	for k, v := range inputs {
		vars[k] = fmt.Sprintf("%v", v)
	}
	pool.Exec(ctx, `UPDATE workflow_projects SET status='running', started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1`, p.ProjectID)

	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	var totalCost float64
	lastText := ""
	for seq, node := range nodes {
		if existing, ok := mapAny(outputs[node.ID]); ok {
			absorbNodeOutputVars(vars, node.ID, existing)
			if s := firstNonEmpty(stringAny(existing["text"]), stringAny(existing["generation_prompt"]), stringAny(existing["summary"]), stringAny(existing["raw_text"])); s != "" {
				lastText = s
			}
			continue
		}
		prompt := renderTemplate(node.PromptTemplate, vars)
		if strings.TrimSpace(prompt) == "" {
			if node.Type == "image" || node.Type == "video" {
				prompt = mediaPromptFallback(lastText, vars, inputs)
			}
		}
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, node.ID, node.Name, node.Type, map[string]interface{}{"prompt": prompt, "model_code": node.ModelCode}, seq)
		start := time.Now()
		out, errMsg := runNode(ctx, pool, baseURL, token, p.UserID, publicID, category, node, prompt, inputs)
		duration := int(time.Since(start).Milliseconds())
		if errMsg != "" {
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', error=$1, duration_ms=$2 WHERE id=$3`, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, fmt.Sprintf("节点「%s」执行失败：%s", node.Name, errMsg))
		}
		absorbNodeOutputVars(vars, node.ID, out)
		lastText = firstNonEmpty(vars[node.ID+"_generation_prompt"], vars["generation_prompt"], vars[node.ID+"_text"], lastText)
		outputs[node.ID] = out
		for k, v := range out {
			outputs[node.ID+"_"+k] = v
		}
		if node.Type == "image" || node.Type == "video" {
			outputs["media_tasks"] = appendMediaTaskOutput(outputs["media_tasks"], out)
		}
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, node.Cost, duration)
		totalCost += node.Cost
		if node.Type == "llm" && stringAny(inputs["_mode"]) != "auto" && !boolAny(outputs["autopilot"]) && stringAny(outputs["confirmed_step"]) == "" {
			outputs["current_step"] = "confirm"
			outputs["autopilot"] = false
			saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
			pool.Exec(ctx, `UPDATE workflow_projects SET status='waiting_confirm', updated_at=now() WHERE id=$1`, p.ProjectID)
			return nil
		}
	}

	totalCost = workflowActualCost(ctx, pool, p.ProjectID, outputs)
	chargeCost := incrementalWorkflowCharge(ctx, pool, p.ProjectID, totalCost)
	if err := chargeBillingWithFinalize(ctx, pool, p.UserID, estimated, chargeCost, "workflow", publicID, "workflow_usage", "智能体工作流", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE workflow_projects SET status='succeeded', outputs=$1, actual_cost=$2, error_message=NULL, finished_at=now(), updated_at=now() WHERE id=$3 AND status='running'`,
			mustJSON(outputs), totalCost, p.ProjectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("workflow is no longer running")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("workflow %s billing/finalize: %w", publicID, err)
	}
	log.Printf("Workflow project %s completed (cost=%.4f)", publicID, totalCost)
	return nil
}

func appendMediaTaskOutput(raw interface{}, out map[string]interface{}) []map[string]interface{} {
	items := []map[string]interface{}{}
	if arr, ok := raw.([]map[string]interface{}); ok {
		items = append(items, arr...)
	} else if arr, ok := raw.([]interface{}); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
	}
	taskNo := stringAny(out["_task_no"])
	if taskNo == "" {
		taskNo = newWorkflowTaskNo(len(items))
	}
	items = append(items, map[string]interface{}{
		"task_no":  taskNo,
		"status":   "succeeded",
		"progress": 100,
		"output":   out,
	})
	return items
}

func absorbNodeOutputVars(vars map[string]string, nodeID string, out map[string]interface{}) {
	if text := stringAny(out["text"]); text != "" {
		vars[nodeID] = text
		vars[nodeID+"_text"] = text
		if parsed := parseJSONish(text); len(parsed) > 0 {
			for k, v := range parsed {
				if s := stringAny(v); s != "" {
					vars[nodeID+"_"+k] = s
					if k == "generation_prompt" {
						vars["generation_prompt"] = s
					}
				}
			}
		}
	}
	for k, v := range out {
		if s := stringAny(v); s != "" {
			vars[nodeID+"_"+k] = s
			if k == "generation_prompt" {
				vars["generation_prompt"] = s
			}
		}
	}
}

func mediaPromptFallback(lastText string, vars map[string]string, inputs map[string]interface{}) string {
	if s := firstNonEmpty(vars["generation_prompt"], vars["analysis_generation_prompt"], lastText, vars["analysis"], vars["analysis_text"], firstUserPrompt(inputs)); s != "" {
		if parsed := parseJSONish(s); len(parsed) > 0 {
			return firstNonEmpty(stringAny(parsed["generation_prompt"]), stringAny(parsed["summary"]), stringAny(parsed["raw_text"]), s)
		}
		return s
	}
	return ""
}

func processSimpleAgentWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, workflowID int64, category string, estimated float64, inputs map[string]interface{}, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	autopilot := boolAny(outputs["autopilot"]) || stringAny(inputs["_mode"]) == "auto"
	pool.Exec(ctx, `UPDATE workflow_projects SET status='running', started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1`, p.ProjectID)

	analysis, ok := mapAny(outputs["analysis"])
	if !ok {
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "analysis", "需求分析", "llm", map[string]interface{}{"inputs": inputs}, 0)
		start := time.Now()
		out, errMsg := runAgentAnalysis(ctx, pool, baseURL, token, stringAny(runtimeCfg["analysis_model_code"]), category, runtimeCfg, inputs)
		duration := int(time.Since(start).Milliseconds())
		if errMsg != "" {
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', error=$1, duration_ms=$2 WHERE id=$3`, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, "需求分析失败："+errMsg)
		}
		analysis = out
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, floatAny(out["_analysis_cost"]), duration)
		outputs["analysis"] = out
		outputs["current_step"] = "confirm"
		outputs["autopilot"] = autopilot
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
		if !autopilot {
			pool.Exec(ctx, `UPDATE workflow_projects SET status='waiting_confirm', updated_at=now() WHERE id=$1`, p.ProjectID)
			return nil
		}
	}

	confirmed := mapAnyOr(outputs["confirmation_payload"], map[string]interface{}{})
	candidateID := stringAny(confirmed["candidate_id"])
	finalPrompt := firstNonEmpty(stringAny(confirmed["prompt"]), stringAny(confirmed["final_prompt"]), selectedAnalysisPrompt(analysis, candidateID), firstUserPrompt(inputs))
	generationInputs := mergeAgentGenerationInputs(inputs, analysis, candidateID, confirmed)
	finalPrompt = agentPromptWithScene(finalPrompt, generationInputs)
	if _, done := outputs["media_tasks"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}

	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "generate", "生成结果", stringAny(runtimeCfg["generation_type"]), map[string]interface{}{"prompt": finalPrompt}, 1)
	start := time.Now()
	var mediaTasks []map[string]interface{}
	var errMsg string
	if stringAny(generationInputs["creative_scene"]) == "detail_image" && stringAny(runtimeCfg["generation_type"]) != "video" {
		var detailPage map[string]interface{}
		mediaTasks, detailPage, errMsg = runAgentDetailPageTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, runtimeCfg, generationInputs, analysis, finalPrompt)
		outputs["detail_page"] = detailPage
	} else {
		mediaTasks, errMsg = runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, runtimeCfg, generationInputs, finalPrompt)
	}
	duration := int(time.Since(start).Milliseconds())
	generationCost := sumAgentMediaTaskCost(mediaTasks)
	out := map[string]interface{}{"media_tasks": mediaTasks, "cost": generationCost}
	outputs["media_tasks"] = mediaTasks
	outputs["current_step"] = "result"
	saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	if errMsg != "" {
		pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, error=$2, duration_ms=$3 WHERE id=$4`, mustJSON(out), errMsg, duration, nodeRunID)
		return failWorkflow(ctx, pool, p, publicID, estimated, errMsg)
	}
	updateNodeRunSuccess(ctx, pool, nodeRunID, out, generationCost, duration)
	return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
}

func processComicDramaWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, workflowID int64, category string, estimated float64, inputs map[string]interface{}, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	autopilot := boolAny(outputs["autopilot"]) || stringAny(inputs["_mode"]) == "auto"
	pool.Exec(ctx, `UPDATE workflow_projects SET status='running', started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1`, p.ProjectID)

	plan, ok := mapAny(outputs["comic_drama"])
	if !ok {
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "comic_plan", "AI漫剧规划", "llm", map[string]interface{}{"inputs": inputs}, 0)
		start := time.Now()
		out, errMsg := runComicDramaPlan(ctx, pool, baseURL, token, p.ProjectID, p.UserID, runtimeCfg, inputs)
		duration := int(time.Since(start).Milliseconds())
		if errMsg != "" {
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', error=$1, duration_ms=$2 WHERE id=$3`, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, "AI漫剧规划失败："+errMsg)
		}
		plan = out
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, floatAny(out["_analysis_cost"]), duration)
		outputs["comic_drama"] = plan
		outputs["analysis"] = map[string]interface{}{
			"summary":           stringAny(plan["intent"]),
			"generation_prompt": stringAny(plan["outline"]),
			"candidates": []map[string]interface{}{
				{"id": "A", "title": "AI漫剧方案", "reason": "根据输入自动生成完整漫剧流程", "prompt": stringAny(plan["outline"])},
			},
			"recommendation": "A",
		}
		outputs["current_step"] = "storyboard_confirm"
		outputs["autopilot"] = autopilot
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
		if !autopilot {
			pool.Exec(ctx, `UPDATE workflow_projects SET status='waiting_confirm', updated_at=now() WHERE id=$1`, p.ProjectID)
			return nil
		}
	}

	if confirmed := mapAnyOr(outputs["confirmation_payload"], map[string]interface{}{}); stringAny(confirmed["prompt"]) != "" {
		plan["outline"] = stringAny(confirmed["prompt"])
		outputs["comic_drama"] = plan
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	}

	if _, done := outputs["final_video_url"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}

	storyboards := comicStoryboards(plan, runtimeCfg)
	if len(storyboards) == 0 {
		return failWorkflow(ctx, pool, p, publicID, estimated, "AI漫剧规划未生成有效分镜")
	}

	var totalCost float64
	keyframes, _ := outputs["keyframes"].([]interface{})
	if !comicStageComplete(keyframes, storyboards, "image_url") {
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "keyframes", "关键帧生成", "image", map[string]interface{}{"storyboard_count": len(storyboards)}, 1)
		start := time.Now()
		items, cost, errMsg := runComicKeyframes(ctx, pool, baseURL, token, p, publicID, runtimeCfg, inputs, storyboards, keyframes)
		duration := int(time.Since(start).Milliseconds())
		totalCost += cost
		out := map[string]interface{}{"keyframes": items, "cost": cost}
		if errMsg != "" {
			keyframes = mapSliceToInterfaces(items)
			outputs["keyframes"] = keyframes
			if comic, ok := mapAny(outputs["comic_drama"]); ok {
				comic["keyframes"] = items
				outputs["comic_drama"] = comic
			}
			outputs["current_step"] = "keyframes"
			saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, cost=$2, error=$3, duration_ms=$4 WHERE id=$5`, mustJSON(out), cost, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, errMsg)
		}
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, cost, duration)
		keyframes = mapSliceToInterfaces(items)
		outputs["keyframes"] = keyframes
		if comic, ok := mapAny(outputs["comic_drama"]); ok {
			comic["keyframes"] = items
			outputs["comic_drama"] = comic
		}
		outputs["current_step"] = "video_segments"
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	}

	segments, _ := outputs["segments"].([]interface{})
	if !comicStageComplete(segments, storyboards, "video_url") {
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "video_segments", "分段视频生成", "video", map[string]interface{}{"storyboard_count": len(storyboards)}, 2)
		start := time.Now()
		items, cost, errMsg := runComicVideoSegments(ctx, pool, baseURL, token, p, publicID, runtimeCfg, inputs, storyboards, keyframes, segments)
		duration := int(time.Since(start).Milliseconds())
		totalCost += cost
		out := map[string]interface{}{"segments": items, "cost": cost}
		if errMsg != "" {
			segments = mapSliceToInterfaces(items)
			outputs["segments"] = segments
			if comic, ok := mapAny(outputs["comic_drama"]); ok {
				comic["segments"] = items
				outputs["comic_drama"] = comic
			}
			outputs["current_step"] = "video_segments"
			saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, cost=$2, error=$3, duration_ms=$4 WHERE id=$5`, mustJSON(out), cost, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, errMsg)
		}
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, cost, duration)
		segments = mapSliceToInterfaces(items)
		outputs["segments"] = segments
		if comic, ok := mapAny(outputs["comic_drama"]); ok {
			comic["segments"] = items
			outputs["comic_drama"] = comic
		}
		outputs["current_step"] = "compose"
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	}

	narrations, _ := outputs["narrations"].([]interface{})
	narrationModelCode := firstNonEmpty(stringAny(inputs["narration_model_code"]), stringAny(runtimeCfg["narration_model_code"]))
	audioStrategy := comicAudioStrategy(inputs, runtimeCfg)
	if narrationModelCode != "" && audioStrategy != "video_native" && !comicNarrationStageComplete(narrations, storyboards) {
		nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "narrations", "对白与旁白配音", "audio", map[string]interface{}{"storyboard_count": len(storyboards)}, 3)
		start := time.Now()
		items, cost, errMsg := runComicNarrations(ctx, pool, baseURL, token, p, publicID, runtimeCfg, inputs, storyboards, narrations)
		duration := int(time.Since(start).Milliseconds())
		totalCost += cost
		out := map[string]interface{}{"narrations": items, "cost": cost}
		if errMsg != "" {
			narrations = mapSliceToInterfaces(items)
			outputs["narrations"] = narrations
			outputs["current_step"] = "narrations"
			saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
			pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', output=$1, cost=$2, error=$3, duration_ms=$4 WHERE id=$5`, mustJSON(out), cost, errMsg, duration, nodeRunID)
			return failWorkflow(ctx, pool, p, publicID, estimated, errMsg)
		}
		updateNodeRunSuccess(ctx, pool, nodeRunID, out, cost, duration)
		narrations = mapSliceToInterfaces(items)
		outputs["narrations"] = narrations
		outputs["current_step"] = "compose"
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	}

	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "compose", "视频合成", "video", map[string]interface{}{"segments": len(segments), "narrations": len(narrations)}, 4)
	start := time.Now()
	final, errMsg := composeComicDramaVideo(ctx, pool, publicID, segments, narrations, inputs, runtimeCfg)
	duration := int(time.Since(start).Milliseconds())
	if errMsg != "" {
		pool.Exec(ctx, `UPDATE workflow_node_runs SET status='failed', error=$1, duration_ms=$2 WHERE id=$3`, errMsg, duration, nodeRunID)
		return failWorkflow(ctx, pool, p, publicID, estimated, errMsg)
	}
	updateNodeRunSuccess(ctx, pool, nodeRunID, final, 0, duration)
	outputs["final_video_url"] = final["final_video_url"]
	outputs["thumbnail"] = final["thumbnail"]
	outputs["current_step"] = "result"
	outputs["media_tasks"] = append(outputsInterfaceSlice(outputs["media_tasks"]), map[string]interface{}{"task_no": "compose_" + publicID, "status": "succeeded", "progress": 100, "output": final})
	if comic, ok := mapAny(outputs["comic_drama"]); ok {
		comic["final_video_url"] = final["final_video_url"]
		comic["thumbnail"] = final["thumbnail"]
		comic["compose_status"] = "succeeded"
		outputs["comic_drama"] = comic
	}
	insertComicDramaWork(ctx, pool, p.UserID, runtimeCfg, inputs, final)
	saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	if totalCost > 0 {
		outputs["_comic_media_cost"] = totalCost
	}
	return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
}

func runComicDramaPlan(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, workflowProjectID, userID int64, runtimeCfg, inputs map[string]interface{}) (map[string]interface{}, string) {
	grid := comicStoryboardGrid(runtimeCfg, inputs)
	durationMode := firstNonEmpty(stringAny(inputs["duration_mode"]), stringAny(runtimeCfg["duration_mode"]), "standard")
	styleMode := firstNonEmpty(stringAny(inputs["style_reference_mode"]), stringAny(runtimeCfg["style_reference_mode"]), "image_reference")
	narrationMode := comicNarrationPerspective(inputs, runtimeCfg)
	narrationInstruction := comicNarrationInstruction(narrationMode)
	system := fmt.Sprintf(`你是 AI 漫剧创作工作流引擎。只输出严格 JSON，不要 Markdown。
目标：把用户创意拆解成可执行的一键 AI 漫剧工作流。
JSON 字段必须包含：
{
  "intent": "一句话目标",
  "creative_direction": "创意方向",
  "outline": "故事大纲",
  "script": "分场剧本",
  "characters": [{"code":"CHAR_01","name":"角色名","description":"外观与性格","visual_prompt":"角色视觉提示词"}],
  "props": [{"code":"PROP_01","name":"道具名","description":"外观与用途","visual_prompt":"道具视觉提示词"}],
  "locations": [{"code":"LOC_01","name":"场景名","description":"空间、时间与光线","visual_prompt":"场景视觉提示词"}],
  "storyboards": [{"id":"S01","title":"分镜标题","duration_sec":5,"character_codes":["CHAR_01"],"prop_codes":["PROP_01"],"location_code":"LOC_01","scene":"画面描述","dialogue":"角色说出的对白，没有则为空","narration":"画外旁白或内心独白，没有则为空","camera":"镜头运动","keyframe_prompt":"关键帧图片提示词","video_prompt":"视频生成提示词"}],
  "keyframes": [],
  "segments": [],
  "current_step": "storyboard_confirm"
}
分镜数量必须为 %d。时长模式：%s。参考图模式：%s。配音叙事模式：%s。
配音规则：%s
dialogue 只能填写画面中角色实际说出的话；narration 只能填写画外旁白或内心独白，二者不要混写。每个角色、道具和场景必须有稳定 code，分镜必须通过 code 引用资产。必须保持角色和画风一致，提示词可以直接传给图片/视频模型。`, grid, durationMode, styleMode, narrationMode, narrationInstruction)
	style := mapAnyOr(inputs["comic_style"], map[string]interface{}{})
	user := fmt.Sprintf("用户需求：%s\n项目说明：%s\n风格名称：%s\n风格提示词：%s\n配音叙事模式：%s\n已锁定角色/道具/场景：%s\n参考图URL：%s\n生成参数：%s", firstUserPrompt(inputs), stringAny(inputs["comic_project_description"]), stringAny(style["name"]), stringAny(style["prompt"]), narrationMode, string(mustJSON(inputs["comic_assets"])), strings.Join(referenceImageURLs(inputs), "\n"), agentGenerationParamSummary(inputs))
	modelCodes := comicDialogueModelCandidates(inputs, runtimeCfg)
	failures := make([]string, 0, len(modelCodes))
	for _, modelCode := range modelCodes {
		model, errMsg := loadAgentAnalysisModel(ctx, pool, modelCode)
		if errMsg != "" {
			failures = append(failures, modelCode+"："+errMsg)
			continue
		}
		requestID := fmt.Sprintf("workflow_%d_comic_%s", workflowProjectID, modelCode)
		result, err := executeWorkerLLMWithRoutes(ctx, pool, baseURL, token, requestID, model, system, user, 0.7, 120*time.Second)
		if err != nil {
			failures = append(failures, modelCode+"："+err.Error())
			continue
		}
		text := extractLLMText(result.ResponseBody)
		if strings.TrimSpace(text) == "" {
			failures = append(failures, modelCode+"：模型未返回漫剧规划内容")
			continue
		}
		out := parseJSONish(text)
		if len(out) == 0 {
			out = fallbackComicDramaPlan(inputs, grid, text)
		}
		out = normalizeComicDramaPlan(out, inputs, runtimeCfg)
		persistComicDramaPlan(ctx, pool, workflowProjectID, userID, inputs, out)
		pt, ct := chatUsageTokens(result.ResponseBody)
		out["_analysis_cost"] = estimateModelCostByCodeWorker(ctx, pool, modelCode, result.RequestBody, pt, ct)
		out["_provider_cost"] = workerRouteProviderCost(result.Route, result.RequestBody, pt, ct)
		out["_route_id"] = nullableRouteID(result.Route.ID)
		out["_dialogue_model_code"] = modelCode
		out["raw_text"] = text
		return out, ""
	}
	if len(failures) == 0 {
		return nil, "未配置可用的 AI 漫剧剧本/对话模型"
	}
	return nil, "主备模型均不可用：" + strings.Join(failures, "；")
}

func compactUpstreamError(body []byte) string {
	message := strings.Join(strings.Fields(string(body)), " ")
	runes := []rune(message)
	if len(runes) > 180 {
		message = string(runes[:180]) + "…"
	}
	return message
}

func fallbackComicDramaPlan(inputs map[string]interface{}, grid int, raw string) map[string]interface{} {
	prompt := firstNonEmpty(firstUserPrompt(inputs), raw, "一个高质量 AI 漫剧短片")
	dialogue, narration := "", prompt
	if comicNarrationPerspective(inputs, map[string]interface{}{}) == "character_dialogue" {
		dialogue, narration = prompt, ""
	}
	storyboards := make([]map[string]interface{}, 0, grid)
	for i := 0; i < grid; i++ {
		id := fmt.Sprintf("S%02d", i+1)
		storyboards = append(storyboards, map[string]interface{}{
			"id":              id,
			"title":           fmt.Sprintf("分镜 %d", i+1),
			"duration_sec":    5,
			"scene":           prompt,
			"dialogue":        dialogue,
			"narration":       narration,
			"camera":          "稳定推进，电影感构图",
			"keyframe_prompt": prompt + "，AI 漫剧关键帧，角色一致，电影光影",
			"video_prompt":    prompt + "，AI 漫剧视频片段，镜头稳定推进，角色一致",
		})
	}
	return map[string]interface{}{
		"intent":             prompt,
		"creative_direction": "AI 漫剧短片",
		"outline":            prompt,
		"script":             prompt,
		"characters":         []map[string]interface{}{},
		"storyboards":        storyboards,
		"current_step":       "storyboard_confirm",
	}
}

func normalizeComicDramaPlan(plan, inputs, runtimeCfg map[string]interface{}) map[string]interface{} {
	grid := comicStoryboardGrid(runtimeCfg, inputs)
	storyboards := comicStoryboards(plan, runtimeCfg)
	if len(storyboards) == 0 {
		storyboards = comicStoryboards(fallbackComicDramaPlan(inputs, grid, ""), runtimeCfg)
	}
	if len(storyboards) > grid {
		storyboards = storyboards[:grid]
	}
	for len(storyboards) < grid {
		idx := len(storyboards) + 1
		fallbackSpeech := firstNonEmpty(stringAny(plan["outline"]), firstUserPrompt(inputs))
		dialogue, narration := "", fallbackSpeech
		if comicNarrationPerspective(inputs, runtimeCfg) == "character_dialogue" {
			dialogue, narration = fallbackSpeech, ""
		}
		storyboards = append(storyboards, map[string]interface{}{
			"id":              fmt.Sprintf("S%02d", idx),
			"title":           fmt.Sprintf("分镜 %d", idx),
			"duration_sec":    5,
			"scene":           firstNonEmpty(stringAny(plan["outline"]), firstUserPrompt(inputs)),
			"dialogue":        dialogue,
			"narration":       narration,
			"camera":          "电影感推进",
			"keyframe_prompt": firstNonEmpty(stringAny(plan["outline"]), firstUserPrompt(inputs)) + "，AI 漫剧关键帧",
			"video_prompt":    firstNonEmpty(stringAny(plan["outline"]), firstUserPrompt(inputs)) + "，AI 漫剧视频片段",
		})
	}
	plan["storyboards"] = storyboards
	plan["current_step"] = "storyboard_confirm"
	if stringAny(plan["intent"]) == "" {
		plan["intent"] = firstNonEmpty(stringAny(plan["outline"]), firstUserPrompt(inputs))
	}
	return plan
}

func runComicKeyframes(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, runtimeCfg, inputs map[string]interface{}, storyboards []map[string]interface{}, existing []interface{}) ([]map[string]interface{}, float64, string) {
	imageRuntime := copyMap(runtimeCfg)
	imageRuntime["generation_model_code"] = firstNonEmpty(stringAny(inputs["image_model_code"]), stringAny(runtimeCfg["image_model_code"]), stringAny(runtimeCfg["generation_model_code"]))
	imageRuntime["generation_type"] = "image"
	if stringAny(imageRuntime["generation_model_code"]) == "" {
		return nil, 0, "未配置 AI 漫剧图片模型"
	}
	items := make([]map[string]interface{}, 0, len(storyboards))
	var total float64
	maxRetry := intAny(firstNonNil(inputs["max_retry"], runtimeCfg["max_retry"]))
	if maxRetry < 0 {
		maxRetry = 0
	}
	if maxRetry > 5 {
		maxRetry = 5
	}
	existingByID := comicItemsByID(existing)
	for idx, sb := range storyboards {
		itemID := firstNonEmpty(stringAny(sb["id"]), fmt.Sprintf("S%02d", idx+1))
		if previous, ok := existingByID[itemID]; ok && stringAny(previous["image_url"]) != "" && stringAny(previous["status"]) != "failed" {
			if stored, err := persistComicKeyframeURL(ctx, pool, baseURL, token, stringAny(imageRuntime["generation_model_code"]), publicID, idx, stringAny(previous["image_url"])); err != nil {
				log.Printf("Workflow %s could not repair existing keyframe %s: %v", publicID, itemID, err)
			} else if stored != "" {
				previous["image_url"] = stored
			}
			items = append(items, previous)
			continue
		}
		prompt := firstNonEmpty(stringAny(sb["keyframe_prompt"]), stringAny(sb["scene"]), firstUserPrompt(inputs))
		prompt = comicStylePrompt(inputs, prompt)
		prompt = comicIdentityPrompt(inputs, prompt)
		taskInputs := copyMap(inputs)
		taskInputs["count"] = 1
		taskInputs["n"] = 1
		references := referenceImageURLs(inputs)
		// Project-level character/prop/location assets carry their selected image
		// URLs in metadata. Feed them into keyframe generation so the asset manager
		// is part of the real generation chain instead of prompt-only bookkeeping.
		for _, assetURL := range comicAssetReferenceURLs(inputs) {
			if len(references) >= 8 {
				break
			}
			references = appendUniqueMediaReference(references, assetURL)
		}
		// The first successful keyframe becomes a visual identity anchor for all
		// later shots, while the original user portrait remains the primary ref.
		if len(items) > 0 {
			if anchor := stringAny(items[0]["image_url"]); anchor != "" {
				references = appendUniqueMediaReference(references, anchor)
			}
		}
		if len(references) > 0 {
			taskInputs["reference_images"] = references
			taskInputs["image_url"] = references[0]
		}
		var results []map[string]interface{}
		errMsg := ""
		imageURL := ""
		retryCount := 0
		for attempt := 0; attempt <= maxRetry; attempt++ {
			if attempt > 0 {
				taskInputs["retry_reason"] = "previous keyframe result did not pass availability checks"
			}
			results, errMsg = runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, imageRuntime, taskInputs, prompt)
			total += sumAgentMediaTaskCost(results)
			output := map[string]interface{}{}
			if len(results) > 0 {
				output, _ = results[0]["output"].(map[string]interface{})
			}
			imageURL = firstMediaURL(output, "image_url", "url", "result_url")
			if errMsg == "" && imageURL != "" {
				break
			}
			retryCount = attempt + 1
		}
		if errMsg != "" && imageURL == "" {
			items = append(items, map[string]interface{}{
				"id": itemID, "title": stringAny(sb["title"]),
				"prompt": prompt, "status": "failed", "error_message": errMsg, "retry_count": retryCount,
			})
			return items, total, fmt.Sprintf("关键帧 %d 生成失败：%s", idx+1, errMsg)
		}
		if stored, err := persistComicKeyframeURL(ctx, pool, baseURL, token, stringAny(imageRuntime["generation_model_code"]), publicID, idx, imageURL); err != nil {
			// Persistence is a durability enhancement. Keep the upstream result
			// usable when storage is temporarily unavailable instead of failing
			// an otherwise successful and billable generation.
			log.Printf("Workflow %s keyframe %s persist failed, keeping upstream URL: %v", publicID, itemID, err)
		} else if stored != "" {
			imageURL = stored
		}
		items = append(items, map[string]interface{}{
			"id":          itemID,
			"title":       stringAny(sb["title"]),
			"prompt":      prompt,
			"image_url":   imageURL,
			"task":        firstMapOrNil(results),
			"scores":      comicPassScores(runtimeCfg, inputs),
			"retry_count": retryCount,
		})
	}
	return items, total, ""
}

func persistComicKeyframeURL(ctx context.Context, pool *pgxpool.Pool, baseURL, token, modelCode, publicID string, index int, imageURL string) (string, error) {
	conn := connectionConfig{}
	var extraRaw []byte
	if modelCode != "" {
		if err := pool.QueryRow(ctx, `SELECT COALESCE(new_api_extra_params,'{}'::jsonb) FROM models WHERE code=$1`, modelCode).Scan(&extraRaw); err == nil {
			extra := map[string]interface{}{}
			_ = json.Unmarshal(extraRaw, &extra)
			conn = parseConnection(extra, baseURL, token)
		}
	}
	return persistGeneratedMedia(ctx, conn, imageURL, publicID, fmt.Sprintf("keyframe_%03d", index+1), "image", 50<<20)
}

func runComicVideoSegments(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, runtimeCfg, inputs map[string]interface{}, storyboards []map[string]interface{}, keyframes, existing []interface{}) ([]map[string]interface{}, float64, string) {
	videoRuntime := copyMap(runtimeCfg)
	videoRuntime["generation_model_code"] = firstNonEmpty(stringAny(inputs["video_model_code"]), stringAny(runtimeCfg["video_model_code"]), stringAny(runtimeCfg["generation_model_code"]))
	videoRuntime["generation_type"] = "video"
	if stringAny(videoRuntime["generation_model_code"]) == "" {
		return nil, 0, "未配置 AI 漫剧视频模型"
	}
	items := make([]map[string]interface{}, 0, len(storyboards))
	var total float64
	maxRetry := intAny(firstNonNil(inputs["max_retry"], runtimeCfg["max_retry"]))
	if maxRetry < 0 {
		maxRetry = 0
	}
	if maxRetry > 5 {
		maxRetry = 5
	}
	existingByID := comicItemsByID(existing)
	audioStrategy := comicAudioStrategy(inputs, runtimeCfg)
	for idx, sb := range storyboards {
		itemID := firstNonEmpty(stringAny(sb["id"]), fmt.Sprintf("S%02d", idx+1))
		if previous, ok := existingByID[itemID]; ok && stringAny(previous["video_url"]) != "" && stringAny(previous["status"]) != "failed" {
			items = append(items, previous)
			continue
		}
		prompt := firstNonEmpty(stringAny(sb["video_prompt"]), stringAny(sb["scene"]), firstUserPrompt(inputs))
		prompt = comicStylePrompt(inputs, prompt)
		prompt = comicIdentityPrompt(inputs, prompt)
		speechText, speechType := comicStoryboardSpeech(sb, inputs, runtimeCfg)
		switch audioStrategy {
		case "video_native":
			if speechText != "" && speechType == "dialogue" {
				prompt += "\n原生同步音频要求：由画面中的角色自然说出以下对白，保持口型、人物身份、情绪和说话节奏一致；不要改写台词：\n“" + speechText + "”"
			} else if speechText != "" {
				prompt += "\n原生同步音频要求：使用清晰自然的画外旁白/内心独白朗读以下文字，不要让画面角色对口型说出旁白，不要改写：\n“" + speechText + "”"
			} else {
				prompt += "\n原生同步音频要求：生成与场景匹配的环境音和动作音效，不添加无关对白。"
			}
		case "hybrid":
			prompt += "\n音频要求：只生成与画面匹配的环境音、动作音效或轻背景氛围，不生成任何角色对白或旁白；对白将由独立配音轨道混合。"
		}
		taskInputs := copyMap(inputs)
		taskInputs["count"] = 1
		taskInputs["n"] = 1
		taskInputs["resolution"] = normalizeComicWorkerResolution(firstNonEmpty(stringAny(inputs["quality"]), stringAny(runtimeCfg["quality"])))
		taskInputs["ratio"] = comicWorkerAspectRatio(firstNonEmpty(stringAny(inputs["orientation"]), stringAny(runtimeCfg["orientation"])))
		taskInputs["aspect_ratio"] = taskInputs["ratio"]
		taskInputs["generate_audio"] = audioStrategy != "tts_only"
		if duration := intAny(sb["duration_sec"]); duration > 0 {
			taskInputs["duration"] = duration
			taskInputs["duration_sec"] = duration
		}
		if idx < len(keyframes) {
			if kf, ok := keyframes[idx].(map[string]interface{}); ok {
				if imageURL := stringAny(kf["image_url"]); imageURL != "" {
					// A video segment must use its generated keyframe as the only image
					// reference. Do not leak the comic style cover or the original upload
					// into Seedance's multimodal content array.
					for _, key := range []string{
						"image", "images", "product_image", "reference_image",
						"reference_images", "first_frame", "last_frame",
					} {
						delete(taskInputs, key)
					}
					taskInputs["image_url"] = imageURL
					taskInputs["reference_images"] = []string{imageURL}
					taskInputs["generation_mode"] = "image"
				}
			}
		}
		if stringAny(taskInputs["generation_mode"]) == "" {
			taskInputs["generation_mode"] = "text"
		}
		var results []map[string]interface{}
		errMsg := ""
		videoURL := ""
		retryCount := 0
		for attempt := 0; attempt <= maxRetry; attempt++ {
			if attempt > 0 {
				taskInputs["retry_reason"] = "previous video segment result did not pass availability checks"
			}
			results, errMsg = runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, videoRuntime, taskInputs, prompt)
			total += sumAgentMediaTaskCost(results)
			output := map[string]interface{}{}
			if len(results) > 0 {
				output, _ = results[0]["output"].(map[string]interface{})
			}
			videoURL = firstMediaURL(output, "video_url", "url", "result_url")
			if errMsg == "" && videoURL != "" {
				break
			}
			retryCount = attempt + 1
			if errMsg != "" && !isRetryableComicMediaError(errMsg) {
				break
			}
		}
		if errMsg != "" && videoURL == "" {
			items = append(items, map[string]interface{}{
				"id": itemID, "title": stringAny(sb["title"]),
				"prompt": prompt, "status": "failed", "error_message": errMsg, "retry_count": retryCount,
			})
			return items, total, fmt.Sprintf("分段视频 %d 生成失败：%s", idx+1, errMsg)
		}
		items = append(items, map[string]interface{}{
			"id":          itemID,
			"title":       stringAny(sb["title"]),
			"prompt":      prompt,
			"video_url":   videoURL,
			"task":        firstMapOrNil(results),
			"audio_mode":  audioStrategy,
			"speech_type": speechType,
			"retry_count": retryCount,
		})
	}
	return items, total, ""
}

func runComicNarrations(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, runtimeCfg, inputs map[string]interface{}, storyboards []map[string]interface{}, existing []interface{}) ([]map[string]interface{}, float64, string) {
	modelCode := firstNonEmpty(stringAny(inputs["narration_model_code"]), stringAny(runtimeCfg["narration_model_code"]))
	if modelCode == "" || comicAudioStrategy(inputs, runtimeCfg) == "video_native" {
		return nil, 0, ""
	}
	audioRuntime := copyMap(runtimeCfg)
	audioRuntime["generation_model_code"] = modelCode
	audioRuntime["generation_type"] = "audio"
	items := make([]map[string]interface{}, 0, len(storyboards))
	existingByID := comicItemsByID(existing)
	var total float64
	for idx, storyboard := range storyboards {
		itemID := firstNonEmpty(stringAny(storyboard["id"]), fmt.Sprintf("S%02d", idx+1))
		if previous, ok := existingByID[itemID]; ok && stringAny(previous["audio_url"]) != "" && stringAny(previous["status"]) != "failed" {
			items = append(items, previous)
			continue
		}
		speechText, speechType := comicStoryboardSpeech(storyboard, inputs, runtimeCfg)
		if speechText == "" {
			items = append(items, map[string]interface{}{
				"id": itemID, "title": stringAny(storyboard["title"]), "status": "skipped",
				"audio_url": "", "duration_sec": comicStoryboardDuration(storyboard),
			})
			continue
		}
		taskInputs := map[string]interface{}{
			"count":       1,
			"n":           1,
			"user_prompt": speechText,
			"speech_type": speechType,
			"_mode":       "auto",
		}
		for _, key := range []string{"voice_id", "emotion", "speed", "format"} {
			if value, ok := inputs[key]; ok {
				taskInputs[key] = value
			}
		}
		results, errMsg := runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, audioRuntime, taskInputs, speechText)
		total += sumAgentMediaTaskCost(results)
		output := map[string]interface{}{}
		if len(results) > 0 {
			output, _ = results[0]["output"].(map[string]interface{})
		}
		audioURL := firstMediaURL(output, "audio_url", "url", "result_url", "download_url")
		if errMsg != "" || audioURL == "" {
			errMsg = firstNonEmpty(errMsg, "配音模型未返回音频")
			items = append(items, map[string]interface{}{"id": itemID, "title": stringAny(storyboard["title"]), "dialogue": speechText, "speech_type": speechType, "status": "failed", "error_message": errMsg})
			return items, total, fmt.Sprintf("分镜 %d 配音失败：%s", idx+1, errMsg)
		}
		items = append(items, map[string]interface{}{
			"id": itemID, "title": stringAny(storyboard["title"]), "dialogue": speechText, "speech_type": speechType,
			"audio_url": audioURL, "status": "succeeded", "task": firstMapOrNil(results),
			"duration_sec": comicStoryboardDuration(storyboard),
		})
	}
	return items, total, ""
}

func comicStoryboardDuration(storyboard map[string]interface{}) float64 {
	duration := floatAny(storyboard["duration_sec"])
	if duration <= 0 {
		duration = 5
	}
	return duration
}

func comicNarrationPerspective(inputs, runtimeCfg map[string]interface{}) string {
	value := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringAny(inputs["narration_perspective"]), stringAny(runtimeCfg["narration_perspective"]), "smart")))
	switch value {
	case "smart", "first_person", "third_person", "character_dialogue":
		return value
	default:
		return "smart"
	}
}

func comicNarrationInstruction(mode string) string {
	switch mode {
	case "first_person":
		return "以主角第一人称“我”讲述，narration 使用主角内心独白，不使用全知视角；必要的角色对白单独放入 dialogue。"
	case "third_person":
		return "使用画外第三人称旁白，以角色姓名、他或她叙述，不让角色把旁白当对白说出；角色真实对白单独放入 dialogue。"
	case "character_dialogue":
		return "主要通过角色之间自然对白推动剧情，dialogue 必须适合角色口型与身份；仅在无法用画面和对白表达时使用极少量 narration。"
	default:
		return "根据剧情智能混合角色对白与画外旁白；动作场景减少旁白，情绪和信息转场可使用简短旁白。"
	}
}

func comicStoryboardSpeech(storyboard, inputs, runtimeCfg map[string]interface{}) (string, string) {
	dialogue := stringAny(storyboard["dialogue"])
	narration := stringAny(storyboard["narration"])
	switch comicNarrationPerspective(inputs, runtimeCfg) {
	case "first_person", "third_person":
		if narration != "" {
			return narration, "narration"
		}
		if dialogue != "" {
			return dialogue, "narration"
		}
	case "character_dialogue":
		if dialogue != "" {
			return dialogue, "dialogue"
		}
		if narration != "" {
			return narration, "narration"
		}
	default:
		if dialogue != "" {
			return dialogue, "dialogue"
		}
		if narration != "" {
			return narration, "narration"
		}
	}
	return "", "none"
}

func comicAudioStrategy(inputs, runtimeCfg map[string]interface{}) string {
	value := firstNonEmpty(stringAny(inputs["audio_strategy"]), stringAny(runtimeCfg["audio_strategy"]))
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "video_native", "tts_only", "hybrid":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		if firstNonEmpty(stringAny(inputs["narration_model_code"]), stringAny(runtimeCfg["narration_model_code"])) != "" {
			return "hybrid"
		}
		return "video_native"
	}
}

func comicItemsByID(items []interface{}) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{}, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if id := stringAny(item["id"]); id != "" {
			result[id] = item
		}
	}
	return result
}

func comicStageComplete(items []interface{}, storyboards []map[string]interface{}, outputKey string) bool {
	if len(items) < len(storyboards) || len(storyboards) == 0 {
		return false
	}
	byID := comicItemsByID(items)
	for idx, storyboard := range storyboards {
		id := firstNonEmpty(stringAny(storyboard["id"]), fmt.Sprintf("S%02d", idx+1))
		item, ok := byID[id]
		if !ok || stringAny(item[outputKey]) == "" || stringAny(item["status"]) == "failed" {
			return false
		}
	}
	return true
}

func comicNarrationStageComplete(items []interface{}, storyboards []map[string]interface{}) bool {
	if len(items) < len(storyboards) || len(storyboards) == 0 {
		return false
	}
	byID := comicItemsByID(items)
	for idx, storyboard := range storyboards {
		id := firstNonEmpty(stringAny(storyboard["id"]), fmt.Sprintf("S%02d", idx+1))
		item, ok := byID[id]
		if !ok || stringAny(item["status"]) == "failed" {
			return false
		}
		dialogue := firstNonEmpty(stringAny(storyboard["dialogue"]), stringAny(storyboard["narration"]))
		if dialogue == "" {
			if stringAny(item["status"]) != "skipped" {
				return false
			}
			continue
		}
		if stringAny(item["audio_url"]) == "" {
			return false
		}
	}
	return true
}

func normalizeComicWorkerResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480p", "720p", "1080p", "4k":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "480p"
	}
}

func comicWorkerAspectRatio(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "portrait") {
		return "9:16"
	}
	return "16:9"
}

func composeComicDramaVideo(ctx context.Context, pool *pgxpool.Pool, publicID string, segments, narrations []interface{}, inputs, runtimeCfg map[string]interface{}) (map[string]interface{}, string) {
	if objectStore == nil {
		return nil, "对象存储未配置，无法保存 AI 漫剧合成视频"
	}
	ffmpegPath, err := ffmpegBinaryPath()
	if err != nil {
		return nil, "AI 漫剧视频合成不可用：" + err.Error()
	}
	tmpDir, err := os.MkdirTemp("", "starai-comic-*")
	if err != nil {
		return nil, "创建临时目录失败：" + err.Error()
	}
	defer os.RemoveAll(tmpDir)
	listPath := filepath.Join(tmpDir, "list.txt")
	var list bytes.Buffer
	downloaded := 0
	conn := connectionConfig{}
	modelCode := firstNonEmpty(stringAny(inputs["video_model_code"]), stringAny(runtimeCfg["video_model_code"]), stringAny(runtimeCfg["generation_model_code"]))
	if modelCode != "" {
		var extraRaw []byte
		if err := pool.QueryRow(ctx, `SELECT COALESCE(new_api_extra_params,'{}'::jsonb) FROM models WHERE code=$1`, modelCode).Scan(&extraRaw); err == nil {
			extra := map[string]interface{}{}
			_ = json.Unmarshal(extraRaw, &extra)
			conn = parseConnection(extra, "", "")
		}
	}
	for idx, raw := range segments {
		seg, _ := raw.(map[string]interface{})
		if seg == nil {
			continue
		}
		videoURL := stringAny(seg["video_url"])
		if videoURL == "" {
			continue
		}
		data, _, err := downloadAuthenticatedMedia(ctx, conn, videoURL, 500<<20)
		if err != nil {
			return nil, fmt.Sprintf("下载分段视频 %d 失败：%s", idx+1, err.Error())
		}
		partPath := filepath.Join(tmpDir, fmt.Sprintf("part_%03d.mp4", idx+1))
		if err := os.WriteFile(partPath, data, 0600); err != nil {
			return nil, "写入分段视频失败：" + err.Error()
		}
		list.WriteString("file '")
		list.WriteString(strings.ReplaceAll(partPath, "'", "'\\''"))
		list.WriteString("'\n")
		downloaded++
	}
	if downloaded == 0 {
		return nil, "没有可合成的分段视频"
	}
	if err := os.WriteFile(listPath, list.Bytes(), 0600); err != nil {
		return nil, "写入合成列表失败：" + err.Error()
	}
	outPath := filepath.Join(tmpDir, "final.mp4")
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-c:a", "aac", "-movflags", "+faststart", outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "ffmpeg 合成失败：" + truncateText(stderr.String(), 300)
	}
	narrationCount := 0
	if len(narrations) > 0 {
		narrationPath, count, narrationErr := prepareComicNarrationTrack(ctx, tmpDir, narrations)
		if narrationErr != nil {
			return nil, narrationErr.Error()
		}
		narrationCount = count
		if narrationPath != "" {
			dubbedPath := filepath.Join(tmpDir, "final_dubbed.mp4")
			args := []string{"-y", "-i", outPath, "-i", narrationPath}
			if mediaHasAudio(ctx, outPath) {
				args = append(args,
					"-filter_complex", "[0:a]volume=0.25[bg];[1:a]volume=1.0[voice];[bg][voice]amix=inputs=2:duration=first:dropout_transition=2[a]",
					"-map", "0:v:0", "-map", "[a]")
			} else {
				args = append(args, "-map", "0:v:0", "-map", "1:a:0")
			}
			args = append(args, "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-shortest", dubbedPath)
			if err := runFFmpeg(ctx, args...); err != nil {
				return nil, "配音混合失败：" + err.Error()
			}
			outPath = dubbedPath
		}
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, "读取合成视频失败：" + err.Error()
	}
	objectName := fmt.Sprintf("works/video/%s/final_%d.mp4", publicID, time.Now().UnixNano())
	publicURL, err := objectStore.Upload(ctx, objectName, "video/mp4", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "上传合成视频失败：" + err.Error()
	}
	thumbnailURL := ""
	thumbPath := filepath.Join(tmpDir, "thumbnail.jpg")
	thumbCmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-ss", "0.1", "-i", outPath, "-frames:v", "1", "-q:v", "3", thumbPath)
	if err := thumbCmd.Run(); err == nil {
		if thumbData, readErr := os.ReadFile(thumbPath); readErr == nil && len(thumbData) > 0 {
			thumbName := fmt.Sprintf("works/video/%s/thumbnail_%d.jpg", publicID, time.Now().UnixNano())
			thumbnailURL, _ = objectStore.Upload(ctx, thumbName, "image/jpeg", bytes.NewReader(thumbData), int64(len(thumbData)))
		}
	}
	return map[string]interface{}{"final_video_url": publicURL, "video_url": publicURL, "thumbnail": thumbnailURL, "segments": downloaded, "narrations": narrationCount, "audio_strategy": comicAudioStrategy(inputs, runtimeCfg), "narration_perspective": comicNarrationPerspective(inputs, runtimeCfg), "orientation": stringAny(inputs["orientation"]), "quality": stringAny(inputs["quality"])}, ""
}

func prepareComicNarrationTrack(ctx context.Context, tmpDir string, narrations []interface{}) (string, int, error) {
	listPath := filepath.Join(tmpDir, "narrations.txt")
	var list strings.Builder
	count := 0
	for idx, raw := range narrations {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		duration := floatAny(item["duration_sec"])
		if duration <= 0 {
			duration = 5
		}
		durationArg := strconv.FormatFloat(duration, 'f', 3, 64)
		audioURL := stringAny(item["audio_url"])
		path := filepath.Join(tmpDir, fmt.Sprintf("narration_part_%03d.m4a", idx+1))
		if audioURL == "" {
			if err := runFFmpeg(ctx,
				"-y", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
				"-t", durationArg, "-c:a", "aac", "-b:a", "192k", path,
			); err != nil {
				return "", count, fmt.Errorf("生成分镜 %d 静音占位失败：%w", idx+1, err)
			}
		} else {
			data, _, err := downloadAuthenticatedMedia(ctx, connectionConfig{}, audioURL, 100<<20)
			if err != nil {
				return "", count, fmt.Errorf("下载分镜 %d 配音失败：%w", idx+1, err)
			}
			sourcePath := filepath.Join(tmpDir, fmt.Sprintf("narration_source_%03d", idx+1))
			if err := os.WriteFile(sourcePath, data, 0600); err != nil {
				return "", count, fmt.Errorf("写入分镜配音失败：%w", err)
			}
			if err := runFFmpeg(ctx,
				"-y", "-i", sourcePath, "-af", "apad",
				"-t", durationArg, "-ar", "44100", "-ac", "2",
				"-c:a", "aac", "-b:a", "192k", path,
			); err != nil {
				return "", count, fmt.Errorf("标准化分镜 %d 配音失败：%w", idx+1, err)
			}
			count++
		}
		list.WriteString("file '")
		list.WriteString(strings.ReplaceAll(path, "'", "'\\''"))
		list.WriteString("'\n")
	}
	if count == 0 {
		return "", 0, nil
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0600); err != nil {
		return "", count, err
	}
	outputPath := filepath.Join(tmpDir, "narration_track.m4a")
	if err := runFFmpeg(ctx, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c:a", "aac", "-b:a", "192k", outputPath); err != nil {
		return "", count, fmt.Errorf("拼接配音失败：%w", err)
	}
	return outputPath, count, nil
}

func insertComicDramaWork(ctx context.Context, pool *pgxpool.Pool, userID int64, runtimeCfg, inputs, final map[string]interface{}) {
	videoURL := firstNonEmpty(stringAny(final["final_video_url"]), stringAny(final["video_url"]))
	if videoURL == "" {
		return
	}
	var modelID *int64
	modelCode := firstNonEmpty(stringAny(inputs["video_model_code"]), stringAny(runtimeCfg["video_model_code"]), stringAny(runtimeCfg["generation_model_code"]))
	if modelCode != "" {
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM models WHERE code=$1`, modelCode).Scan(&id); err == nil {
			modelID = &id
		}
	}
	meta := map[string]interface{}{
		"video_url":        videoURL,
		"final_video_url":  videoURL,
		"thumbnail":        firstNonEmpty(stringAny(final["thumbnail"]), videoURL),
		"source":           "ai_comic_drama",
		"comic_project_id": stringAny(inputs["comic_project_id"]),
		"segments":         final["segments"],
		"narrations":       final["narrations"],
		"narration_mode":   comicNarrationPerspective(inputs, runtimeCfg),
	}
	publicID := fmt.Sprintf("work_%d", time.Now().UnixNano())
	_, _ = pool.Exec(ctx, `
		INSERT INTO works (public_id, user_id, model_id, type, title, prompt, thumbnail_url, metadata)
		VALUES ($1,$2,$3,'video',$4,$5,$6,$7)`,
		publicID,
		userID,
		modelID,
		firstNonEmpty(stringAny(inputs["comic_project_name"]), "AI漫剧成片"),
		firstUserPrompt(inputs),
		firstNonEmpty(stringAny(final["thumbnail"]), videoURL),
		mustJSON(meta),
	)
}

func runAgentAnalysis(ctx context.Context, pool *pgxpool.Pool, baseURL, token, modelCode, category string, runtimeCfg, inputs map[string]interface{}) (map[string]interface{}, string) {
	if modelCode == "" {
		modelCode = "chat_demo_v1"
	}
	model, errMsg := loadAgentAnalysisModel(ctx, pool, modelCode)
	if errMsg != "" {
		return nil, errMsg
	}
	sceneCode := stringAny(inputs["creative_scene"])
	sceneLabel := firstNonEmpty(stringAny(inputs["creative_scene_label"]), agentCreativeSceneLabel(sceneCode))
	system := buildAgentAnalysisSystemPrompt(category, stringAny(runtimeCfg["preset_code"]), intAny(runtimeCfg["candidate_count"]), sceneCode)
	content := fmt.Sprintf("用户需求：%s\n参考图URL：%s\n出图场景：%s\n当前生成参数：%s\n请补全创作方案。", firstUserPrompt(inputs), firstImageURL(inputs), sceneLabel, agentGenerationParamSummary(inputs))
	if hasSubjectReferenceImage(inputs) {
		content += "\n参考图是生成主体的唯一视觉真值。不得猜测、替换或重新发明主体品类；如果无法从 URL 直接识别图片内容，候选提示词必须写成严格保持参考图主体，不得擅自写成手机、无人机或其他具体品类。"
	}
	requestID := fmt.Sprintf("agent_%s_%d", modelCode, time.Now().UnixNano())
	result, err := executeWorkerLLMWithRoutes(ctx, pool, baseURL, token, requestID, model, system, content, 0.65, 90*time.Second)
	if err != nil {
		return nil, "模型服务异常：" + err.Error()
	}
	text := extractLLMText(result.ResponseBody)
	if strings.TrimSpace(text) == "" {
		return nil, "模型未返回分析内容"
	}
	out := normalizeAgentAnalysisOutput(text, category)
	pt, ct := chatUsageTokens(result.ResponseBody)
	out["_analysis_cost"] = estimateModelCostByCodeWorker(ctx, pool, modelCode, result.RequestBody, pt, ct)
	out["_provider_cost"] = workerRouteProviderCost(result.Route, result.RequestBody, pt, ct)
	out["_route_id"] = nullableRouteID(result.Route.ID)
	return out, ""
}

type agentAnalysisModel struct {
	ID            int64
	Code          string
	UpstreamModel string
	Endpoint      string
	RequestMode   string
	ExtraParams   map[string]interface{}
	RuntimeRule   map[string]interface{}
}

type workerLLMResult struct {
	Route        workerModelRoute
	RequestBody  map[string]interface{}
	ResponseBody []byte
}

func loadAgentAnalysisModel(ctx context.Context, pool *pgxpool.Pool, modelCode string) (agentAnalysisModel, string) {
	model := agentAnalysisModel{Code: modelCode}
	var extraRaw, runtimeRaw []byte
	if err := pool.QueryRow(ctx, `
		SELECT id, COALESCE(new_api_model,''), COALESCE(new_api_endpoint,''), COALESCE(request_mode,''),
		       COALESCE(new_api_extra_params,'{}'::jsonb), COALESCE(runtime_rule,'{}'::jsonb)
		FROM models WHERE code=$1 AND is_enabled=true AND category='chat'`, modelCode).Scan(&model.ID, &model.UpstreamModel, &model.Endpoint, &model.RequestMode, &extraRaw, &runtimeRaw); err != nil {
		return agentAnalysisModel{}, "分析模型不存在、类型不匹配或未启用：" + modelCode
	}
	_ = json.Unmarshal(extraRaw, &model.ExtraParams)
	_ = json.Unmarshal(runtimeRaw, &model.RuntimeRule)
	if model.ExtraParams == nil {
		model.ExtraParams = map[string]interface{}{}
	}
	if model.RuntimeRule == nil {
		model.RuntimeRule = map[string]interface{}{}
	}
	return model, ""
}

func executeWorkerLLMWithRoutes(ctx context.Context, pool *pgxpool.Pool, baseURL, token, requestID string, model agentAnalysisModel, system, user string, temperature float64, defaultTimeout time.Duration) (workerLLMResult, error) {
	routes, err := loadWorkerModelRoutes(ctx, pool, model.ID, baseURL, token, model.UpstreamModel, model.Endpoint, model.ExtraParams, model.RuntimeRule)
	if err != nil {
		return workerLLMResult{}, err
	}
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 64 {
		requestID = requestID[:64]
	}
	attempt := 0
	failures := make([]string, 0, len(routes))
	// 仅多线路时启用自动切换/熔断降级；单线路保持旧的直连行为。
	poolEnabled := len(routes) > 1
	for _, route := range routes {
		if attempt >= maxWorkerRouteAttempts {
			break
		}
		if poolEnabled && !acquireWorkerRouteProbe(ctx, pool, route) {
			continue
		}
		bodyMap, endpoint := buildWorkerLLMRequest(route, model.RequestMode, model.Code, system, user, temperature)
		body, marshalErr := json.Marshal(bodyMap)
		if marshalErr != nil {
			return workerLLMResult{}, marshalErr
		}
		conn := route.Connection
		if conn.Headers == nil {
			conn.Headers = map[string]string{}
		}
		if !hasWorkerHeader(conn.Headers, "Idempotency-Key") && requestID != "" {
			conn.Headers["Idempotency-Key"] = requestID
		}
		if normalizeWorkerLLMProtocol(route.Protocol) == "claude" && !hasWorkerHeader(conn.Headers, "anthropic-version") {
			conn.Headers["anthropic-version"] = "2023-06-01"
		}
		timeout := defaultTimeout
		if route.TimeoutSeconds > 0 {
			timeout = time.Duration(route.TimeoutSeconds) * time.Second
		}
		retries := route.MaxRetries
		if retries < 0 {
			retries = 0
		}
		for retry := 0; retry <= retries && attempt < maxWorkerRouteAttempts; retry++ {
			attempt++
			started := time.Now()
			responseBody, status, requestErr := doJSONRequest(ctx, conn, "POST", joinBaseEndpoint(conn.BaseURL, resolveModelEndpoint(endpoint, route.UpstreamModel)), body, timeout)
			latencyMS := int(time.Since(started).Milliseconds())
			if requestErr == nil && status >= 200 && status < 300 {
				markWorkerRouteSuccess(ctx, pool, route.ID)
				logWorkerRouteAttempt(ctx, pool, requestID, model.ID, route.ID, attempt, "SUCCESS", status, latencyMS)
				promptTokens, outputTokens := chatUsageTokens(responseBody)
				updateWorkerRouteAttemptProviderCost(ctx, pool, requestID, route.ID, workerRouteProviderCost(route, bodyMap, promptTokens, outputTokens))
				return workerLLMResult{Route: route, RequestBody: bodyMap, ResponseBody: responseBody}, nil
			}
			statusLabel := "ERROR"
			if status > 0 {
				statusLabel = fmt.Sprintf("HTTP_%d", status)
			}
			logWorkerRouteAttempt(ctx, pool, requestID, model.ID, route.ID, attempt, statusLabel, status, latencyMS)
			if workerStatusCanFailover(status) {
				markWorkerRouteFailure(ctx, pool, route.ID, poolEnabled)
			}
			message := compactUpstreamError(responseBody)
			if requestErr != nil {
				message = requestErr.Error()
			}
			failures = append(failures, fmt.Sprintf("%s (HTTP %d): %s", firstNonEmpty(route.UpstreamModel, model.Code), status, message))
			if !workerStatusCanFailover(status) {
				return workerLLMResult{}, fmt.Errorf("上游拒绝请求：HTTP %d %s", status, message)
			}
			if retry < retries && workerShouldRetrySameRoute(requestErr, status) && waitWorkerRouteRetry(ctx, retry) {
				continue
			}
			break
		}
	}
	if len(failures) == 0 {
		return workerLLMResult{}, errors.New("没有可用线路，线路可能已禁用、正在冷却或被其他请求探测")
	}
	return workerLLMResult{}, errors.New(strings.Join(failures, "；"))
}

func buildWorkerLLMRequest(route workerModelRoute, requestMode, fallbackModel, system, user string, temperature float64) (map[string]interface{}, string) {
	body := copyLLMExtraParams(route.ExtraParams)
	model := firstNonEmpty(route.UpstreamModel, fallbackModel)
	body["model"] = model
	protocol := normalizeWorkerLLMProtocol(route.Protocol)
	switch protocol {
	case "claude":
		delete(body, "input")
		body["system"] = system
		body["messages"] = []map[string]string{{"role": "user", "content": user}}
		if _, ok := body["max_tokens"]; !ok {
			body["max_tokens"] = 4096
		}
		if _, ok := body["temperature"]; !ok {
			body["temperature"] = temperature
		}
		return body, firstNonEmpty(strings.TrimSpace(route.Endpoint), "/v1/messages")
	case "gemini":
		delete(body, "messages")
		delete(body, "input")
		delete(body, "temperature")
		body["systemInstruction"] = map[string]interface{}{"parts": []map[string]string{{"text": system}}}
		body["contents"] = []map[string]interface{}{{"role": "user", "parts": []map[string]string{{"text": user}}}}
		generationConfig, _ := body["generationConfig"].(map[string]interface{})
		if generationConfig == nil {
			generationConfig = map[string]interface{}{}
		}
		if _, ok := generationConfig["temperature"]; !ok {
			generationConfig["temperature"] = temperature
		}
		body["generationConfig"] = generationConfig
		return body, firstNonEmpty(strings.TrimSpace(route.Endpoint), "/v1beta/models/{model}:generateContent")
	default:
		setLLMRequestContent(body, requestMode, system, user)
		if _, ok := body["temperature"]; !ok {
			body["temperature"] = temperature
		}
		if requestMode == "responses" {
			return body, firstNonEmpty(strings.TrimSpace(route.Endpoint), "/v1/responses")
		}
		return body, firstNonEmpty(strings.TrimSpace(route.Endpoint), "/v1/chat/completions")
	}
}

func normalizeWorkerLLMProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "anthropic", "anthropic_messages", "claude":
		return "claude"
	case "google", "google_gemini", "gemini":
		return "gemini"
	default:
		return "openai"
	}
}

func hasWorkerHeader(headers map[string]string, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func setLLMRequestContent(body map[string]interface{}, requestMode, system, user string) {
	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	if requestMode == "responses" {
		body["input"] = messages
		delete(body, "messages")
		return
	}
	body["messages"] = messages
	delete(body, "input")
}

func buildAgentAnalysisSystemPrompt(category, presetCode string, candidateCount int, creativeScene string) string {
	target := "图片"
	extra := "每个候选方案必须适合图片生成模型，包含主体、材质、构图、光线、背景、商品卖点、商业质感、平台电商主图规范；prompt 要能直接传给图片生成接口。"
	if category == "video" {
		target = "视频"
		extra = "每个候选方案必须适合视频生成模型，包含镜头运动、节奏、时长感、商品卖点、首尾帧衔接、平台短视频风格；prompt 要能直接传给视频生成接口。"
	}
	if candidateCount <= 0 {
		candidateCount = 3
	}
	scene := agentPresetInstruction(presetCode, category)
	scene = firstNonEmpty(agentCreativeSceneInstruction(creativeScene), scene)
	detailPlan := ""
	if creativeScene == "detail_image" && category != "video" {
		detailPlan = `
这是商品详情长图任务。除 candidates 外必须额外返回 detail_sections，按详情页从上到下排列 4–8 个模块。
每个模块结构：{"id":"detail_01","type":"hero|benefit|material|feature|usage|specification|closing","title":"模块标题","objective":"本模块目的","copy_title":"后期排版标题","copy_points":["已确认卖点"],"image_prompt":"只描述商品、场景、构图、材质、光影和文字留白区，不要求图片模型绘制文字"}。
必须保持商品外观、颜色、包装、Logo位置和比例跨模块一致；不得编造用户未提供的成分、尺寸、容量、认证或功效。规格模块没有可靠参数时只提供版式和留白，不得杜撰数据。`
	}
	return fmt.Sprintf(`你是电商AI创作智能体的方案分析引擎，当前生成类型是%s。
只输出严格JSON，不要Markdown，不要标题，不要解释，不要出现“某模型的回答”。
禁止输出与创作无关的运维、CPU、IO、数据库、系统瓶颈、监控等泛化建议。
当前创作场景：%s
必须严格遵守用户当前选择的生成参数，例如数量、时长、画面方向、比例、质量、参考图设置；不要在 prompt 中写入与这些参数冲突的时长、比例或方向。
你必须基于用户需求和参考图，给出%d条可选择的创作方案，并标记AI推荐方案。
JSON结构：
{
  "summary": "一句话概括创作目标",
  "user_intent": "用户真实需求",
  "asset_notes": "参考图中可利用的视觉信息；没有参考图则说明无",
  "selling_points": ["卖点1","卖点2","卖点3"],
  "style": "整体商业风格",
  "recommendation": "A",
  "candidates": [
    {"id":"A","title":"方案名","reason":"推荐理由","prompt":"可直接生成的完整提示词","negative_prompt":"需要避免的内容","params":{}},
    {"id":"B","title":"方案名","reason":"适用场景","prompt":"可直接生成的完整提示词","negative_prompt":"需要避免的内容","params":{}},
    {"id":"C","title":"方案名","reason":"适用场景","prompt":"可直接生成的完整提示词","negative_prompt":"需要避免的内容","params":{}}
  ],
  "generation_prompt": "AI推荐方案的prompt",
  "detail_sections": []
}
%s
%s`, target, scene, candidateCount, extra, detailPlan)
}

func agentPresetInstruction(code, category string) string {
	switch code {
	case "ecommerce_scene_image":
		return "电商场景图。重点是保留商品主体识别度，补全真实使用场景，强化材质、尺度、光影和购买欲。"
	case "poster_image":
		return "营销海报。重点是广告构图、标题留白、品牌质感、活动氛围和可读性，避免把文字直接画错。"
	case "product_showcase_video":
		return "商品展示短视频。重点是首秒吸引、商品运镜、卖点节奏、镜头运动和平台短视频质感。"
	case "image_to_video":
		return "图生视频。重点是保持参考图主体一致，添加合理运动、镜头推进、光影变化和动态氛围。"
	default:
		if category == "video" {
			return "通用视频创作。重点是镜头、运动、节奏、主体一致性和可直接执行的视频提示词。"
		}
		return "电商商品主图。重点是商品主体清晰、白底或高级简洁背景、材质纹理、商业光影和平台主图规范。"
	}
}

func agentCreativeSceneLabel(code string) string {
	switch code {
	case "detail_image":
		return "商品详情图"
	case "scene_image":
		return "场景图"
	case "marketing_poster":
		return "营销海报"
	case "product_video":
		return "商品视频"
	case "image_to_video":
		return "图生视频"
	default:
		if code == "" {
			return "商品主图"
		}
		return code
	}
}

func agentCreativeSceneInstruction(code string) string {
	switch code {
	case "detail_image":
		return "商品详情图 / Product detail image. 必须突出商品结构、材质细节、功能卖点、规格层次和详情页模块感；不要生成普通商品主图、单一白底主图或营销海报。"
	case "scene_image":
		return "电商场景图 / Lifestyle scene image. 必须保留商品主体识别度，并把商品放入真实、有购买欲的使用场景；强化环境、生活方式、光影和商业质感；不要生成普通白底主图。"
	case "marketing_poster":
		return "营销海报 / Marketing poster. 必须使用广告构图、活动氛围、品牌质感、标题留白和传播冲击力；画面应像平台推广素材；不要生成普通商品主图或详情图。"
	case "product_video":
		return "商品视频 / Product showcase video. 必须围绕商品主体做展示短视频，包含首秒吸引、卖点节奏、商品运镜、商业光影和平台短视频质感；不要生成无关风景、空镜或默认素材。"
	case "image_to_video":
		return "图生视频 / Image-to-video. 必须严格保持参考图主体、材质和核心结构一致，只增加合理运动、镜头推进、光影变化和动态氛围；不要重新设计主体，不要变成普通商品视频。"
	case "main_image", "":
		return "电商商品主图 / Main product image. 必须商品主体清晰，背景干净或高级简洁，材质纹理突出，符合平台主图规范；避免过度场景化、详情页排版和复杂文字。"
	default:
		return ""
	}
}

func agentGenerationParamSummary(inputs map[string]interface{}) string {
	items := []string{}
	if s := generationLanguageLabel(inputs); s != "" {
		items = append(items, "生成语言="+s)
	}
	if s := stringAny(inputs["creative_scene_label"]); s != "" {
		items = append(items, "场景="+s)
	} else if s := agentCreativeSceneLabel(stringAny(inputs["creative_scene"])); s != "" {
		items = append(items, "场景="+s)
	}
	if n := intAny(inputs["count"]); n > 0 {
		items = append(items, fmt.Sprintf("数量=%d", n))
	} else if n := intAny(inputs["n"]); n > 0 {
		items = append(items, fmt.Sprintf("数量=%d", n))
	}
	for _, key := range []string{"duration", "duration_sec", "seconds"} {
		if s := stringAny(inputs[key]); s != "" {
			items = append(items, "时长="+s)
			break
		}
		if n := intAny(inputs[key]); n > 0 {
			items = append(items, fmt.Sprintf("时长=%d秒", n))
			break
		}
	}
	for _, key := range []string{"orientation", "direction"} {
		if s := stringAny(inputs[key]); s != "" {
			items = append(items, "画面方向="+s)
			break
		}
	}
	for _, key := range []string{"aspect_ratio", "ratio", "size"} {
		if s := stringAny(inputs[key]); s != "" {
			items = append(items, "比例/尺寸="+s)
			break
		}
	}
	if s := stringAny(inputs["quality"]); s != "" {
		items = append(items, "质量="+s)
	}
	refCount := 0
	if s := stringAny(inputs["image_url"]); s != "" {
		refCount++
	}
	if s := stringAny(inputs["first_frame"]); s != "" {
		refCount++
	}
	if s := stringAny(inputs["last_frame"]); s != "" {
		refCount++
	}
	for _, key := range []string{"reference_images", "reference_asset_ids", "asset_ids"} {
		switch v := inputs[key].(type) {
		case []interface{}:
			refCount += len(v)
		case []string:
			refCount += len(v)
		}
	}
	if refCount > 0 {
		items = append(items, fmt.Sprintf("参考图=%d张", refCount))
	} else {
		items = append(items, "参考图=无")
	}
	if len(items) == 0 {
		return "无特别参数"
	}
	return strings.Join(items, "；")
}

func agentPromptWithScene(prompt string, inputs map[string]interface{}) string {
	sceneCode := stringAny(inputs["creative_scene"])
	sceneLabel := firstNonEmpty(stringAny(inputs["creative_scene_label"]), agentCreativeSceneLabel(sceneCode))
	sceneInstruction := agentCreativeSceneInstruction(sceneCode)
	sections := make([]string, 0, 3)
	if strings.TrimSpace(sceneInstruction) != "" {
		sections = append(sections, fmt.Sprintf("SCENE HARD REQUIREMENT: %s (%s)\n%s\nThe final media MUST visibly follow this scene. If the user prompt or AI analysis conflicts, obey this scene requirement.\n当前生成参数：%s", sceneLabel, sceneCode, sceneInstruction, agentGenerationParamSummary(inputs)))
	}
	if hasSubjectReferenceImage(inputs) {
		sections = append(sections, "REFERENCE IMAGE HARD REQUIREMENT: The uploaded reference image is the authoritative subject. Preserve its object category, identity, silhouette, structure, proportions, materials, colors, visible details, branding and logo placement. Never replace it with another object (for example, never turn a phone into a drone). If the user prompt or AI analysis conflicts with the reference subject, obey the reference image. Only change the scene, composition, lighting or presentation requested by the user.")
	}
	if cleanPrompt := strings.TrimSpace(prompt); cleanPrompt != "" {
		sections = append(sections, cleanPrompt)
	}
	return applyGenerationLanguage(strings.Join(sections, "\n\n"), inputs)
}

func generationLanguageLabel(inputs map[string]interface{}) string {
	return firstNonEmpty(
		stringAny(inputs["language_label"]),
		stringAny(inputs["generation_language_label"]),
		stringAny(inputs["language_name"]),
		stringAny(inputs["generation_language_name"]),
		stringAny(inputs["language"]),
		stringAny(inputs["generation_language"]),
	)
}

func applyGenerationLanguage(prompt string, inputs map[string]interface{}) string {
	lang := strings.TrimSpace(generationLanguageLabel(inputs))
	if lang == "" {
		return prompt
	}
	cleanPrompt := strings.TrimSpace(prompt)
	instruction := fmt.Sprintf("LANGUAGE HARD REQUIREMENT: Generate all visible text, labels, captions, subtitles, product copy and marketing copy in %s. Unless the user's prompt explicitly requests another language, do not switch languages.", lang)
	if strings.Contains(cleanPrompt, "LANGUAGE HARD REQUIREMENT:") {
		return cleanPrompt
	}
	if cleanPrompt == "" {
		return instruction
	}
	return instruction + "\n\n" + cleanPrompt
}

func sumAgentMediaTaskCost(tasks []map[string]interface{}) float64 {
	total := 0.0
	for _, item := range tasks {
		total += floatAny(item["actual_cost"])
	}
	return total
}

func workflowActualCost(ctx context.Context, pool *pgxpool.Pool, projectID int64, outputs map[string]interface{}) float64 {
	base := workflowBaseCost(ctx, pool, projectID)
	var nodeCost float64
	_ = pool.QueryRow(ctx, `SELECT COALESCE(SUM(cost),0) FROM workflow_node_runs WHERE project_id=$1`, projectID).Scan(&nodeCost)
	mediaCost := 0.0
	if raw, ok := outputs["media_tasks"].([]interface{}); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]interface{}); ok {
				mediaCost += floatAny(m["actual_cost"])
			}
		}
	}
	if raw, ok := outputs["media_tasks"].([]map[string]interface{}); ok {
		for _, item := range raw {
			mediaCost += floatAny(item["actual_cost"])
		}
	}
	if mediaCost <= 0 {
		mediaCost = floatAny(outputs["cost"])
	}
	return selectWorkflowActualCost(base, nodeCost, mediaCost)
}

func workflowAccruedCost(ctx context.Context, pool *pgxpool.Pool, projectID int64) float64 {
	var nodeCost float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(cost),0) FROM workflow_node_runs WHERE project_id=$1`, projectID).Scan(&nodeCost); err != nil || nodeCost <= 0 {
		return 0
	}
	if base := workflowBaseCost(ctx, pool, projectID); base > 0 {
		return base
	}
	return nodeCost
}

func incrementalWorkflowCharge(ctx context.Context, pool *pgxpool.Pool, projectID int64, cumulativeCost float64) float64 {
	var settled float64
	if err := pool.QueryRow(ctx, `SELECT actual_cost FROM workflow_projects WHERE id=$1`, projectID).Scan(&settled); err != nil {
		return cumulativeCost
	}
	return incrementalChargeAmount(cumulativeCost, settled)
}

func incrementalChargeAmount(cumulativeCost, settledCost float64) float64 {
	incremental := cumulativeCost - settledCost
	if incremental < 0 {
		return 0
	}
	return incremental
}

func selectWorkflowActualCost(flatPrice, nodeCost, mediaCost float64) float64 {
	if flatPrice > 0 {
		return flatPrice
	}
	if nodeCost > 0 {
		return nodeCost
	}
	if mediaCost > 0 {
		return mediaCost
	}
	return 0
}

func workflowBaseCost(ctx context.Context, pool *pgxpool.Pool, projectID int64) float64 {
	var raw []byte
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(w.price_rule, '{}'::jsonb)
		FROM workflow_projects p
		JOIN workflow_definitions w ON w.id=p.workflow_id
		WHERE p.id=$1`, projectID).Scan(&raw); err != nil {
		return 0
	}
	rule := map[string]interface{}{}
	_ = json.Unmarshal(raw, &rule)
	if stringAny(rule["billing_type"]) != "per_request" {
		return 0
	}
	return floatAny(rule["unit_price"])
}

func chatUsageTokens(body []byte) (int, int) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, 0
	}
	if usage, ok := raw["usage"].(map[string]interface{}); ok {
		return intAny(firstNonNil(usage["prompt_tokens"], usage["input_tokens"])), intAny(firstNonNil(usage["completion_tokens"], usage["output_tokens"]))
	}
	if usage, ok := raw["usageMetadata"].(map[string]interface{}); ok {
		return intAny(usage["promptTokenCount"]), intAny(firstNonNil(usage["candidatesTokenCount"], usage["responseTokenCount"]))
	}
	return 0, 0
}

func estimateModelCostByCodeWorker(ctx context.Context, pool *pgxpool.Pool, code string, params map[string]interface{}, promptTokens, outputTokens int) float64 {
	var raw []byte
	var category string
	if err := pool.QueryRow(ctx, `SELECT price_rule, category FROM models WHERE code=$1`, code).Scan(&raw, &category); err != nil {
		return 0
	}
	rule := map[string]interface{}{}
	_ = json.Unmarshal(raw, &rule)
	params = workerBillingParams(params, category)
	return estimatePriceRuleCostWorker(rule, params, promptTokens, outputTokens)
}

func estimateModelCostByIDWorker(ctx context.Context, pool *pgxpool.Pool, modelID int64, params map[string]interface{}, promptTokens, outputTokens int) float64 {
	var raw []byte
	var category string
	if err := pool.QueryRow(ctx, `SELECT price_rule, category FROM models WHERE id=$1`, modelID).Scan(&raw, &category); err != nil {
		return 0
	}
	rule := map[string]interface{}{}
	_ = json.Unmarshal(raw, &rule)
	params = workerBillingParams(params, category)
	return estimatePriceRuleCostWorker(rule, params, promptTokens, outputTokens)
}

func workerBillingParams(params map[string]interface{}, category string) map[string]interface{} {
	if category != "audio" {
		return params
	}
	out := make(map[string]interface{}, len(params)+1)
	for key, value := range params {
		out[key] = value
	}
	count := floatAny(params["count"])
	if count <= 0 {
		count = floatAny(params["n"])
	}
	if count <= 0 {
		count = 1
	}
	out["_billing_item_count"] = count
	return out
}

func estimatePriceRuleCostWorker(rule map[string]interface{}, params map[string]interface{}, promptTokens, outputTokens int) float64 {
	switch stringAny(rule["billing_type"]) {
	case "per_image":
		n := floatAny(params["n"])
		if n <= 0 {
			n = floatAny(params["count"])
		}
		if n <= 0 {
			n = 1
		}
		return floatAny(rule["unit_price"]) * n
	case "per_token":
		promptTokens, outputTokens = workerEstimatedTokenCounts(rule, params, promptTokens, outputTokens)
		cost := float64(promptTokens)*tokenPriceWorker(rule, "input_price") + float64(outputTokens)*tokenPriceWorker(rule, "output_price")
		if surcharge := floatAny(rule["surcharge_per_m"]); surcharge > 0 {
			cost += float64(promptTokens+outputTokens) / 1_000_000 * surcharge
		}
		count := floatAny(params["_billing_item_count"])
		if count <= 0 {
			count = 1
		}
		return cost * count
	case "per_second":
		duration := workerDurationSeconds(params)
		n := floatAny(params["count"])
		if n <= 0 {
			n = floatAny(params["n"])
		}
		if n <= 0 {
			n = 1
		}
		return floatAny(rule["unit_price"]) * duration * n
	case "per_request":
		return floatAny(rule["unit_price"])
	case "dynamic":
		return estimateDynamicPriceRuleCostWorker(rule, params)
	default:
		return 0
	}
}

func workerEstimatedTokenCounts(rule, params map[string]interface{}, promptTokens, outputTokens int) (int, int) {
	if promptTokens <= 0 {
		for _, source := range []map[string]interface{}{params, rule} {
			for _, key := range []string{"_estimated_input_tokens", "estimated_input_tokens"} {
				if value := int(math.Ceil(floatAny(source[key]))); value > 0 {
					promptTokens = value
					break
				}
			}
			if promptTokens > 0 {
				break
			}
		}
		if promptTokens <= 0 {
			promptTokens = workerTextTokenEstimate(stringAny(params["prompt"]))
		}
		if promptTokens <= 0 {
			promptTokens = 500
		}
	}
	if outputTokens <= 0 {
		for _, key := range []string{"_estimated_output_tokens", "max_completion_tokens", "max_tokens"} {
			if value := int(math.Ceil(floatAny(params[key]))); value > 0 {
				outputTokens = value
				break
			}
		}
		if outputTokens <= 0 {
			outputTokens = int(math.Ceil(floatAny(rule["estimated_output_tokens"])))
		}
		if outputTokens <= 0 {
			outputTokens = 1000
		}
	}
	return promptTokens, outputTokens
}

func workerTextTokenEstimate(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	weighted := 0.0
	for _, r := range []rune(value) {
		if r <= 127 {
			weighted += 0.25
		} else {
			weighted += 0.75
		}
	}
	return int(math.Ceil(weighted)) + 8
}

func estimateDynamicPriceRuleCostWorker(rule, params map[string]interface{}) float64 {
	switch strings.ToLower(strings.TrimSpace(stringAny(rule["strategy"]))) {
	case "seedance_2_tokens":
		return estimateSeedance2PriceRuleCostWorker(rule, params)
	case "minimax_h3_seconds":
		return estimateMiniMaxH3PriceRuleCostWorker(rule, params)
	default:
		return floatAny(rule["fallback_cost"])
	}
}

func estimateMiniMaxH3PriceRuleCostWorker(rule, params map[string]interface{}) float64 {
	resolution := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringAny(params["resolution"]), stringAny(rule["default_resolution"]), "2k")))
	rate := nestedWorkerFloat(rule["rates_per_second"], resolution, "")
	if rate <= 0 {
		rate = map[string]float64{"2k": 0.8, "768p": 0.5}[resolution]
	}
	if rate <= 0 {
		return floatAny(rule["fallback_cost"])
	}
	outputSeconds := workerDurationSeconds(params)
	videoCount := workerURLFieldCount(params["reference_videos"])
	inputSeconds := floatAny(params["reference_video_duration_seconds"])
	if videoCount > 0 && inputSeconds <= 0 {
		inputSeconds = float64(videoCount) * floatAny(rule["default_input_video_seconds"])
		if inputSeconds <= 0 {
			inputSeconds = float64(videoCount) * 4
		}
	}
	imageCount := workerURLFieldCount(params["reference_images"]) +
		workerURLFieldCount(params["first_frame"]) +
		workerURLFieldCount(params["last_frame"])
	freeImages := int(floatAny(rule["free_reference_images"]))
	if freeImages < 0 {
		freeImages = 0
	}
	excessImages := imageCount - freeImages
	if excessImages < 0 {
		excessImages = 0
	}
	imagePrice := floatAny(rule["excess_image_price"])
	if imagePrice <= 0 {
		imagePrice = 0.2
	}
	multiplier := floatAny(rule["platform_multiplier"])
	if multiplier <= 0 {
		multiplier = 1
	}
	pointsPerCNY := floatAny(rule["points_per_cny"])
	if pointsPerCNY <= 0 {
		pointsPerCNY = 1
	}
	return ((outputSeconds+inputSeconds)*rate + float64(excessImages)*imagePrice) * multiplier * pointsPerCNY
}

func estimateSeedance2PriceRuleCostWorker(rule, params map[string]interface{}) float64 {
	resolution := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringAny(params["resolution"]), stringAny(rule["default_resolution"]), "720p")))
	tokensPerSecond := nestedWorkerFloat(rule["tokens_per_second"], resolution, "")
	if tokensPerSecond <= 0 {
		tokensPerSecond = map[string]float64{"480p": 10044, "720p": 21600, "1080p": 48600, "4k": 194400}[resolution]
	}
	if tokensPerSecond <= 0 {
		tokensPerSecond = 21600
	}
	mode := strings.ToLower(strings.TrimSpace(stringAny(params["generation_mode"])))
	hasVideo := strings.Contains(mode, "video") || workerURLFieldCount(params["reference_videos"]) > 0
	rateKind := "without_video"
	if hasVideo {
		rateKind = "with_video"
	}
	rate := nestedWorkerFloat(rule["rates_per_m_tokens"], resolution, rateKind)
	if rate <= 0 {
		defaultRates := map[string]map[string]float64{
			"480p":  {"without_video": 46, "with_video": 28},
			"720p":  {"without_video": 46, "with_video": 28},
			"1080p": {"without_video": 51, "with_video": 31},
			"4k":    {"without_video": 26, "with_video": 16},
		}
		rate = defaultRates[resolution][rateKind]
	}
	duration := workerDurationSeconds(params)
	tokens := duration * tokensPerSecond
	if hasVideo {
		inputDuration := floatAny(params["reference_video_duration_seconds"])
		if inputDuration <= 0 {
			inputDuration = floatAny(rule["default_input_video_seconds"])
		}
		if inputDuration <= 0 {
			inputDuration = 4
		}
		tokens = (duration + inputDuration) * tokensPerSecond
		minMultiplier := floatAny(rule["video_min_token_multiplier"])
		if minMultiplier <= 0 {
			minMultiplier = 1.8
		}
		if minimum := duration * tokensPerSecond * minMultiplier; tokens < minimum {
			tokens = minimum
		}
	}
	multiplier := floatAny(rule["platform_multiplier"])
	if multiplier <= 0 {
		multiplier = 1
	}
	pointsPerCurrency := floatAny(rule["points_per_cny"])
	if pointsPerCurrency <= 0 {
		pointsPerCurrency = 1
	}
	return tokens / 1_000_000 * rate * multiplier * pointsPerCurrency
}

func nestedWorkerFloat(raw interface{}, first, second string) float64 {
	m, _ := raw.(map[string]interface{})
	if m == nil {
		return 0
	}
	if second == "" {
		return floatAny(m[first])
	}
	child, _ := m[first].(map[string]interface{})
	return floatAny(child[second])
}

func workerDurationSeconds(params map[string]interface{}) float64 {
	for _, key := range []string{"duration", "duration_sec", "seconds"} {
		if value := floatAny(params[key]); value > 0 {
			return value
		}
		if raw := strings.TrimSpace(stringAny(params[key])); raw != "" {
			raw = strings.TrimSuffix(strings.ToLower(raw), "s")
			if value, err := strconv.ParseFloat(raw, 64); err == nil && value > 0 {
				return value
			}
		}
	}
	return 5
}

func tokenPriceWorker(rule map[string]interface{}, key string) float64 {
	if v := floatAny(rule[key]); v > 0 {
		return v
	}
	if v := floatAny(rule[key+"_per_m"]); v > 0 {
		return v / 1_000_000
	}
	return 0
}

func copyLLMExtraParams(extra map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range extra {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "connection", "model", "messages", "input", "prompt", "stream":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

func extractLLMText(body []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}
	if s := stringAny(raw["output_text"]); s != "" {
		return s
	}
	if content, ok := raw["content"].([]interface{}); ok {
		items := make([]string, 0, len(content))
		for _, part := range content {
			if item, ok := part.(map[string]interface{}); ok && (stringAny(item["type"]) == "text" || item["type"] == nil) {
				items = append(items, stringAny(item["text"]))
			}
		}
		if text := strings.TrimSpace(strings.Join(items, "\n")); text != "" {
			return text
		}
	}
	if candidates, ok := raw["candidates"].([]interface{}); ok && len(candidates) > 0 {
		candidate, _ := candidates[0].(map[string]interface{})
		content, _ := candidate["content"].(map[string]interface{})
		parts, _ := content["parts"].([]interface{})
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if item, ok := part.(map[string]interface{}); ok {
				items = append(items, stringAny(item["text"]))
			}
		}
		if text := strings.TrimSpace(strings.Join(items, "\n")); text != "" {
			return text
		}
	}
	if choices, ok := raw["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if s := stringAny(msg["content"]); s != "" {
					return s
				}
				if parts, ok := msg["content"].([]interface{}); ok {
					items := make([]string, 0, len(parts))
					for _, part := range parts {
						if m, ok := part.(map[string]interface{}); ok {
							items = append(items, firstNonEmpty(stringAny(m["text"]), stringAny(m["content"])))
						}
					}
					return strings.TrimSpace(strings.Join(items, "\n"))
				}
			}
			if s := stringAny(choice["text"]); s != "" {
				return s
			}
		}
	}
	if output, ok := raw["output"].([]interface{}); ok {
		items := []string{}
		for _, item := range output {
			m, _ := item.(map[string]interface{})
			content, _ := m["content"].([]interface{})
			for _, part := range content {
				pm, _ := part.(map[string]interface{})
				items = append(items, firstNonEmpty(stringAny(pm["text"]), stringAny(pm["content"])))
			}
		}
		return strings.TrimSpace(strings.Join(items, "\n"))
	}
	return ""
}

func normalizeAgentAnalysisOutput(text, category string) map[string]interface{} {
	out := parseJSONish(text)
	if len(out) == 0 {
		prompt := strings.TrimSpace(text)
		if prompt == "" {
			prompt = "根据用户需求生成高质量电商商品视觉内容。"
		}
		out = map[string]interface{}{
			"summary":        "AI已生成创作方案，请确认后继续生成。",
			"user_intent":    prompt,
			"style":          "商业电商风格",
			"raw_text":       text,
			"candidates":     []map[string]interface{}{{"id": "A", "title": "默认方案", "reason": "模型返回了非JSON内容，已作为可编辑方案保留。", "prompt": prompt, "negative_prompt": "低清晰度、畸变、错别字、水印"}},
			"recommendation": "A",
		}
	}
	candidates := analysisCandidates(out)
	if len(candidates) == 0 {
		prompt := firstNonEmpty(stringAny(out["generation_prompt"]), stringAny(out["summary"]), strings.TrimSpace(text))
		candidates = []map[string]interface{}{{"id": "A", "title": defaultCandidateTitle(category), "reason": "基于分析内容自动整理。", "prompt": prompt, "negative_prompt": "低清晰度、畸变、错别字、水印"}}
		out["candidates"] = candidates
		out["recommendation"] = "A"
	}
	if stringAny(out["recommendation"]) == "" {
		out["recommendation"] = stringAny(candidates[0]["id"])
	}
	if stringAny(out["generation_prompt"]) == "" {
		out["generation_prompt"] = selectedAnalysisPrompt(out, "")
	}
	out["raw_text"] = text
	return out
}

func defaultCandidateTitle(category string) string {
	if category == "video" {
		return "视频创作方案"
	}
	return "图片创作方案"
}

func analysisCandidates(analysis map[string]interface{}) []map[string]interface{} {
	raw, ok := analysis["candidates"].([]interface{})
	if !ok {
		if arr, ok := analysis["candidates"].([]map[string]interface{}); ok {
			return arr
		}
		return nil
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for idx, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			if stringAny(m["id"]) == "" {
				m["id"] = string(rune('A' + idx))
			}
			items = append(items, m)
		}
	}
	return items
}

func selectedAnalysisPrompt(analysis map[string]interface{}, candidateID string) string {
	candidates := analysisCandidates(analysis)
	preferred := firstNonEmpty(candidateID, stringAny(analysis["recommendation"]))
	if preferred != "" {
		for _, item := range candidates {
			if strings.EqualFold(stringAny(item["id"]), preferred) {
				if s := stringAny(item["prompt"]); s != "" {
					return s
				}
			}
		}
	}
	for _, item := range candidates {
		if s := stringAny(item["prompt"]); s != "" {
			return s
		}
	}
	return firstNonEmpty(stringAny(analysis["generation_prompt"]), stringAny(analysis["summary"]), stringAny(analysis["raw_text"]))
}

func mergeAgentGenerationInputs(inputs, analysis map[string]interface{}, candidateID string, confirmed map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range inputs {
		out[k] = v
	}
	candidate := selectedAnalysisCandidate(analysis, candidateID)
	if params, ok := candidate["params"].(map[string]interface{}); ok {
		for k, v := range params {
			if strings.HasPrefix(k, "_") {
				continue
			}
			if !hasMeaningfulInput(out, k) {
				out[k] = v
			}
		}
	}
	if s := stringAny(candidate["negative_prompt"]); s != "" {
		out["negative_prompt"] = s
	}
	if params, ok := confirmed["params"].(map[string]interface{}); ok {
		for k, v := range params {
			if strings.HasPrefix(k, "_") {
				continue
			}
			if !hasMeaningfulInput(out, k) {
				out[k] = v
			}
		}
	}
	if s := stringAny(confirmed["negative_prompt"]); s != "" {
		out["negative_prompt"] = s
	}
	return out
}

func hasMeaningfulInput(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []interface{}:
		return len(t) > 0
	case []string:
		return len(t) > 0
	default:
		return true
	}
}

func selectedAnalysisCandidate(analysis map[string]interface{}, candidateID string) map[string]interface{} {
	candidates := analysisCandidates(analysis)
	preferred := firstNonEmpty(candidateID, stringAny(analysis["recommendation"]))
	if preferred != "" {
		for _, item := range candidates {
			if strings.EqualFold(stringAny(item["id"]), preferred) {
				return item
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return map[string]interface{}{}
}

func runAgentMediaTasks(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, projectID, userID int64, publicID string, runtimeCfg, inputs map[string]interface{}, prompt string) ([]map[string]interface{}, string) {
	modelCode := stringAny(runtimeCfg["generation_model_code"])
	if modelCode == "" {
		return nil, "未配置生成模型"
	}
	genType := firstNonEmpty(stringAny(runtimeCfg["generation_type"]), "image")
	count := intAny(inputs["count"])
	if count <= 0 {
		count = intAny(runtimeCfg["default_count"])
	}
	if count <= 0 {
		count = 1
	}
	if count > 20 {
		count = 20
	}
	var modelID int64
	var requestMode string
	var defaultsRaw, runtimeRaw []byte
	if err := pool.QueryRow(ctx, `SELECT id, request_mode, default_params, runtime_rule FROM models WHERE code=$1 AND is_enabled=true`, modelCode).Scan(&modelID, &requestMode, &defaultsRaw, &runtimeRaw); err != nil {
		return nil, "生成模型不存在：" + modelCode
	}
	defaultParams := map[string]interface{}{}
	runtimeRule := map[string]interface{}{}
	_ = json.Unmarshal(defaultsRaw, &defaultParams)
	_ = json.Unmarshal(runtimeRaw, &runtimeRule)
	taskType := "image"
	if requestMode == "video" || genType == "video" {
		taskType = "video"
	} else if requestMode == "audio" || genType == "audio" {
		taskType = "audio"
	}
	referenceImages := referenceImageURLs(inputs)
	results := make([]map[string]interface{}, 0, count)
	successCount := 0
	firstErr := ""
	for i := 0; i < count; i++ {
		taskNo := newWorkflowTaskNo(i)
		taskInput := agentMediaTaskInput(inputs, prompt, publicID)
		applyAgentModelDefaults(taskInput, defaultParams, runtimeRule, taskType)
		taskInput["count"] = 1
		taskInput["n"] = 1
		if len(referenceImages) > 0 {
			taskInput["reference_images"] = referenceImages
		}
		taskEstimated := estimateModelCostByIDWorker(ctx, pool, modelID, taskInput, 0, 0)
		inputJSON, _ := json.Marshal(taskInput)
		_, err := pool.Exec(ctx, `
			INSERT INTO tasks (task_no, user_id, model_id, type, status, input, estimated_cost)
			VALUES ($1,$2,$3,$4,'pending',$5,$6)`, taskNo, userID, modelID, taskType, inputJSON, taskEstimated)
		if err != nil {
			if firstErr == "" {
				firstErr = err.Error()
			}
			results = append(results, map[string]interface{}{"task_no": taskNo, "status": "failed", "progress": 100, "error_message": err.Error()})
			continue
		}
		appendWorkflowMediaTask(ctx, pool, projectID, map[string]interface{}{"task_no": taskNo, "type": taskType, "status": "pending", "progress": 5, "output": map[string]interface{}{}})
		_ = processImageTask(ctx, pool, baseURL, token, ImageTaskPayload{TaskNo: taskNo, UserID: userID, ModelID: modelID, ModelCode: modelCode, Input: taskInput})
		item := loadAgentMediaTask(ctx, pool, taskNo)
		item["type"] = taskType
		appendWorkflowMediaTask(ctx, pool, projectID, item)
		if stringAny(item["status"]) == "succeeded" {
			successCount++
		} else if firstErr == "" {
			firstErr = firstNonEmpty(stringAny(item["error_message"]), "生成任务失败")
		}
		results = append(results, item)
	}
	if successCount == 0 {
		return results, firstNonEmpty(firstErr, "生成任务全部失败")
	}
	return results, ""
}

func applyAgentModelDefaults(taskInput, defaults, runtimeRule map[string]interface{}, taskType string) {
	videoRule, _ := runtimeRule["video"].(map[string]interface{})
	modeKey := firstNonEmpty(stringAny(videoRule["mode_param"]), "generation_mode")
	explicitMode := stringAny(taskInput[modeKey]) != ""
	for key, value := range defaults {
		if _, exists := taskInput[key]; !exists {
			taskInput[key] = value
		}
	}
	if taskType != "video" {
		return
	}
	if !strings.EqualFold(stringAny(videoRule["upload_profile"]), "seedance_2") {
		return
	}
	if explicitMode {
		return
	}
	parts := make([]string, 0, 3)
	if workerURLFieldCount(taskInput["reference_images"]) > 0 || stringAny(taskInput["image_url"]) != "" {
		parts = append(parts, "image")
	}
	if workerURLFieldCount(taskInput["reference_videos"]) > 0 {
		parts = append(parts, "video")
	}
	if workerURLFieldCount(taskInput["reference_audios"]) > 0 {
		parts = append(parts, "audio")
	}
	if len(parts) == 0 {
		taskInput[modeKey] = "text"
	} else {
		taskInput[modeKey] = strings.Join(parts, "_")
	}
}

func workerURLFieldCount(value interface{}) int {
	switch v := value.(type) {
	case []string:
		return len(v)
	case []interface{}:
		return len(v)
	case string:
		if strings.TrimSpace(v) != "" {
			return 1
		}
	}
	return 0
}

func runAgentDetailPageTasks(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, projectID, userID int64, publicID string, runtimeCfg, inputs, analysis map[string]interface{}, basePrompt string) ([]map[string]interface{}, map[string]interface{}, string) {
	modelCode := stringAny(runtimeCfg["generation_model_code"])
	if modelCode == "" {
		return nil, map[string]interface{}{"status": "failed"}, "未配置生成模型"
	}
	var modelID int64
	var requestMode string
	if err := pool.QueryRow(ctx, "SELECT id, request_mode FROM models WHERE code=$1", modelCode).Scan(&modelID, &requestMode); err != nil {
		return nil, map[string]interface{}{"status": "failed"}, "生成模型不存在：" + modelCode
	}
	if requestMode == "video" || requestMode == "audio" {
		return nil, map[string]interface{}{"status": "failed"}, "商品详情页必须配置图片生成模型"
	}
	sections := agentDetailSections(analysis, inputs, basePrompt)
	results := make([]map[string]interface{}, 0, len(sections))
	completedSections := make([]map[string]interface{}, 0, len(sections))
	imageURLs := make([]string, 0, len(sections))
	successCount := 0
	firstErr := ""
	for i, section := range sections {
		sectionPrompt := detailSectionGenerationPrompt(basePrompt, section, i, len(sections), inputs)
		taskNo := newWorkflowTaskNo(i)
		taskInput := agentMediaTaskInput(inputs, sectionPrompt, publicID)
		taskInput["count"] = 1
		taskInput["n"] = 1
		if imageURL := firstImageURL(inputs); imageURL != "" {
			taskInput["reference_images"] = []string{imageURL}
		}
		taskEstimated := estimateModelCostByIDWorker(ctx, pool, modelID, taskInput, 0, 0)
		inputJSON, _ := json.Marshal(taskInput)
		_, err := pool.Exec(ctx, "INSERT INTO tasks (task_no, user_id, model_id, type, status, input, estimated_cost) VALUES ($1,$2,$3,'image','pending',$4,$5)", taskNo, userID, modelID, inputJSON, taskEstimated)
		if err != nil {
			if firstErr == "" {
				firstErr = err.Error()
			}
			results = append(results, map[string]interface{}{"task_no": taskNo, "status": "failed", "progress": 100, "error_message": err.Error(), "detail_section": section})
			continue
		}
		pending := map[string]interface{}{"task_no": taskNo, "status": "pending", "progress": 5, "output": map[string]interface{}{}, "detail_section": section}
		appendWorkflowMediaTask(ctx, pool, projectID, pending)
		_ = processImageTask(ctx, pool, baseURL, token, ImageTaskPayload{TaskNo: taskNo, UserID: userID, ModelID: modelID, ModelCode: modelCode, Input: taskInput})
		item := loadAgentMediaTask(ctx, pool, taskNo)
		item["detail_section"] = section
		appendWorkflowMediaTask(ctx, pool, projectID, item)
		if stringAny(item["status"]) == "succeeded" {
			successCount++
			out, _ := item["output"].(map[string]interface{})
			imageURL := firstNonEmpty(stringAny(out["image_url"]), firstImageResultURL(out))
			sectionResult := copyMap(section)
			sectionResult["task_no"] = taskNo
			sectionResult["image_url"] = imageURL
			sectionResult["status"] = "succeeded"
			completedSections = append(completedSections, sectionResult)
			if imageURL != "" {
				imageURLs = append(imageURLs, imageURL)
			}
		} else if firstErr == "" {
			firstErr = firstNonEmpty(stringAny(item["error_message"]), "详情模块生成失败")
		}
		results = append(results, item)
	}
	detailPage := map[string]interface{}{
		"status":          "modules_ready",
		"sections":        completedSections,
		"section_count":   len(sections),
		"completed_count": successCount,
	}
	if successCount == 0 {
		detailPage["status"] = "failed"
		return results, detailPage, firstNonEmpty(firstErr, "商品详情模块全部生成失败")
	}
	if len(imageURLs) == successCount && len(imageURLs) > 1 {
		if longURL, err := composeDetailPageLongImage(ctx, publicID, imageURLs); err != nil {
			detailPage["compose_status"] = "skipped"
			detailPage["compose_error"] = err.Error()
			log.Printf("Workflow %s detail page compose skipped: %v", publicID, err)
		} else if longURL != "" {
			detailPage["status"] = "completed"
			detailPage["compose_status"] = "succeeded"
			detailPage["long_image_url"] = longURL
		}
	}
	return results, detailPage, ""
}

func agentDetailSections(analysis, inputs map[string]interface{}, basePrompt string) []map[string]interface{} {
	wanted := intAny(inputs["detail_section_count"])
	if wanted < 4 || wanted > 8 {
		if n := intAny(inputs["count"]); n >= 4 && n <= 8 {
			wanted = n
		} else {
			wanted = 6
		}
	}
	items := []map[string]interface{}{}
	if raw, ok := analysis["detail_sections"].([]interface{}); ok {
		for idx, item := range raw {
			section, _ := item.(map[string]interface{})
			if section == nil || strings.TrimSpace(stringAny(section["image_prompt"])) == "" {
				continue
			}
			next := copyMap(section)
			if stringAny(next["id"]) == "" {
				next["id"] = fmt.Sprintf("detail_%02d", idx+1)
			}
			items = append(items, next)
			if len(items) >= wanted {
				break
			}
		}
	}
	defaults := []map[string]interface{}{
		{"type": "hero", "title": "商品首屏", "objective": "建立商品定位和第一视觉", "copy_title": "核心商品定位", "image_prompt": "详情页首屏视觉，商品居中或黄金分割构图，高级商业光影，背景简洁，预留标题与核心卖点区域"},
		{"type": "benefit", "title": "核心卖点", "objective": "突出用户已提供的主要购买理由", "copy_title": "核心卖点", "image_prompt": "详情页核心卖点模块，商品与功能视觉符号结合，层次清晰，预留三项卖点排版区域"},
		{"type": "material", "title": "材质细节", "objective": "展示结构、材质和工艺", "copy_title": "细节与材质", "image_prompt": "商品局部微距特写，突出材质纹理、结构和工艺细节，商业摄影，预留细节标注区域"},
		{"type": "feature", "title": "功能展示", "objective": "解释商品功能和使用价值", "copy_title": "功能展示", "image_prompt": "商品功能可视化详情模块，清晰表现工作原理或使用价值，简洁信息图式构图但不绘制文字"},
		{"type": "usage", "title": "使用场景", "objective": "建立真实使用情境和购买欲", "copy_title": "使用场景", "image_prompt": "真实高品质使用场景，商品主体外观保持一致，尺度准确，生活方式商业摄影，预留场景说明区域"},
		{"type": "specification", "title": "规格与收尾", "objective": "承载真实规格和购买信息", "copy_title": "规格参数", "image_prompt": "详情页规格收尾模块，商品多角度或包装组合展示，干净背景，大面积规整留白用于后期参数排版，不生成任何参数文字"},
		{"type": "closing", "title": "品牌收尾", "objective": "形成完整详情页结束视觉", "copy_title": "品牌收尾", "image_prompt": "品牌感详情页收尾视觉，商品英雄式展示，统一品牌色和高级光影，预留行动文案区域"},
	}
	for len(items) < wanted {
		next := copyMap(defaults[len(items)%len(defaults)])
		next["id"] = fmt.Sprintf("detail_%02d", len(items)+1)
		if stringAny(next["image_prompt"]) == "" {
			next["image_prompt"] = basePrompt
		}
		items = append(items, next)
	}
	return items
}

func detailSectionGenerationPrompt(basePrompt string, section map[string]interface{}, index, total int, inputs map[string]interface{}) string {
	sectionPrompt := firstNonEmpty(stringAny(section["image_prompt"]), stringAny(section["objective"]), basePrompt)
	prompt := fmt.Sprintf("DETAIL PAGE MODULE %d/%d\n模块类型：%s\n模块标题：%s\n模块目标：%s\n\n%s\n\n全页一致性要求：严格保持参考商品的外观、颜色、材质、包装、Logo位置和比例一致；本模块只生成视觉底图和排版留白，不绘制任何标题、参数、促销文字或虚构认证；与其他模块使用统一品牌色、光线和商业风格。\n基础商品方案：%s", index+1, total, stringAny(section["type"]), stringAny(section["title"]), stringAny(section["objective"]), sectionPrompt, basePrompt)
	return agentPromptWithScene(prompt, inputs)
}

func firstImageResultURL(out map[string]interface{}) string {
	if raw, ok := out["images"].([]interface{}); ok && len(raw) > 0 {
		if item, ok := raw[0].(map[string]interface{}); ok {
			return stringAny(item["url"])
		}
	}
	return ""
}

func composeDetailPageLongImage(ctx context.Context, publicID string, urls []string) (string, error) {
	if objectStore == nil {
		return "", errors.New("对象存储未初始化")
	}
	images := make([]image.Image, 0, len(urls))
	maxWidth := 0
	totalHeight := 0
	const gap = 16
	for _, mediaURL := range urls {
		data, _, err := loadMediaBytes(ctx, mediaURL)
		if err != nil {
			return "", err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("图片格式暂不支持自动拼接: %w", err)
		}
		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 || bounds.Dx() > 4096 || bounds.Dy() > 8192 {
			return "", errors.New("详情模块尺寸超出安全范围")
		}
		images = append(images, img)
		if bounds.Dx() > maxWidth {
			maxWidth = bounds.Dx()
		}
		totalHeight += bounds.Dy()
	}
	if len(images) < 2 {
		return "", errors.New("可拼接模块不足")
	}
	totalHeight += gap * (len(images) - 1)
	if int64(maxWidth)*int64(totalHeight) > 80_000_000 {
		return "", errors.New("详情长图像素超过安全上限")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, maxWidth, totalHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	y := 0
	for _, img := range images {
		bounds := img.Bounds()
		x := (maxWidth - bounds.Dx()) / 2
		target := image.Rect(x, y, x+bounds.Dx(), y+bounds.Dy())
		draw.Draw(canvas, target, img, bounds.Min, draw.Src)
		y += bounds.Dy() + gap
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	objectName := fmt.Sprintf("workflows/%s/detail-page-%d.jpg", publicID, time.Now().UnixNano())
	return objectStore.Upload(ctx, objectName, "image/jpeg", bytes.NewReader(encoded.Bytes()), int64(encoded.Len()))
}

func completeSimpleAgentWorkflow(ctx context.Context, pool *pgxpool.Pool, p WorkflowTaskPayload, publicID string, estimated float64, outputs map[string]interface{}) error {
	saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
	actual := workflowActualCost(ctx, pool, p.ProjectID, outputs)
	chargeCost := incrementalWorkflowCharge(ctx, pool, p.ProjectID, actual)
	if err := chargeBillingWithFinalize(ctx, pool, p.UserID, estimated, chargeCost, "workflow", publicID, "workflow_usage", "智能体工作流", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE workflow_projects SET status='succeeded', outputs=$1, actual_cost=$2, error_message=NULL, finished_at=now(), updated_at=now() WHERE id=$3 AND status='running'`,
			mustJSON(outputs), actual, p.ProjectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("workflow is no longer running")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("workflow %s billing/finalize: %w", publicID, err)
	}
	log.Printf("Workflow project %s completed (cost=%.4f)", publicID, actual)
	return nil
}

func runNode(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, userID int64, publicID string, category string, node workflowNode, prompt string, inputs map[string]interface{}) (map[string]interface{}, string) {
	switch node.Type {
	case "llm":
		if strings.TrimSpace(prompt) == "" {
			out, errMsg := runAgentAnalysis(ctx, pool, baseURL, token, node.ModelCode, category, map[string]interface{}{}, inputs)
			if errMsg != "" {
				return nil, errMsg
			}
			if stringAny(out["text"]) == "" {
				out["text"] = firstNonEmpty(stringAny(out["generation_prompt"]), stringAny(out["summary"]), stringAny(out["raw_text"]))
			}
			return out, ""
		}
		body, _ := json.Marshal(map[string]interface{}{
			"model":    node.ModelCode,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
		})
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if !postJSON(ctx, baseURL+"/v1/chat/completions", token, body, &result) {
			return nil, "模型服务异常"
		}
		text := ""
		if len(result.Choices) > 0 {
			text = result.Choices[0].Message.Content
		}
		return map[string]interface{}{"text": text}, ""
	case "image":
		return runMediaNode(ctx, pool, baseURL, token, userID, publicID, node, prompt, inputs, "image")
	case "video":
		return runMediaNode(ctx, pool, baseURL, token, userID, publicID, node, prompt, inputs, "video")
	default:
		return nil, "未知节点类型"
	}
}

func runMediaNode(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, userID int64, publicID string, node workflowNode, prompt string, inputs map[string]interface{}, fallbackType string) (map[string]interface{}, string) {
	modelCode := strings.TrimSpace(node.ModelCode)
	if modelCode == "" {
		return nil, "未配置生成模型"
	}
	var modelID int64
	var requestMode string
	if err := pool.QueryRow(ctx, `SELECT id, request_mode FROM models WHERE code=$1`, modelCode).Scan(&modelID, &requestMode); err != nil {
		return nil, "生成模型不存在：" + modelCode
	}
	taskType := fallbackType
	if requestMode == "video" || requestMode == "audio" || requestMode == "image" {
		taskType = requestMode
	}
	taskInput := agentMediaTaskInput(inputs, prompt, publicID)
	taskNo := newWorkflowTaskNo(0)
	taskEstimated := estimateModelCostByIDWorker(ctx, pool, modelID, taskInput, 0, 0)
	inputJSON, _ := json.Marshal(taskInput)
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (task_no, user_id, model_id, type, status, input, estimated_cost)
		VALUES ($1,$2,$3,$4,'pending',$5,$6)`, taskNo, userID, modelID, taskType, inputJSON, taskEstimated); err != nil {
		return nil, err.Error()
	}
	_ = processImageTask(ctx, pool, baseURL, token, ImageTaskPayload{TaskNo: taskNo, UserID: userID, ModelID: modelID, ModelCode: modelCode, Input: taskInput})
	item := loadAgentMediaTask(ctx, pool, taskNo)
	if stringAny(item["status"]) != "succeeded" {
		if msg := stringAny(item["error_message"]); msg != "" {
			return nil, msg
		}
		return nil, "生成任务失败"
	}
	out, _ := item["output"].(map[string]interface{})
	if out == nil {
		return nil, "生成完成但未返回结果"
	}
	out["_task_no"] = taskNo
	return out, ""
}

func agentMediaTaskInput(inputs map[string]interface{}, prompt, publicID string) map[string]interface{} {
	taskInput := map[string]interface{}{}
	for k, v := range inputs {
		if strings.HasPrefix(k, "_") || k == "prompt" || k == "product" || k == "input" || k == "description" || k == "requirement" {
			continue
		}
		taskInput[k] = v
	}
	taskInput["prompt"] = prompt
	taskInput["_skip_billing"] = true
	taskInput["_workflow_project"] = publicID
	if _, ok := taskInput["count"]; !ok {
		taskInput["count"] = 1
	}
	if _, ok := taskInput["n"]; !ok {
		taskInput["n"] = taskInput["count"]
	}
	imageURL := firstImageURL(inputs)
	if imageURL != "" {
		if _, ok := taskInput["reference_images"]; !ok {
			taskInput["reference_images"] = []string{imageURL}
		}
		if _, ok := taskInput["image_url"]; !ok {
			taskInput["image_url"] = imageURL
		}
	}
	return taskInput
}

func appendWorkflowMediaTask(ctx context.Context, pool *pgxpool.Pool, projectID int64, item map[string]interface{}) {
	if stringAny(item["task_no"]) == "" {
		return
	}
	outputs := loadWorkflowOutputs(ctx, pool, projectID)
	raw, _ := outputs["media_tasks"].([]interface{})
	next := make([]interface{}, 0, len(raw)+1)
	replaced := false
	for _, existing := range raw {
		m, _ := existing.(map[string]interface{})
		if stringAny(m["task_no"]) == stringAny(item["task_no"]) {
			next = append(next, item)
			replaced = true
		} else {
			next = append(next, existing)
		}
	}
	if !replaced {
		next = append(next, item)
	}
	outputs["media_tasks"] = next
	outputs["current_step"] = "generate"
	saveWorkflowOutputs(ctx, pool, projectID, outputs)
}

func postJSON(ctx context.Context, url, token string, body []byte, out interface{}) bool {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, jsonReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}

func renderTemplate(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func insertWorkflowNodeRun(ctx context.Context, pool *pgxpool.Pool, projectID int64, nodeID, name, typ string, input map[string]interface{}, seq int) int64 {
	var nodeRunID int64
	pool.QueryRow(ctx, `
		INSERT INTO workflow_node_runs (project_id, node_id, name, type, status, input, seq)
		VALUES ($1,$2,$3,$4,'running',$5,$6) RETURNING id`,
		projectID, nodeID, name, typ, mustJSON(input), seq).Scan(&nodeRunID)
	return nodeRunID
}

func updateNodeRunSuccess(ctx context.Context, pool *pgxpool.Pool, nodeRunID int64, output map[string]interface{}, cost float64, duration int) {
	pool.Exec(ctx, `UPDATE workflow_node_runs SET status='succeeded', output=$1, cost=$2, duration_ms=$3 WHERE id=$4`,
		mustJSON(output), cost, duration, nodeRunID)
}

func loadWorkflowOutputs(ctx context.Context, pool *pgxpool.Pool, projectID int64) map[string]interface{} {
	var raw []byte
	out := map[string]interface{}{}
	if err := pool.QueryRow(ctx, `SELECT outputs FROM workflow_projects WHERE id=$1`, projectID).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out
}

func saveWorkflowOutputs(ctx context.Context, pool *pgxpool.Pool, projectID int64, outputs map[string]interface{}) {
	pool.Exec(ctx, `UPDATE workflow_projects SET outputs=$1, updated_at=now() WHERE id=$2`, mustJSON(outputs), projectID)
}

func loadAgentMediaTask(ctx context.Context, pool *pgxpool.Pool, taskNo string) map[string]interface{} {
	var status string
	var outputRaw []byte
	var errMsg *string
	var estimatedCost, actualCost float64
	if err := pool.QueryRow(ctx, `SELECT status, output, error_message, estimated_cost, actual_cost FROM tasks WHERE task_no=$1`, taskNo).Scan(&status, &outputRaw, &errMsg, &estimatedCost, &actualCost); err != nil {
		return map[string]interface{}{"task_no": taskNo, "status": "failed", "progress": 100, "error_message": err.Error()}
	}
	output := map[string]interface{}{}
	_ = json.Unmarshal(outputRaw, &output)
	progress := latestTaskEventProgress(ctx, pool, taskNo, status)
	if status == "succeeded" || status == "failed" {
		progress = 100
	}
	item := map[string]interface{}{"task_no": taskNo, "status": status, "progress": progress, "output": output, "estimated_cost": estimatedCost, "actual_cost": actualCost}
	if errMsg != nil && *errMsg != "" {
		item["error_message"] = *errMsg
	}
	return item
}

func latestTaskEventProgress(ctx context.Context, pool *pgxpool.Pool, taskNo, status string) int {
	var progress int
	err := pool.QueryRow(ctx, `
		SELECT COALESCE((payload->>'progress')::int, 0)
		FROM task_events e
		JOIN tasks t ON t.id=e.task_id
		WHERE t.task_no=$1 AND e.event_type='progress'
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT 1`, taskNo).Scan(&progress)
	if err == nil && progress > 0 {
		if progress > 99 {
			return 99
		}
		return progress
	}
	if status == "running" || status == "processing" || status == "in_progress" {
		return 25
	}
	return 8
}

func parseJSONish(text string) map[string]interface{} {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	out := map[string]interface{}{}
	if json.Unmarshal([]byte(text), &out) == nil {
		return out
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		_ = json.Unmarshal([]byte(text[start:end+1]), &out)
	}
	return out
}

func firstUserPrompt(inputs map[string]interface{}) string {
	for _, key := range []string{"prompt", "product", "input", "description", "requirement"} {
		if s := stringAny(inputs[key]); s != "" {
			return s
		}
	}
	return ""
}

func firstImageURL(inputs map[string]interface{}) string {
	items := referenceImageURLs(inputs)
	if len(items) > 0 {
		return items[0]
	}
	return ""
}

func referenceImageURLs(inputs map[string]interface{}) []string {
	seen := map[string]bool{}
	items := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if isSupportedMediaReference(value) && !seen[value] {
			seen[value] = true
			items = append(items, value)
		}
	}
	for _, key := range []string{"image_url", "product_image", "reference_image"} {
		add(stringAny(inputs[key]))
	}
	if style := mapAnyOr(inputs["comic_style"], map[string]interface{}{}); style != nil {
		add(stringAny(style["cover_url"]))
	}
	for _, key := range []string{"reference_images", "images"} {
		switch v := inputs[key].(type) {
		case []interface{}:
			for _, item := range v {
				add(stringAny(item))
			}
		case []string:
			for _, item := range v {
				add(item)
			}
		}
	}
	return items
}

func hasSubjectReferenceImage(inputs map[string]interface{}) bool {
	for _, key := range []string{"image_url", "product_image", "reference_image", "first_frame", "last_frame"} {
		if isSupportedMediaReference(stringAny(inputs[key])) {
			return true
		}
	}
	for _, key := range []string{"reference_images", "images"} {
		switch values := inputs[key].(type) {
		case []interface{}:
			for _, value := range values {
				if isSupportedMediaReference(stringAny(value)) {
					return true
				}
			}
		case []string:
			for _, value := range values {
				if isSupportedMediaReference(value) {
					return true
				}
			}
		}
	}
	return false
}

func comicAssetReferenceURLs(inputs map[string]interface{}) []string {
	seen := map[string]bool{}
	items := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if isSupportedMediaReference(value) && !seen[value] {
			seen[value] = true
			items = append(items, value)
		}
	}
	for _, rawAsset := range comicCollection(inputs["comic_assets"]) {
		asset, _ := rawAsset.(map[string]interface{})
		if asset == nil {
			continue
		}
		metadata := mapAnyOr(asset["metadata"], map[string]interface{}{})
		for _, key := range []string{"reference_urls", "reference_images"} {
			switch values := metadata[key].(type) {
			case []interface{}:
				for _, value := range values {
					add(stringAny(value))
				}
			case []string:
				for _, value := range values {
					add(value)
				}
			}
		}
	}
	return items
}

func isSupportedMediaReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "asset://")
}

func isRetryableComicMediaError(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return true
	}
	for _, marker := range []string{
		"invalid_request", "invalid parameter", "parameter content[",
		"input is required", "unsupported", "unauthorized", "forbidden",
		"permission denied", "model not found", "未匹配到任何通道",
		"参数错误", "参数无效", "缺少必填",
	} {
		if strings.Contains(message, marker) {
			return false
		}
	}
	return true
}

func comicStylePrompt(inputs map[string]interface{}, prompt string) string {
	style := mapAnyOr(inputs["comic_style"], map[string]interface{}{})
	stylePrompt := stringAny(style["prompt"])
	if stylePrompt == "" {
		return prompt
	}
	return fmt.Sprintf("STYLE HARD REQUIREMENT: %s\nKeep the same character identity, props, palette, line work and lighting across every shot.\n\n%s", stylePrompt, prompt)
}

func comicIdentityPrompt(inputs map[string]interface{}, prompt string) string {
	references := referenceImageURLs(inputs)
	for _, assetURL := range comicAssetReferenceURLs(inputs) {
		references = appendUniqueMediaReference(references, assetURL)
	}
	if len(references) == 0 {
		return prompt
	}
	return "CHARACTER IDENTITY HARD REQUIREMENT: Reference image 1 is the immutable identity source for the main character. Preserve the same facial geometry, eyes, hairstyle, age, body proportions and distinctive clothing details in every shot. Do not redesign, beautify, gender-swap or replace the referenced person. Other references and previous keyframes are continuity aids only.\n\n" + prompt
}

func appendUniqueMediaReference(items []string, value string) []string {
	if !isSupportedMediaReference(value) {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func persistComicDramaPlan(ctx context.Context, pool *pgxpool.Pool, workflowProjectID, userID int64, inputs, plan map[string]interface{}) {
	projectPublicID := stringAny(inputs["comic_project_id"])
	if projectPublicID == "" {
		return
	}
	var projectID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM comic_drama_projects WHERE public_id=$1 AND user_id=$2`, projectPublicID, userID).Scan(&projectID); err != nil {
		return
	}
	for _, assetType := range []string{"character", "prop", "location"} {
		key := assetType + "s"
		for idx, raw := range comicCollection(plan[key]) {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			code := firstNonEmpty(stringAny(item["code"]), fmt.Sprintf("%s_%02d", strings.ToUpper(assetType), idx+1))
			name := firstNonEmpty(stringAny(item["name"]), code)
			referenceAssetIDs := item["reference_asset_ids"]
			if assetType == "character" && idx == 0 && workerURLFieldCount(referenceAssetIDs) == 0 {
				referenceAssetIDs = inputs["reference_asset_ids"]
			}
			digest := sha256.Sum256([]byte(assetType + ":" + code))
			publicID := fmt.Sprintf("cda_%d_%x", projectID, digest[:6])
			_, _ = pool.Exec(ctx, `INSERT INTO comic_drama_assets
				(public_id, project_id, asset_type, asset_code, name, description, visual_prompt, reference_asset_ids, metadata, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())
				ON CONFLICT (project_id, asset_type, asset_code) DO UPDATE SET
				name=EXCLUDED.name, description=EXCLUDED.description, visual_prompt=EXCLUDED.visual_prompt,
				reference_asset_ids=EXCLUDED.reference_asset_ids, metadata=EXCLUDED.metadata,
				version=comic_drama_assets.version+1, updated_at=now()`,
				publicID, projectID, assetType, code, name, stringAny(item["description"]), stringAny(item["visual_prompt"]), mustJSON(referenceAssetIDs), mustJSON(item))
		}
	}
	for idx, raw := range comicCollection(plan["storyboards"]) {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		shotID := firstNonEmpty(stringAny(item["id"]), fmt.Sprintf("S%02d", idx+1))
		duration := floatAny(item["duration_sec"])
		if duration <= 0 {
			duration = 5
		}
		_, _ = pool.Exec(ctx, `INSERT INTO comic_drama_storyboards
			(project_id, workflow_project_id, shot_id, seq, title, duration_sec, character_codes, prop_codes, location_code, data, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
			ON CONFLICT (workflow_project_id, shot_id) DO UPDATE SET
			seq=EXCLUDED.seq, title=EXCLUDED.title, duration_sec=EXCLUDED.duration_sec,
			character_codes=EXCLUDED.character_codes, prop_codes=EXCLUDED.prop_codes,
			location_code=EXCLUDED.location_code, data=EXCLUDED.data, updated_at=now()`,
			projectID, workflowProjectID, shotID, idx, stringAny(item["title"]), duration,
			mustJSON(item["character_codes"]), mustJSON(item["prop_codes"]), stringAny(item["location_code"]), mustJSON(item))
	}
}

func comicCollection(value interface{}) []interface{} {
	switch items := value.(type) {
	case []interface{}:
		return items
	case []map[string]interface{}:
		out := make([]interface{}, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	case []string:
		out := make([]interface{}, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	default:
		return nil
	}
}

func newWorkflowTaskNo(i int) string {
	return fmt.Sprintf("task_%d_wf%02d", time.Now().UnixNano(), i+1)
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func mapAny(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok && m != nil
}

func mapAnyOr(v interface{}, fallback map[string]interface{}) map[string]interface{} {
	if m, ok := mapAny(v); ok {
		return m
	}
	return fallback
}

func copyMap(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapSliceToInterfaces(items []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func outputsInterfaceSlice(v interface{}) []interface{} {
	switch items := v.(type) {
	case []interface{}:
		return append([]interface{}{}, items...)
	case []map[string]interface{}:
		return mapSliceToInterfaces(items)
	default:
		return []interface{}{}
	}
}

func firstMapOrNil(items []map[string]interface{}) map[string]interface{} {
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

func firstComicDialogueModel(runtimeCfg map[string]interface{}) string {
	switch raw := runtimeCfg["dialogue_model_codes"].(type) {
	case []interface{}:
		for _, item := range raw {
			if s := stringAny(item); s != "" {
				return s
			}
		}
	case []string:
		for _, item := range raw {
			if strings.TrimSpace(item) != "" {
				return strings.TrimSpace(item)
			}
		}
	}
	return ""
}

func comicDialogueModelCandidates(inputs, runtimeCfg map[string]interface{}) []string {
	result := []string{}
	seen := map[string]bool{}
	appendCodes := func(value interface{}) {
		var codes []string
		switch raw := value.(type) {
		case []interface{}:
			for _, item := range raw {
				codes = append(codes, stringAny(item))
			}
		case []string:
			codes = append(codes, raw...)
		case string:
			codes = append(codes, strings.Split(raw, ",")...)
		}
		for _, code := range codes {
			code = strings.TrimSpace(code)
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			result = append(result, code)
		}
	}
	appendCodes(inputs["dialogue_model_codes"])
	appendCodes(runtimeCfg["dialogue_model_codes"])
	appendCodes(runtimeCfg["analysis_model_code"])
	if len(result) == 0 {
		result = append(result, "chat_demo_v1")
	}
	return result
}

func comicStoryboardGrid(runtimeCfg, inputs map[string]interface{}) int {
	grid := intAny(inputs["storyboard_grid"])
	if grid <= 0 {
		grid = intAny(runtimeCfg["storyboard_grid"])
	}
	switch grid {
	case 4, 6, 9:
		return grid
	default:
		return 6
	}
}

func comicStoryboards(plan, runtimeCfg map[string]interface{}) []map[string]interface{} {
	raw := comicCollection(plan["storyboards"])
	if len(raw) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for idx, item := range raw {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		if stringAny(m["id"]) == "" {
			m["id"] = fmt.Sprintf("S%02d", idx+1)
		}
		if stringAny(m["keyframe_prompt"]) == "" {
			m["keyframe_prompt"] = firstNonEmpty(stringAny(m["scene"]), stringAny(m["title"])) + "，AI 漫剧关键帧，角色一致，画风统一"
		}
		if stringAny(m["video_prompt"]) == "" {
			m["video_prompt"] = firstNonEmpty(stringAny(m["scene"]), stringAny(m["title"])) + "，AI 漫剧视频片段，镜头自然，角色一致"
		}
		assetContext := comicStoryboardAssetContext(plan, m)
		if assetContext != "" {
			m["keyframe_prompt"] = appendComicAssetContext(stringAny(m["keyframe_prompt"]), assetContext)
			m["video_prompt"] = appendComicAssetContext(stringAny(m["video_prompt"]), assetContext)
		}
		out = append(out, m)
	}
	return out
}

func appendComicAssetContext(prompt, assetContext string) string {
	const marker = "\nCONSISTENCY ASSETS:\n"
	if assetContext == "" || strings.Contains(prompt, marker) {
		return prompt
	}
	return prompt + marker + assetContext
}

func comicStoryboardAssetContext(plan, storyboard map[string]interface{}) string {
	typeByCode := map[string]string{}
	for _, key := range []string{"characters", "props", "locations"} {
		for _, raw := range comicCollection(plan[key]) {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			code := stringAny(item["code"])
			if code != "" {
				typeByCode[code] = fmt.Sprintf("%s: %s", code, firstNonEmpty(stringAny(item["visual_prompt"]), stringAny(item["description"]), stringAny(item["name"])))
			}
		}
	}
	codes := []string{}
	for _, key := range []string{"character_codes", "prop_codes"} {
		for _, raw := range comicCollection(storyboard[key]) {
			if code := stringAny(raw); code != "" {
				codes = append(codes, code)
			}
		}
	}
	if code := stringAny(storyboard["location_code"]); code != "" {
		codes = append(codes, code)
	}
	lines := []string{}
	for _, code := range codes {
		if line := typeByCode[code]; line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func comicDefaultScores(runtimeCfg map[string]interface{}) map[string]interface{} {
	asset := intAny(runtimeCfg["asset_consistency_score"])
	if asset <= 0 {
		asset = 80
	}
	logic := intAny(runtimeCfg["logic_score"])
	if logic <= 0 {
		logic = 50
	}
	return map[string]interface{}{"asset_consistency": asset, "logic": logic}
}

func comicPassScores(runtimeCfg, inputs map[string]interface{}) map[string]interface{} {
	thresholds := comicDefaultScores(runtimeCfg)
	asset := intAny(firstNonNil(inputs["asset_consistency_score"], thresholds["asset_consistency"]))
	logic := intAny(firstNonNil(inputs["logic_score"], thresholds["logic"]))
	if asset <= 0 {
		asset = 80
	}
	if logic <= 0 {
		logic = 50
	}
	return map[string]interface{}{
		"threshold_asset": asset,
		"threshold_logic": logic,
		"checked":         false,
		"status":          "quality_model_not_configured",
	}
}

func stringAny(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		return strconv.Itoa(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func boolAny(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || strings.TrimSpace(t) == "1"
	default:
		return false
	}
}

func intAny(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func floatAny(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

func firstNonNil(values ...interface{}) interface{} {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func failWorkflow(ctx context.Context, pool *pgxpool.Pool, p WorkflowTaskPayload, publicID string, estimated float64, msg string) error {
	actual := workflowAccruedCost(ctx, pool, p.ProjectID)
	chargeCost := incrementalWorkflowCharge(ctx, pool, p.ProjectID, actual)
	finalize := func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE workflow_projects SET status='failed', actual_cost=$1, error_message=$2, finished_at=now(), updated_at=now()
			WHERE id=$3 AND status IN ('pending','running')`, actual, msg, p.ProjectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("workflow is no longer active")
		}
		return nil
	}
	var err error
	if chargeCost > 0 {
		err = chargeBillingWithFinalize(ctx, pool, p.UserID, estimated, chargeCost, "workflow", publicID, "workflow_usage", "工作流失败前已完成步骤", finalize)
	} else {
		err = unfreezeBillingWithFinalize(ctx, pool, p.UserID, estimated, "workflow", publicID, finalize)
	}
	if err != nil {
		return fmt.Errorf("workflow %s release billing: %w", publicID, err)
	}
	log.Printf("Workflow project %s failed: %s", publicID, msg)
	return nil
}
