package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/service"
	"github.com/starai/api/internal/util"
)

func agentIncrementalRequest(text string) bool {
	return agentDurationOnlyRequest(text) || regexp.MustCompile(`^(请|帮我)?(改|修改|换|调整|保持|还是|加|去掉|不要|用刚才|按刚才|就按|根据刚才|根据这个|按这个|继续|生成吧)|^你这.*(啥|什么)|整理成正确|按提示修正|使用默认画质`).MatchString(strings.TrimSpace(text))
}

func agentDurationOnlyRequest(text string) bool {
	return regexp.MustCompile(`^(请|帮我)?(时长)?(改成|改为|修改成|调整为|换成)?\s*\d{1,3}\s*秒(吧|。|！|!)?$`).MatchString(strings.TrimSpace(text))
}

// Merge only the proposed delta. Rule-derived exact durations win over LLM
// guesses, while questions preserve slots without granting execution intent.
func mergeCreativeAgentDraft(d *service.AgentDraft, plan map[string]interface{}, text string) error {
	d.Init()
	action := stringAny(plan["action"])
	question := regexp.MustCompile(`^(这是什么|为什么|为何|怎么回事|什么意思|解释|说明|不对|有问题)`).MatchString(strings.TrimSpace(text))
	if (question && !creativeAgentWritingRequest(text)) || action == "cancel" || creativeAgentResearchOnly(text) {
		return nil
	}
	// Legacy conversations may contain a whole template/options answer in this
	// slot. An implicit "根据提示词生成" must not authorize the model to pick one.
	if !creativeAgentTextOnly(text) && regexp.MustCompile(`根据|按照|用刚才|用上面|用这个|按刚才|按上面|按这个|继续|生成吧`).MatchString(text) && stringAny(d.Slots["generation_prompt"]) != "" {
		if _, issue := creativeAgentArtifactText(stringAny(d.Slots["generation_prompt"])); issue != "" && !regexp.MustCompile(`选择|选第|第[一二三四五六七八九十0-9]+[个条种项]|用.{1,20}(方案|示例)|按.{1,20}(方案|示例)`).MatchString(text) {
			return fmt.Errorf("上次提示词尚未定稿：%s。请先确定一个完整方案，原对话和素材仍保留", issue)
		}
	}
	if action == "new_task" && !agentIncrementalRequest(text) {
		d.Slots, d.Sources = map[string]interface{}{}, map[string]service.AgentSlotSource{}
		d.SlotIssues = nil
	}
	updates, _ := plan["slot_updates"].(map[string]interface{})
	updates = copyStringMap(updates)
	if creativeAgentWantsTemplate(text) {
		delete(updates, "generation_prompt")
	}
	evidence, _ := plan["slot_evidence"].(map[string]interface{})
	if value, ok := updates["generation_prompt"]; ok {
		content, issue := creativeAgentArtifactForRequest(stringAny(value), text)
		if issue != "" {
			return fmt.Errorf("提示词验收未通过：%s", issue)
		}
		updates["generation_prompt"] = content
	}
	// An exact duration-only edit must not rewrite a script even if the LLM
	// accidentally proposes a full replacement with copied evidence.
	durationOnly := agentDurationOnlyRequest(text)
	if durationOnly {
		updates = map[string]interface{}{}
	}
	updates, evidence, corrections, err := prepareCreativeSlotUpdates(d, updates, evidence, text)
	if err != nil {
		return err
	}
	plan["slot_corrections"] = corrections
	if err := service.ApplyAgentSlotUpdates(d, updates, evidence, text); err != nil {
		return err
	}
	if stringAny(d.Slots["aspect_ratio"]) == "" {
		for _, key := range []string{"generation_prompt", "prompt", "script"} {
			if ratio, quote, ok := creativeAgentRequestedAspectRatio(stringAny(d.Slots[key])); ok {
				d.SetSlot("aspect_ratio", ratio, "inferred", quote)
				break
			}
		}
	}
	if source, ok := d.Sources["generation_prompt"]; ok && source.Version == d.Version && updates["generation_prompt"] != nil {
		delete(d.Slots, "artifact_issue")
		delete(d.Sources, "artifact_issue")
	}
	// A new script invalidates an older derived generation prompt, not vice versa.
	if source, ok := d.Sources["script"]; ok && source.Version == d.Version && updates["script"] != nil && updates["generation_prompt"] == nil {
		delete(d.Slots, "generation_prompt")
		delete(d.Sources, "generation_prompt")
	}
	intent := stringAny(plan["intent"])
	if intent == "workflow" {
		if stringAny(plan["workflow_code"]) == "content_image_post" {
			intent = "image"
		} else {
			intent = "video"
		}
	}
	if d.Slots["media_type"] == nil && (intent == "video" || intent == "image" || intent == "speech" || intent == "music") {
		d.SetSlot("media_type", intent, "inferred", "")
	}
	if stringAny(d.Slots["prompt"]) == "" && !durationOnly && !creativeAgentTextOnly(text) {
		prompt := stringAny(plan["prompt"])
		if prompt != "" {
			d.SetSlot("prompt", prompt, "inferred", "")
		}
	}
	if creativeAgentTextOnly(text) && creativeAgentWritingRequest(text) && !durationOnly {
		if reply := stringAny(plan["reply"]); reply != "" && intent == "chat" {
			key := "script"
			if creativeAgentPromptDraftRequest(text) {
				key = "generation_prompt"
				if creativeAgentWantsTemplate(text) {
					d.SetSlot("artifact_issue", "当前交付的是提示词模板，尚未填写定稿；请先完成正文再生成", "validation", "")
					return nil
				}
				content, issue := creativeAgentArtifactForRequest(creativeAgentArtifactCandidate(plan), text)
				if issue != "" {
					return fmt.Errorf("提示词验收未通过：%s。请明确主题后继续完善；不会创建生成任务", issue)
				}
				reply = content
			} else {
				delete(d.Slots, "generation_prompt")
				delete(d.Sources, "generation_prompt")
			}
			// Prefer structured content without the conversational introduction.
			if source, ok := d.Sources[key]; !ok || source.Version != d.Version {
				d.SetSlot(key, reply, "draft", text)
			}
			if key == "generation_prompt" {
				delete(d.Slots, "artifact_issue")
				delete(d.Sources, "artifact_issue")
			}
			if d.Slots["media_type"] == nil && strings.Contains(text, "视频") {
				d.SetSlot("media_type", "video", "inferred", "")
			}
		}
	}
	return nil
}

