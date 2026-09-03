package handler

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/starai/api/internal/service"
)

// Normalize deliverable nouns before checking verbs: writing a video prompt is
// not making a video. Keep a separate later "再生成视频" clause executable.
func creativeAgentMediaRequest(text string) bool {
	if creativeAgentGenerationProhibited(text) {
		return false
	}
	text = regexp.MustCompile(`(?i)(?:短视频|视频|短剧|图片|图像)(?:生成)?\s*(?:的)?\s*(提示词|prompt|文案|脚本)|生成(?:视频|图片)(?:的)?提示词`).ReplaceAllString(text, "文字稿")
	return regexp.MustCompile(`(生成|制作|做成|合成|转成).{0,40}(视频|短剧|成片|图片|图像|配音|音乐|歌曲)`).MatchString(text)
}

func creativeAgentGenerationProhibited(text string) bool {
	return regexp.MustCompile(`^(取消|停止)|(?:先别|不要|不用|暂不|不需要)(再|继续|自动|立即|直接|现在|马上|帮我|去|先|\s)*(生成|制作|执行)`).MatchString(strings.TrimSpace(text))
}

func creativeAgentPromptDraftRequest(text string) bool {
	return regexp.MustCompile(`(?i)提示词|\bprompt\b`).MatchString(text) && !creativeAgentMediaRequest(text)
}

func creativeAgentWritingRequest(text string) bool {
	return creativeAgentPromptDraftRequest(text) || regexp.MustCompile(`(写|创作|编|改|润色|完善).{0,30}(文案|脚本|故事)|(?:文案|脚本|故事).{0,12}(改|润色|完善)`).MatchString(text)
}

func creativeAgentResearchOnly(text string) bool {
	return !creativeAgentMediaRequest(text) && !creativeAgentWritingRequest(text) && regexp.MustCompile(`搜索|联网|热搜|榜单|检索|提取.{0,20}(文案|内容)`).MatchString(text)
}

func creativeAgentTextOnly(text string) bool {
	// These are conservative safety guards; semantic understanding stays with
	// the planner. Mentioning a medium is not authorization to generate it.
	text = strings.TrimSpace(text)
	if creativeAgentGenerationProhibited(text) {
		return true
	}
	if creativeAgentPromptDraftRequest(text) || regexp.MustCompile(`^(这是什么|为什么|为何|怎么回事|什么意思|解释|说明|不对|有问题|你这.*(啥|什么)|你.*整理.*(啥|什么))`).MatchString(text) {
		return true
	}
	// "生成视频文案" produces text; "根据文案生成视频" produces media.
	if creativeAgentMediaRequest(text) {
		return false
	}
	return regexp.MustCompile(`文案|脚本|写.{0,8}故事|解释|这是什么|什么意思|怎么回事|先别|先讨论`).MatchString(text)
}

func guardCreativeAgentIntent(plan map[string]interface{}, text string) map[string]interface{} {
	intent := strings.ToLower(strings.TrimSpace(stringAny(plan["intent"])))
	if creativeAgentTextOnly(text) && intent != "chat" && intent != "clarify" {
		reply := stringAny(plan["reply"])
		if creativeAgentWritingRequest(text) && len([]rune(stringAny(plan["prompt"]))) > len([]rune(reply)) {
			reply = stringAny(plan["prompt"])
		}
		if reply == "" {
			reply = "我先和你确认需求或解释当前内容，不会创建生成任务。"
		}
		// Preserve the validated delta: changing the route must not discard edits.
		plan = copyStringMap(plan)
		plan["intent"], plan["reply"], plan["prompt"] = "chat", reply, ""
		plan["params"], plan["needs_confirm"] = map[string]interface{}{}, false
		return plan
	}
	plan["needs_confirm"] = intent == "image" || intent == "video" || intent == "workflow" || intent == "speech" || intent == "music"
	return plan
}

func (h *Handler) prepareCreativeAgentPlan(ctx context.Context, plan map[string]interface{}, text, videoCode string) map[string]interface{} {
	plan = guardCreativeAgentIntent(plan, text)
	intent := stringAny(plan["intent"])
	if intent != "video" && intent != "workflow" {
		return plan
	}
	if videoCode == "" {
		videoCode = stringAny(h.creativeAgentRuntimeConfig(ctx)["video_model_code"])
	}
	model, err := h.models.GetFullByCode(ctx, videoCode)
	if err != nil || model == nil || !model.IsEnabled || model.RequestMode != "video" {
		return map[string]interface{}{"intent": "clarify", "reply": "请先选择已启用的视频模型，再确认生成计划。"}
	}
	return prepareCreativeAgentVideoPlan(plan, text, model)
}

