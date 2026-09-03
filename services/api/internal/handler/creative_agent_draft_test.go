package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/starai/api/internal/service"
)

func TestAgentDraftIncrementalDurationPreservesEstablishedSlots(t *testing.T) {
	if !agentIncrementalRequest("22秒") {
		t.Fatal("a duration-only clarification must continue the existing draft")
	}
	d := &service.AgentDraft{Version: 2, Slots: map[string]interface{}{"media_type": "video", "prompt": "原始创意", "script": "原有完整文案", "target_duration_sec": 15, "character": "小满", "style": "写实", "aspect_ratio": "9:16", "asset_ids": []string{"asset_1"}}}
	plan := map[string]interface{}{"intent": "chat", "action": "new_task", "prompt": "错误改写整个故事", "slot_updates": map[string]interface{}{"script": "错误新文案", "character": "其他角色"}, "slot_evidence": map[string]interface{}{"script": "改成22秒", "character": "改成22秒"}}
	if err := mergeCreativeAgentDraft(d, plan, "改成22秒"); err != nil {
		t.Fatal(err)
	}
	if d.Slots["target_duration_sec"] != 22 || d.Slots["script"] != "原有完整文案" || d.Slots["character"] != "小满" || d.Slots["style"] != "写实" || d.Slots["aspect_ratio"] != "9:16" {
		t.Fatalf("unrelated slots overwritten: %#v", d.Slots)
	}
	if d.Sources["target_duration_sec"].Source != "user" || d.Sources["target_duration_sec"].Version != 2 {
		t.Fatalf("missing provenance: %#v", d.Sources)
	}
	prompt := creativeAgentSlotPrompt(d.Slots)
	if !strings.Contains(prompt, "原有完整文案") || !strings.Contains(prompt, "成品总秒数：22") || !strings.Contains(prompt, "角色：小满") {
		t.Fatal(prompt)
	}
}

func TestAgentDraftQuestionsAndNewTasks(t *testing.T) {
	d := &service.AgentDraft{Version: 3, Slots: map[string]interface{}{"script": "已写好的文案", "character": "旧角色", "target_duration_sec": 15}}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "chat", "action": "new_task", "slot_updates": map[string]interface{}{"target_duration_sec": 8}, "slot_evidence": map[string]interface{}{"target_duration_sec": "这是什么"}}, "这是什么"); err != nil {
		t.Fatal(err)
	}
	if d.Slots["target_duration_sec"] != 15 {
		t.Fatal("question mutated slots")
	}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "chat", "action": "new_task", "reply": "新的猫咪故事文案"}, "新任务，给我写一个猫咪故事"); err != nil {
		t.Fatal(err)
	}
	if d.Slots["character"] != nil || d.Slots["script"] != "新的猫咪故事文案" {
		t.Fatalf("new task inherited old context: %#v", d.Slots)
	}
}

func TestAgentDraftConfirmedRequestUsesServerSnapshot(t *testing.T) {
	stored := map[string]interface{}{"plan_version": 7, "model_code": "approved-model", "prompt": "approved-script", "params": map[string]interface{}{"target_duration_sec": 22}, "asset_ids": []string{"approved-asset"}}
	req := creativeAgentGenerateRequest{ConversationID: "conv", PlanVersion: 7, Confirmed: true}
	if err := decodeAgentDraftRequest(stored, &req); err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "approved-script" || req.ModelCode != "approved-model" || req.AssetIDs[0] != "approved-asset" || req.PlanVersion != 7 {
		t.Fatalf("snapshot lost: %#v", req)
	}
	raw, _ := json.Marshal(stored)
	var roundTrip map[string]interface{}
	_ = json.Unmarshal(raw, &roundTrip)
	if err := decodeAgentDraftRequest(roundTrip, &req); err != nil {
		t.Fatal(err)
	}
}