func creativeAgentSlotPrompt(slots map[string]interface{}, mediaTypes ...string) string {
	base := stringAny(slots["generation_prompt"])
	if base == "" {
		base = stringAny(slots["script"])
	}
	if base == "" {
		base = stringAny(slots["prompt"])
	}
	if base == "" {
		return ""
	}
	// Speech and song prompts are literal content. Never make the model read or
	// sing execution notes such as aspect ratio, style, or duration.
	if len(mediaTypes) > 0 && (mediaTypes[0] == "speech" || mediaTypes[0] == "music") {
		return base
	}
	constraints := []string{}
	for _, field := range []struct{ key, label string }{{"target_duration_sec", "成品总秒数"}, {"character", "角色"}, {"style", "风格"}, {"aspect_ratio", "画幅"}, {"narration_perspective", "叙事视角"}, {"ending", "结尾要求"}} {
		if value := stringAny(slots[field.key]); value != "" {
			constraints = append(constraints, field.label+"："+value)
		}
	}
	if len(constraints) > 0 {
		base += "\n当前用户要求（优先于旧文案中的参数，镜头按新的总时长重新分配，不重复标题）：\n" + strings.Join(constraints, "\n")
	}
	return base
}

func (h *Handler) finalizeCreativeAgentDraft(ctx context.Context, userID int64, conversationID string, req creativeAgentPlanRequest, plan map[string]interface{}, text string) (map[string]interface{}, error) {
	d := req.Draft
	if d == nil {
		return nil, fmt.Errorf("会话任务状态未初始化")
	}
	workflowCode := strings.TrimSpace(stringAny(plan["workflow_code"]))
	reuseWorkflow := stringAny(plan["action"]) == "update" || agentIncrementalRequest(text) || strings.TrimSpace(text) == ""
	if workflowCode == "" && reuseWorkflow {
		workflowCode = strings.TrimSpace(stringAny(d.Plan["workflow_code"]))
		if workflowCode != "" {
			plan["workflow_code"] = workflowCode
		}
	}
	plan = normalizeCreativeAgentWorkflowPlan(plan, text)
	plan = guardCreativeAgentIntent(plan, text)
	workflowCode = strings.TrimSpace(stringAny(plan["workflow_code"]))
	// Validate a copy so a malformed delta cannot partially erase the old draft.
	copyRaw, _ := json.Marshal(d)
	candidate := &service.AgentDraft{}
	_ = json.Unmarshal(copyRaw, candidate)
	mergeErr := mergeCreativeAgentDraft(candidate, plan, text)
	corrections, _ := plan["slot_corrections"].([]string)
	if mergeErr == nil {
		d = candidate
	}
	policy := service.AgentPolicyFromConfig(h.creativeAgentRuntimeConfig(ctx))
	if stringAny(d.Slots["style"]) == "" && policy.DefaultStyle != "" {
		d.SetSlot("style", policy.DefaultStyle, "configuration", "")
	}
	if len(req.AssetIDs) > 0 || req.ReplaceAssets {
		d.SetSlot("asset_ids", req.AssetIDs, "selection", "")
	}
	d.Status, d.Missing = "draft", []string{}
	if mergeErr != nil {
		plan = map[string]interface{}{"intent": "clarify", "reply": mergeErr.Error()}
		if strings.Contains(mergeErr.Error(), "提示词") {
			d.SetSlot("artifact_issue", mergeErr.Error(), "validation", "")
			d.Missing = append(d.Missing, "generation_prompt")
		}
	} else if stringAny(plan["action"]) == "cancel" || regexp.MustCompile(`^(取消|停止)(计划|生成|任务|吧|。|！|!|\s)*$`).MatchString(text) {
		d.Status = "cancelled"
		plan = map[string]interface{}{"intent": "chat", "reply": "已取消待执行方案，未创建任务；需求草稿仍保留。"}
	} else if len(d.SlotIssues) > 0 {
		for key := range d.SlotIssues {
			d.Missing = append(d.Missing, key)
		}
		sort.Strings(d.Missing)
		guidance := []string{"已保留可用的文案和需求，只需修正以下项目；修正后会展示新方案，请确认后再执行："}
		for _, key := range d.Missing {
			guidance = append(guidance, d.SlotIssues[key])
		}
		plan = map[string]interface{}{"intent": "clarify", "reply": strings.Join(guidance, "\n"), "needs_confirm": false}
	} else {
		intent := stringAny(plan["intent"])
		if !creativeAgentTextOnly(text) && agentIncrementalRequest(text) && stringAny(d.Slots["media_type"]) != "" {
			if workflowCode != "" {
				intent = "workflow"
			} else {
				intent = stringAny(d.Slots["media_type"])
			}
		}
		if intent == "workflow" || intent == "video" || intent == "image" || intent == "speech" || intent == "music" {
			mediaType := intent
			if mediaType == "workflow" {
				if workflowCode == "content_image_post" {
					mediaType = "image"
				} else {
					mediaType = "video"
				}
			}
			if stringAny(d.Slots["media_type"]) != mediaType {
				d.SetSlot("media_type", mediaType, "inferred", "")
			}
			if mediaType == "music" && d.Slots["is_instrumental"] == true && stringAny(d.Slots["music_prompt"]) == "" {
				if description := stringAny(d.Slots["prompt"]); description != "" {
					d.SetSlot("music_prompt", description, "inferred", "")
				}
			}
			prompt := creativeAgentSlotPrompt(d.Slots, mediaType)
			if stringAny(d.Slots["artifact_issue"]) != "" {
				d.Missing = append(d.Missing, "generation_prompt")
			}
			if _, issue := creativeAgentArtifactText(prompt); issue != "" && prompt != "" {
				d.Missing = append(d.Missing, "generation_prompt")
			}
			if prompt == "" {
				d.Missing = append(d.Missing, "prompt")
			}
			if mediaType == "video" && creativeAgentPositiveInt(d.Slots["target_duration_sec"]) == 0 {
				d.Missing = append(d.Missing, "target_duration_sec")
			}
			params := map[string]interface{}{"max_retry": policy.MaxRetry}
			for _, key := range []string{"target_duration_sec", "image_count", "platform", "aspect_ratio", "quality", "audio_strategy", "narration_perspective", "is_instrumental", "music_prompt"} {
				if value, ok := d.Slots[key]; ok {
					params[key] = value
				}
			}
			if ratio := stringAny(params["aspect_ratio"]); ratio != "" {
				params["ratio"] = ratio
				if ratio == "9:16" {
					params["orientation"] = "portrait"
				} else {
					params["orientation"] = "landscape"
				}
			}
			configured := h.creativeAgentRuntimeConfig(ctx)
			selected := map[string]string{"video": req.VideoModelCode, "image": req.ImageModelCode, "speech": req.SpeechModelCode, "music": req.MusicModelCode}[mediaType]
			modelSource := "selection"
			if selected == "" {
				selected = stringAny(configured[mediaType+"_model_code"])
				modelSource = "configuration"
			}
			d.SetSlot("model_code", selected, modelSource, "")
			if workflowCode == "content_image_post" {
				count := creativeAgentPositiveInt(params["image_count"])
				if requested := creativeAgentRequestedImageCount(text); requested > 0 {
					count = requested
					d.SetSlot("image_count", count, "user", text)
				}
				if count < 2 || count > 6 {
					count = 4
				}
				params["image_count"], params["count"], params["creative_scene"] = count, count, "content_image_post"
			}
			plan = map[string]interface{}{"intent": intent, "prompt": prompt, "params": params, "model_code": selected, "needs_confirm": true}
			if intent == "workflow" {
				plan["workflow_code"] = workflowCode
			}
			if mediaType == "video" && creativeAgentPositiveInt(d.Slots["target_duration_sec"]) > policy.MaxDuration {
				d.Missing = append(d.Missing, "target_duration_sec")
				plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", fmt.Sprintf("当前业务允许的成品最长为%d秒，请调整总时长；原文案和素材仍保留。", policy.MaxDuration), false
			} else if len(d.Missing) > 0 {
				labels := []string{}
				for _, key := range d.Missing {
					if key == "generation_prompt" {
						labels = append(labels, "一个已填完整、没有占位符或多个候选的提示词")
					} else if key == "prompt" {
						labels = append(labels, "视频/素材的内容或文案")
					} else {
						labels = append(labels, "成品总时长（秒）")
					}
				}
				plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", "已有需求已保留，请补充："+strings.Join(labels, "、"), false
			} else {
				model, err := h.models.GetFullByCode(ctx, selected)
				if err != nil || model == nil || !model.IsEnabled || !creativeAgentModelSupportsType(model, mediaType) {
					d.Missing = append(d.Missing, "model_code")
					plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", "需求已保留，请选择已启用且类型匹配的模型。", false
				} else if mediaType == "video" {
					plan = prepareCreativeAgentVideoPlan(plan, "", model)
					if stringAny(plan["intent"]) == "clarify" {
						field := stringAny(plan["invalid_field"])
						if field == "" {
							field = "model_duration_capability"
						}
						d.Missing = append(d.Missing, field)
					}
				} else if mediaType == "image" {
					mapped, field, issue := creativeAgentImageModelParams(model, plan["params"].(map[string]interface{}))
					if issue != "" {
						d.Missing = append(d.Missing, field)
						plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", issue, false
					} else {
						plan["params"], plan["reply"] = mapped, "方案已更新，请核对图片内容、画幅与清晰度后确认执行。"
					}
				} else if mediaType == "speech" {
					if gender := stringAny(d.Slots["voice_gender"]); gender != "" {
						key, voice, ok := creativeAgentVoiceForGender(model, gender)
						if !ok {
							d.Missing = append(d.Missing, "voice_gender")
							plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", "所选语音模型没有可确认的"+map[string]string{"male": "男声", "female": "女声"}[gender]+"音色，请更换模型或调整声音要求。", false
						} else {
							plan["params"].(map[string]interface{})[key] = voice
							plan["reply"] = "方案已更新，将使用匹配的" + map[string]string{"male": "男声", "female": "女声"}[gender] + "音色；请核对朗读正文后确认执行。"
						}
					} else {
						plan["reply"] = "方案已更新，请核对朗读正文与模型后确认执行。"
					}
				} else if mediaType == "music" && creativeAgentMusicNeedsStyle(model) && stringAny(d.Slots["music_prompt"]) == "" {
					d.Missing = append(d.Missing, "music_prompt")
					plan["intent"], plan["reply"], plan["needs_confirm"] = "clarify", "歌词或纯音乐需求已保留，请再描述曲风、情绪或使用场景。", false
				} else {
					plan["reply"] = "方案已更新，请核对完整内容与模型后确认执行。"
				}
			}
			if plan["needs_confirm"] == true {
				d.Status = "awaiting_confirmation"
				plan["asset_ids"] = d.Slots["asset_ids"]
				if d.Slots["use_previous_media"] == true {
					// Snapshot the references; later browser selections cannot alter them.
					for key, refs := range map[string][]string{"reference_image_urls": req.ReferenceImageURLs, "reference_video_urls": req.ReferenceVideoURLs, "reference_audio_urls": req.ReferenceAudioURLs} {
						if len(refs) > 0 {
							d.SetSlot(key, refs, "selection", "")
						}
						plan[key] = d.Slots[key]
					}
				}
				if stringAny(plan["intent"]) == "workflow" && workflowCode == "content_image_post" {
					p := plan["params"].(map[string]interface{})
					p["image_model_code"] = selected
					p["creative_guidance"] = policy.CreationGuidance
				} else if stringAny(plan["intent"]) == "workflow" {
					p := plan["params"].(map[string]interface{})
					p["image_model_code"], p["narration_model_code"] = stringAny(configured["image_model_code"]), stringAny(configured["speech_model_code"])
					p["dialogue_model_codes"] = []string{stringAny(configured["analysis_model_code"])}
					p["creative_guidance"] = policy.CreationGuidance
				}
			}
		}
	}
	if len(corrections) > 0 {
		plan["slot_corrections"] = corrections
		plan["reply"] = strings.Join(corrections, "\n") + "\n" + stringAny(plan["reply"])
	}
	plan["plan_version"], plan["draft_status"], plan["slots"], plan["missing_fields"] = d.Version, d.Status, d.Slots, d.Missing
	plan["policy_version"] = policy.Version
	if mergeErr == nil && creativeAgentPromptDraftRequest(text) && !creativeAgentWantsTemplate(text) && stringAny(plan["intent"]) == "chat" {
		if content, issue := creativeAgentArtifactText(stringAny(d.Slots["generation_prompt"])); issue == "" {
			plan["artifact"] = map[string]interface{}{"kind": "generation_prompt", "text": content}
		}
	}
	d.Plan = plan
	if req.Preview {
		return plan, nil
	}
	if err := h.chat.SaveAgentDraft(ctx, userID, conversationID, d); err != nil {
		return nil, err
	}
	return plan, nil
}

