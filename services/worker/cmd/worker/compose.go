package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type composeSource struct {
	Kind string
	URL  string
	Path string
}

func ffmpegBinaryPath() (string, error) {
	configured := strings.TrimSpace(os.Getenv("FFMPEG_PATH"))
	if configured != "" {
		info, err := os.Stat(configured)
		if err == nil && info.IsDir() {
			configured = filepath.Join(configured, ffmpegExecutableName())
			info, err = os.Stat(configured)
		}
		if err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("FFMPEG_PATH 指向的文件不存在：%s", configured)
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("未找到 ffmpeg；本地请重新执行一键启动，或在 .env.local 设置 FFMPEG_PATH")
	}
	return path, nil
}

func ffmpegExecutableName() string {
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func processComposeTask(ctx context.Context, pool *pgxpool.Pool, p ComposeTaskPayload) error {
	pool.Exec(ctx, `UPDATE tasks SET status='running', started_at=now(), updated_at=now() WHERE task_no=$1`, p.TaskNo)
	fail := func(code, message string) error {
		return failTask(ctx, pool, ImageTaskPayload{TaskNo: p.TaskNo, UserID: p.UserID}, code, message)
	}
	if objectStore == nil {
		return fail("STORAGE_UNAVAILABLE", "对象存储未配置，无法保存合成结果")
	}
	_, err := ffmpegBinaryPath()
	if err != nil {
		return fail("FFMPEG_UNAVAILABLE", err.Error())
	}
	rawSources, _ := p.Input["sources"].([]interface{})
	if len(rawSources) == 0 {
		return fail("INVALID_INPUT", "没有可合成的媒体素材")
	}
	tmpDir, err := os.MkdirTemp("", "starai-canvas-compose-*")
	if err != nil {
		return fail("COMPOSE_ERROR", "创建合成工作目录失败")
	}
	defer os.RemoveAll(tmpDir)

	sources := make([]composeSource, 0, len(rawSources))
	for index, raw := range rawSources {
		item, _ := raw.(map[string]interface{})
		kind := strings.ToLower(strings.TrimSpace(stringAny(item["kind"])))
		mediaURL := strings.TrimSpace(stringAny(item["url"]))
		if mediaURL == "" || (kind != "image" && kind != "video" && kind != "audio") {
			continue
		}
		data, contentType, downloadErr := downloadAuthenticatedMedia(ctx, connectionConfig{}, mediaURL, 600<<20)
		if downloadErr != nil {
			return fail("DOWNLOAD_FAILED", fmt.Sprintf("下载第 %d 个合成素材失败：%s", index+1, downloadErr.Error()))
		}
		extension := mediaExtForContentType(contentType, kind)
		if extension == "" {
			extension = map[string]string{"image": ".png", "video": ".mp4", "audio": ".mp3"}[kind]
		}
		path := filepath.Join(tmpDir, fmt.Sprintf("source_%03d%s", index+1, extension))
		if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
			return fail("COMPOSE_ERROR", "写入合成素材失败")
		}
		sources = append(sources, composeSource{Kind: kind, URL: mediaURL, Path: path})
	}
	if len(sources) == 0 {
		return fail("INVALID_INPUT", "没有可合成的有效媒体素材")
	}

	outputPath, outputKind, contentType, composeErr := composeCanvasMedia(
		ctx,
		tmpDir,
		sources,
		stringAny(p.Input["mode"]),
		stringAny(p.Input["output_size"]),
	)
	if composeErr != nil {
		return fail("COMPOSE_ERROR", composeErr.Error())
	}
	outputData, err := os.ReadFile(outputPath)
	if err != nil || len(outputData) == 0 {
		return fail("COMPOSE_ERROR", "读取合成结果失败")
	}
	extension := filepath.Ext(outputPath)
	objectName := fmt.Sprintf("works/compose/%s/result_%d%s", p.TaskNo, time.Now().UnixNano(), extension)
	publicURL, err := objectStore.Upload(ctx, objectName, contentType, bytes.NewReader(outputData), int64(len(outputData)))
	if err != nil {
		return fail("UPLOAD_FAILED", "上传合成结果失败："+err.Error())
	}

	outputKey := map[string]string{"image": "image_url", "video": "video_url", "audio": "audio_url"}[outputKind]
	outputMap := map[string]interface{}{outputKey: publicURL, "url": publicURL, "media_kind": outputKind, "source_count": len(sources)}
	outputJSON, _ := json.Marshal(outputMap)
	var taskID int64
	pool.QueryRow(ctx, `SELECT id FROM tasks WHERE task_no=$1`, p.TaskNo).Scan(&taskID)
	if _, err := pool.Exec(ctx, `
		UPDATE tasks SET status='succeeded', output=$1, actual_cost=0, error_code=NULL, error_message=NULL,
			finished_at=now(), updated_at=now() WHERE task_no=$2`, outputJSON, p.TaskNo); err != nil {
		return err
	}
	publicID := fmt.Sprintf("work_%d", time.Now().UnixNano())
	meta, _ := json.Marshal(outputMap)
	expires := configuredWorkExpiration(ctx, pool, 7)
	pool.Exec(ctx, `
		INSERT INTO works (public_id, user_id, task_id, model_id, type, prompt, thumbnail_url, metadata, expires_at)
		VALUES ($1,$2,$3,NULL,$4,$5,$6,$7,$8)`,
		publicID, p.UserID, taskID, outputKind, "无限画布媒体合成", publicURL, meta, expires)
	pool.Exec(ctx, `INSERT INTO task_events (task_id, event_type, payload) VALUES ($1,'completed',$2)`, taskID, outputJSON)
	insertNotification(ctx, pool, p.UserID, "合成完成", fmt.Sprintf("您的媒体合成任务已完成，任务号：%s", p.TaskNo), "task")
	return nil
}