func prepareCreativeAgentVideoPlan(plan map[string]interface{}, text string, model *service.ModelFull) map[string]interface{} {
	params, _ := plan["params"].(map[string]interface{})
	params = copyStringMap(params)
	// Explicit latest-user duration wins over a stale planner/default value.
	minimum, maximum, ok := creativeAgentRequestedDuration(nil, text)
	if !ok {
		minimum, maximum, ok = creativeAgentRequestedDuration(params, "")
	}
	if !ok {
		return map[string]interface{}{"intent": "clarify", "reply": "希望成品视频总共多少秒？确认目标时长后，我会按所选模型的能力规划分段。"}
	}
	target := (minimum + maximum) / 2
	count, seconds, err := service.AgentVideoLayout(model, target)
	if err != nil {
		return map[string]interface{}{"intent": "clarify", "reply": err.Error()}
	}
	params["target_duration_sec"], params["duration"] = target, seconds
	params["video_model_code"] = model.Code
	plan["model_code"], plan["needs_confirm"] = model.Code, true
	if count > 1 || seconds != target || stringAny(plan["intent"]) == "workflow" || creativeAgentWorkflowCue(text) {
		plan["intent"], plan["workflow_code"] = "workflow", "ai_comic_drama"
		params["storyboard_grid"], params["segment_duration_sec"], params["_mode"] = count, seconds, "auto"
		plan["reply"] = fmt.Sprintf("待确认：使用 %s 生成 %d 段素材（每段 %d 秒），对齐后合成为 %d 秒成品。确认后才开始生成。", model.Code, count, seconds, target)
	} else {
		plan["intent"], plan["workflow_code"] = "video", ""
		plan["reply"] = fmt.Sprintf("待确认：使用 %s 生成一段 %d 秒视频。确认后才开始生成。", model.Code, target)
	}
	plan["params"] = params
	if field, issue := creativeVideoParameterIssue(model, params); issue != "" {
		return map[string]interface{}{"intent": "clarify", "reply": issue, "needs_confirm": false, "invalid_field": field}
	}
	return plan
}

func creativeVideoParameterIssue(model *service.ModelFull, params map[string]interface{}) (string, string) {
	properties, _ := model.InputSchema["properties"].(map[string]interface{})
	for _, field := range []struct {
		slot, label string
		keys        []string
	}{
		{"quality", "画质", []string{"resolution", "quality"}},
		{"aspect_ratio", "画幅", []string{"ratio", "aspect_ratio"}},
	} {
		value := stringAny(params[field.slot])
		if value == "" {
			continue
		}
		for _, key := range field.keys {
			spec, _ := properties[key].(map[string]interface{})
			options := []string{}
			switch items := spec["enum"].(type) {
			case []interface{}:
				for _, item := range items {
					options = append(options, fmt.Sprint(item))
				}
			case []string:
				options = items
			}
			if len(options) == 0 {
				continue
			}
			matched := false
			for _, option := range options {
				if normalizeCreativeSlotValue(field.slot, option) == value {
					matched = true
					break
				}
			}
			if !matched {
				return field.slot, fmt.Sprintf("所选模型 %s 不支持当前%s %s，可选：%s。请修改%s或更换模型后确认，其他需求已保留。", model.Code, field.label, value, strings.Join(options, "、"), field.label)
			}
			break
		}
	}
	return "", ""
}

// Recheck capabilities at execution so a changed model/schema cannot silently
// replace the duration or segment count the user just confirmed.
func validateCreativeVideoExecution(model *service.ModelFull, params map[string]interface{}, workflow bool) error {
	if _, issue := creativeVideoParameterIssue(model, params); issue != "" {
		return fmt.Errorf("%s", issue)
	}
	target := creativeAgentPositiveInt(params["target_duration_sec"])
	count, seconds, err := service.AgentVideoLayout(model, target)
	if err != nil {
		return err
	}
	if workflow {
		if creativeAgentPositiveInt(params["storyboard_grid"]) != count || creativeAgentPositiveInt(params["segment_duration_sec"]) != seconds {
			return fmt.Errorf("视频模型能力或分段方案已变化，请重新规划并确认后执行")
		}
	} else if count != 1 || seconds != target || creativeAgentPositiveInt(params["duration"]) != seconds {
		return fmt.Errorf("目标时长需要分段或裁剪，请先确认成片工作流，不能直接改用模型默认时长")
	}
	return nil
}