func creativeAgentRequestedImageCount(text string) int {
	match := regexp.MustCompile(`([2-6二三四五六])\s*(?:张|幅)(?:配图|图片|图)?|([2-6二三四五六])\s*个(?:卡片|图文卡)`).FindStringSubmatch(text)
	if len(match) != 3 {
		return 0
	}
	raw := match[1]
	if raw == "" {
		raw = match[2]
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return value
	}
	return map[string]int{"二": 2, "三": 3, "四": 4, "五": 5, "六": 6}[raw]
}

// Recalculate from persisted slots, without invoking an LLM or creating a task.
func (h *Handler) CreativeAgentReplan(c *gin.Context) {
	var req creativeAgentPlanRequest
	if c.ShouldBindJSON(&req) != nil || req.ConversationID == "" {
		util.BadRequest(c, "更新方案参数错误")
		return
	}
	d, err := h.chat.GetAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID)
	if err != nil || d.Version != req.BaseVersion {
		util.BadRequest(c, "会话已更新，请同步最新版本")
		return
	}
	if d.Status == "executing" || d.Status == "submitted" {
		util.BadRequest(c, "该方案已提交，请在原任务继续，不能重新创建")
		return
	}
	if creativeAgentSlotPrompt(d.Slots) == "" {
		util.BadRequest(c, "尚无需求草稿，请先描述需求")
		return
	}
	intent := stringAny(d.Plan["intent"])
	if intent == "chat" || intent == "clarify" || intent == "" {
		intent = stringAny(d.Slots["media_type"])
	}
	if intent == "" {
		util.BadRequest(c, "请先说明希望生成的媒体类型")
		return
	}
	// An explicit model selection persists unless the caller explicitly supplies a new selection.
	if req.CheckOnly && d.Sources["model_code"].Source == "selection" {
		switch stringAny(d.Slots["media_type"]) {
		case "video":
			req.VideoModelCode = stringAny(d.Slots["model_code"])
		case "image":
			req.ImageModelCode = stringAny(d.Slots["model_code"])
		case "speech":
			req.SpeechModelCode = stringAny(d.Slots["model_code"])
		case "music":
			req.MusicModelCode = stringAny(d.Slots["model_code"])
		}
	}
	req.Draft = d
	req.Preview = true
	preview, err := h.finalizeCreativeAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID, req, map[string]interface{}{"intent": intent, "action": "update"}, "")
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	if req.CheckOnly && d.Status == "awaiting_confirmation" && sameCreativeAgentExecution(d.Plan, preview) {
		util.OK(c, map[string]interface{}{"changed": false, "draft": d})
		return
	}
	old := d.Plan
	req.Draft, err = h.chat.BeginAgentDraftTurn(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID, d.Version)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	req.Preview = false
	plan, err := h.finalizeCreativeAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID, req, map[string]interface{}{"intent": intent, "action": "update"}, "")
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	changes := creativeAgentPlanChanges(old, plan)
	// The draft is already authoritative; persist the visible change summary as history.
	raw, _ := json.Marshal(plan)
	_ = h.chat.AppendConversationMessage(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID, "assistant", string(raw))
	next, err := h.chat.GetAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, map[string]interface{}{"changed": true, "draft": next, "changes": changes})
}