func composeCanvasMedia(ctx context.Context, tmpDir string, sources []composeSource, mode, outputSize string) (string, string, string, error) {
	var videos, audios, images []composeSource
	for _, source := range sources {
		switch source.Kind {
		case "video":
			videos = append(videos, source)
		case "audio":
			audios = append(audios, source)
		case "image":
			images = append(images, source)
		}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "concat":
		kindCount := 0
		for _, count := range []int{len(images), len(videos), len(audios)} {
			if count > 0 {
				kindCount++
			}
		}
		if kindCount != 1 || len(sources) < 2 {
			return "", "", "", fmt.Errorf("顺序拼接需要至少两个同类型素材")
		}
	case "mux":
		if len(videos) == 0 || len(audios) != 1 || len(images) > 0 {
			return "", "", "", fmt.Errorf("音视频合成需要至少一个视频和一个音频，且仅支持一条音轨")
		}
	case "auto":
		if len(images) > 0 && (len(videos) > 0 || len(audios) > 0) {
			return "", "", "", fmt.Errorf("图片不能与视频或音频直接自动合成，请先将图片生成视频")
		}
	default:
		return "", "", "", fmt.Errorf("不支持的合成方式")
	}
	width, height := composeOutputDimensions(outputSize)
	if len(videos) > 0 {
		if width == 0 || height == 0 {
			width, height = probeMediaDimensions(ctx, videos[0].Path)
		}
		if width <= 0 || height <= 0 {
			width, height = 1920, 1080
		}
		preserveSourceAudio := len(audios) == 0
		if preserveSourceAudio {
			for _, source := range videos {
				if !mediaHasAudio(ctx, source.Path) {
					preserveSourceAudio = false
					break
				}
			}
		}
		normalized := make([]string, 0, len(videos))
		for index, source := range videos {
			path := filepath.Join(tmpDir, fmt.Sprintf("normalized_%03d.mp4", index+1))
			filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1", width, height, width, height)
			args := []string{"-y", "-i", source.Path, "-vf", filter, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-r", "30"}
			if preserveSourceAudio {
				args = append(args, "-c:a", "aac", "-ar", "48000", "-ac", "2")
			} else {
				args = append(args, "-an")
			}
			args = append(args, path)
			if err := runFFmpeg(ctx, args...); err != nil {
				return "", "", "", fmt.Errorf("视频标准化失败：%w", err)
			}
			normalized = append(normalized, path)
		}
		listPath := filepath.Join(tmpDir, "videos.txt")
		var list strings.Builder
		for _, path := range normalized {
			list.WriteString("file '")
			list.WriteString(strings.ReplaceAll(path, "'", "'\\''"))
			list.WriteString("'\n")
		}
		if err := os.WriteFile(listPath, []byte(list.String()), 0600); err != nil {
			return "", "", "", err
		}
		silentPath := filepath.Join(tmpDir, "video_silent.mp4")
		if err := runFFmpeg(ctx, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", silentPath); err != nil {
			return "", "", "", fmt.Errorf("视频拼接失败：%w", err)
		}
		outputPath := filepath.Join(tmpDir, "result.mp4")
		if len(audios) > 0 {
			audioPath := audios[0].Path
			if len(audios) > 1 {
				audioPath = filepath.Join(tmpDir, "audio_track.m4a")
				if err := concatAudioSources(ctx, tmpDir, audios, audioPath); err != nil {
					return "", "", "", err
				}
			}
			// Pad a short narration track with silence, then let -shortest stop at the
			// concatenated video duration. Without apad, a short narration would cut
			// off the remaining story clips.
			if err := runFFmpeg(ctx, "-y", "-i", silentPath, "-i", audioPath, "-filter_complex", "[1:a]apad[a]", "-map", "0:v:0", "-map", "[a]", "-c:v", "copy", "-c:a", "aac", "-shortest", "-movflags", "+faststart", outputPath); err != nil {
				return "", "", "", fmt.Errorf("视频与音频合成失败：%w", err)
			}
		} else if err := runFFmpeg(ctx, "-y", "-i", silentPath, "-c", "copy", "-movflags", "+faststart", outputPath); err != nil {
			return "", "", "", err
		}
		return outputPath, "video", "video/mp4", nil
	}
	if len(images) > 0 {
		if width == 0 {
			width, _ = probeMediaDimensions(ctx, images[0].Path)
		}
		if width <= 0 {
			width = 1920
		}
		outputPath := filepath.Join(tmpDir, "result.png")
		args := []string{"-y"}
		for _, source := range images {
			args = append(args, "-i", source.Path)
		}
		parts := make([]string, 0, len(images)+1)
		labels := make([]string, 0, len(images))
		for index := range images {
			label := fmt.Sprintf("i%d", index)
			parts = append(parts, fmt.Sprintf("[%d:v]scale=%d:-1[%s]", index, width, label))
			labels = append(labels, "["+label+"]")
		}
		parts = append(parts, strings.Join(labels, "")+fmt.Sprintf("vstack=inputs=%d[out]", len(images)))
		args = append(args, "-filter_complex", strings.Join(parts, ";"), "-map", "[out]", "-frames:v", "1", outputPath)
		if err := runFFmpeg(ctx, args...); err != nil {
			return "", "", "", fmt.Errorf("图片合成失败：%w", err)
		}
		return outputPath, "image", "image/png", nil
	}
	if len(audios) > 0 {
		outputPath := filepath.Join(tmpDir, "result.m4a")
		if err := concatAudioSources(ctx, tmpDir, audios, outputPath); err != nil {
			return "", "", "", err
		}
		return outputPath, "audio", "audio/mp4", nil
	}
	return "", "", "", fmt.Errorf("没有支持的合成素材")
}

func composeOutputDimensions(value string) (int, int) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1080x1920":
		return 1080, 1920
	case "1080x1080":
		return 1080, 1080
	case "720x1280":
		return 720, 1280
	case "720x480":
		return 720, 480
	case "", "keep":
		return 0, 0
	default:
		return 0, 0
	}
}

