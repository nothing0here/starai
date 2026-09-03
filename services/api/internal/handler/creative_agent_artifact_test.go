package handler

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/starai/api/internal/runtime"
	"github.com/starai/api/internal/service"
)

func TestLatestArtifactTemplatesAreRejected(t *testing.T) {
	for _, text := range []string{"", "[场景描述]：[具体场景，如：办公室]", "[人物动作]：[科技互动动作，如：操作全息界面]", "说明\n```\n方案一\n```\n示例\n```\n方案二\n```", "根据科技热点，我为您整理了一份12秒文案提示词模板"} {
		if _, issue := creativeAgentArtifactText(text); issue == "" {
			t.Fatalf("invalid artifact accepted: %q", text)
		}
	}
	value, issue := creativeAgentArtifactText("以下是原创演示：\n```text\n0–4秒：职场人士打开报表；4–12秒：助手整理数据，人物检查结果。竖屏9:16，真人写实，无旁白。\n```\n尚未生成视频。")
	if issue != "" || strings.Contains(value, "以下是") || strings.Contains(value, "尚未") {
		t.Fatalf("explanation leaked into artifact: %q %s", value, issue)
	}
	d := &service.AgentDraft{Version: 2, Slots: map[string]interface{}{"generation_prompt": "```\n方案一\n```\n```\n方案二\n```", "target_duration_sec": 12}}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "workflow", "action": "new_task", "prompt": "自动选了方案一"}, "根据提示词帮我生成一个12秒竖屏视频"); err == nil {
		t.Fatal("ambiguous old options silently became a new task")
	}
}

func TestTemplateMentionDoesNotAuthorizeUnfinishedContent(t *testing.T) {
	for _, text := range []string{"给我完整提示词，不要模板", "不是模板，是能直接用的提示词", "把提示词模板填好", "补全这个模板"} {
		if creativeAgentWantsTemplate(text) {
			t.Fatalf("finished artifact request treated as template: %s", text)
		}
	}
	text := "给我一份视频提示词模板"
	if !creativeAgentWantsTemplate(text) {
		t.Fatal("explicit template request was lost")
	}
	d := &service.AgentDraft{Version: 2, Slots: map[string]interface{}{"generation_prompt": "上一轮已经完成的办公室视频提示词"}}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "chat", "reply": "提示词模板：[具体场景]"}, text); err != nil {
		t.Fatal(err)
	}
	if stringAny(d.Slots["artifact_issue"]) == "" {
		t.Fatal("template delivery could silently reuse an older finished artifact")
	}
}

func TestLivePromptOnlyRequestPersistsArtifactAndRejectsForbiddenClaim(t *testing.T) {
	text := "请给我一份12秒短视频生成提示词，不要模板，不宣称具体效率数字；只交付文字，不要生成图片、视频、配音，也不要执行任务。"
	if !creativeAgentPromptDraftRequest(text) || !creativeAgentWritingRequest(text) || creativeAgentMediaRequest(text) {
		t.Fatal("negative media mention bypassed prompt writing")
	}
	bad := "9-12秒：屏幕显示效率提升80%"
	if _, issue := creativeAgentArtifactForRequest(bad, text); issue == "" {
		t.Fatal("explicitly forbidden efficiency claim accepted")
	}
	d := &service.AgentDraft{Version: 1}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "chat", "reply": bad}, text); err == nil {
		t.Fatal("bad live reply saved as executable artifact")
	}
	good := "12秒，竖屏9:16，真人写实；办公室女性请AI整理报表，检查结果后发送。旁白：AI辅助整理，结果由我核对。无功效数字、无字幕。"
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "chat", "reply": good}, text); err != nil || d.Slots["generation_prompt"] != good {
		t.Fatalf("valid text-only artifact was lost: %v %#v", err, d.Slots)
	}
}