func sameCreativeAgentExecution(a, b map[string]interface{}) bool {
	for _, key := range []string{"intent", "prompt", "model_code", "workflow_code", "params", "asset_ids", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "policy_version"} {
		left, _ := json.Marshal(a[key])
		right, _ := json.Marshal(b[key])
		if string(left) != string(right) {
			return false
		}
	}
	return true
}

func creativeAgentPlanChanges(old, next map[string]interface{}) []string {
	changes := []string{"保留当前文案、角色和风格；仅更新模型、素材选择及受影响的执行参数。"}
	if old["model_code"] != next["model_code"] {
		changes = append(changes, fmt.Sprintf("模型：%v → %v", old["model_code"], next["model_code"]))
	}
	a, _ := old["params"].(map[string]interface{})
	b, _ := next["params"].(map[string]interface{})
	if !reflect.DeepEqual(a["storyboard_grid"], b["storyboard_grid"]) {
		changes = append(changes, fmt.Sprintf("分段数：%v → %v", a["storyboard_grid"], b["storyboard_grid"]))
	}
	changes = append(changes, "未创建新任务；确认后才执行，费用以更新后的模型计费。")
	return changes
}

func (h *Handler) CreativeAgentState(c *gin.Context) {
	d, err := h.chat.GetAgentDraft(c.Request.Context(), c.GetInt64("user_id"), c.Param("id"))
	if err != nil {
		util.NotFound(c, "会话状态不存在或无权访问")
		return
	}
	util.OK(c, d)
}