func probeMediaDimensions(ctx context.Context, path string) (int, int) {
	ffprobePath := "ffprobe"
	if ffmpegPath, err := ffmpegBinaryPath(); err == nil {
		candidate := filepath.Join(filepath.Dir(ffmpegPath), ffprobeExecutableName())
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			ffprobePath = candidate
		}
	}
	cmd := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=s=x:p=0",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "x")
	if len(parts) != 2 {
		return 0, 0
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

func mediaHasAudio(ctx context.Context, path string) bool {
	ffprobePath := "ffprobe"
	if ffmpegPath, err := ffmpegBinaryPath(); err == nil {
		candidate := filepath.Join(filepath.Dir(ffmpegPath), ffprobeExecutableName())
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			ffprobePath = candidate
		}
	}
	cmd := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=index",
		"-of", "csv=p=0",
		path,
	)
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

func concatAudioSources(ctx context.Context, tmpDir string, audios []composeSource, outputPath string) error {
	listPath := filepath.Join(tmpDir, "audios.txt")
	var list strings.Builder
	for _, source := range audios {
		list.WriteString("file '")
		list.WriteString(strings.ReplaceAll(source.Path, "'", "'\\''"))
		list.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0600); err != nil {
		return err
	}
	if err := runFFmpeg(ctx, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c:a", "aac", outputPath); err != nil {
		return fmt.Errorf("音频拼接失败：%w", err)
	}
	return nil
}

func runFFmpeg(ctx context.Context, args ...string) error {
	ffmpegPath, err := ffmpegBinaryPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", truncateText(stderr.String(), 400))
	}
	return nil
}

func ffprobeExecutableName() string {
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		return "ffprobe.exe"
	}
	return "ffprobe"
}