func TestNarrationLimitCountsAllScenes(t *testing.T) {
	text := "请把旁白精简到10个汉字以内"
	if _, issue := creativeAgentArtifactForRequest("0-6秒\n旁白：数据太多，整理太耗时。\n6-12秒\n旁白：工作更简单，决策更智能。", text); issue == "" {
		t.Fatal("per-scene lines bypassed the total narration limit")
	}
	if _, issue := creativeAgentArtifactForRequest("0-6秒\n旁白：助手整理。\n6-12秒\n旁白：我来核对。", text); issue != "" {
		t.Fatal(issue)
	}
}

func TestArtifactRepairIsBoundedAndCannotChangeUnrelatedSlots(t *testing.T) {
	text := "帮我整理一份12秒科技短视频提示词"
	bad := map[string]interface{}{"intent": "chat", "reply": "[场景描述]：[具体场景]", "action": "update", "slot_updates": map[string]interface{}{"target_duration_sec": 12}}
	input := service.CompletionInput{ModelCode: "current-model", Stream: true, Messages: []runtime.ChatMessage{{Role: "user", Content: text}}}
	calls := 0
	complete := func(in service.CompletionInput) (*service.CompletionResult, error) {
		calls++
		if in.Stream || in.ModelCode != "current-model" || !in.Ephemeral || len(in.Messages) != 3 {
			t.Fatalf("repair context invalid: %#v", in)
		}
		payload, _ := json.Marshal(map[string]interface{}{"intent": "workflow", "reply": "完整提示词正文，写实办公室场景，镜头推进后展示数据，无旁白。", "slot_updates": map[string]interface{}{"generation_prompt": "完整提示词正文，写实办公室场景，镜头推进后展示数据，无旁白。", "target_duration_sec": 50, "character": "不应改变的人物"}})
		return &service.CompletionResult{Content: string(payload)}, nil
	}
	fixed := repairCreativeAgentArtifactOnce(input, bad, text, 1, complete)
	updates := fixed["slot_updates"].(map[string]interface{})
	if calls != 1 || fixed["intent"] != "chat" || fixed["needs_confirm"] != false || updates["target_duration_sec"] != 12 || updates["character"] != nil || updates["generation_prompt"] == nil {
		t.Fatalf("repair escaped scope: %#v", fixed)
	}
	if bad["slot_updates"].(map[string]interface{})["generation_prompt"] != nil || len(input.Messages) != 1 {
		t.Fatal("repair mutated original inputs")
	}
	repairCreativeAgentArtifactOnce(input, fixed, text, 1, complete)
	repairCreativeAgentArtifactOnce(input, bad, text, 0, complete)
	if calls != 1 {
		t.Fatal("valid/disabled repair still called the model")
	}
	failCalls := 0
	unchanged := repairCreativeAgentArtifactOnce(input, bad, text, 1, func(service.CompletionInput) (*service.CompletionResult, error) {
		failCalls++
		return nil, errors.New("unavailable")
	})
	if failCalls != 1 || unchanged["reply"] != bad["reply"] {
		t.Fatal("unbounded failure recovery")
	}
}

func TestResearchThenWritingMustSearchButLocalEditsDoNot(t *testing.T) {
	clock := creativeAgentClockAt(nil, time.Now())
	for _, text := range []string{"帮我分析一下今天的热点短视频，然后整理一份12秒的文案提示词", "根据最新热点写一份视频提示词"} {
		messages := []runtime.ChatMessage{{Role: "user", Content: text}}
		d, decided := creativeAgentFastSearchDecision(messages, clock)
		if !decided || !d.NeedsSearch || !shouldSuggestCreativeAgentSearch(messages, clock) {
			t.Fatalf("research was skipped for %s", text)
		}
	}
	for _, text := range []string{"你现在只有文案，我要完整的视频生成提示词", "把刚才今天热点的提示词改成8秒，不用搜索"} {
		d, _ := creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "user", Content: text}}, clock)
		if d.NeedsSearch {
			t.Fatal("local prompt edit performed web research")
		}
	}
}