func (h *Handler) CreativeAgentCancelPlan(c *gin.Context) {
	var req struct {
		ConversationID string `json:"conversation_id"`
		Version        int64  `json:"plan_version"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Version <= 0 {
		util.BadRequest(c, "取消参数错误")
		return
	}
	if err := h.chat.CancelAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID, req.Version); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	d, err := h.chat.GetAgentDraft(c.Request.Context(), c.GetInt64("user_id"), req.ConversationID)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, d)
}

func (h *Handler) confirmedAgentDraft(c *gin.Context, conversationID string, version int64, workflow bool) (*service.AgentDraft, bool) {
	if conversationID == "" || version <= 0 {
		util.BadRequest(c, "请先规划并确认具体版本，旧客户端确认标记不能执行任务")
		return nil, false
	}
	d, err := h.chat.GetAgentDraft(c.Request.Context(), c.GetInt64("user_id"), conversationID)
	if err != nil || d.Version != version {
		util.BadRequest(c, "确认的方案版本已失效或无权访问，请刷新后重新确认")
		return nil, false
	}
	if d.Status == "submitted" && d.ExecutionRef != "" {
		if d.ExecutionKind == "workflow" {
			result, err := h.agents.GetProject(c.Request.Context(), c.GetInt64("user_id"), d.ExecutionRef)
			if err == nil {
				util.OK(c, result)
				return nil, false
			}
		} else {
			result, err := h.tasks.Get(c.Request.Context(), c.GetInt64("user_id"), d.ExecutionRef)
			if err == nil {
				util.OK(c, result)
				return nil, false
			}
		}
		util.BadRequest(c, "此版本已经提交，请查看历史任务")
		return nil, false
	}
	if d.Status != "awaiting_confirmation" || len(d.Missing) > 0 || (stringAny(d.Plan["intent"]) == "workflow") != workflow {
		util.BadRequest(c, "方案未就绪、已取消或正在提交，请刷新任务状态")
		return nil, false
	}
	configured := h.creativeAgentRuntimeConfig(c.Request.Context())
	if creativeAgentPositiveInt(d.Plan["policy_version"]) != int(service.AgentPolicyFromConfig(configured).Version) {
		util.BadRequest(c, "业务策略已更新，请更新当前方案后确认；原需求已保留")
		return nil, false
	}
	mediaType := stringAny(d.Slots["media_type"])
	if d.Sources["model_code"].Source == "configuration" && stringAny(configured[mediaType+"_model_code"]) != stringAny(d.Plan["model_code"]) {
		util.BadRequest(c, "后台默认模型已改变，请更新方案后重新确认，不能执行旧模型快照")
		return nil, false
	}
	if workflow {
		params, _ := d.Plan["params"].(map[string]interface{})
		for runtimeKey, inputKey := range map[string]string{"image_model_code": "image_model_code", "speech_model_code": "narration_model_code"} {
			if stringAny(configured[runtimeKey]) != stringAny(params[inputKey]) {
				util.BadRequest(c, "工作流模型配置已改变，请更新方案后重新确认")
				return nil, false
			}
		}
		codes, _ := params["dialogue_model_codes"].([]interface{})
		if len(codes) > 0 && stringAny(codes[0]) != stringAny(configured["analysis_model_code"]) {
			util.BadRequest(c, "对话模型配置已改变，请更新方案后重新确认")
			return nil, false
		}
	}
	return d, true
}

func decodeAgentDraftRequest(plan map[string]interface{}, target interface{}) error {
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
