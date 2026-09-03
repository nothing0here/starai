package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starai/api/internal/runtime"
	"github.com/starai/api/internal/service"
)

func TestCreativeSlotRepairNewsHistory(t *testing.T) {
	text := "帮我把这四条热点新闻 做成一个10秒的 9:16的 新闻资讯播报短视频"
	for _, quality := range []interface{}{nil, "auto", "高清", "720P", 1080, map[string]interface{}{"value": "720p"}} {
		d := &service.AgentDraft{Version: 2}
		plan := map[string]interface{}{"intent": "workflow", "prompt": "四条既定新闻，分别呈现精简标题，竖屏播报。", "slot_updates": map[string]interface{}{"quality": quality, "target_duration_sec": "10秒", "aspect_ratio": "9：16"}, "slot_evidence": map[string]interface{}{"quality": text}}
		if err := mergeCreativeAgentDraft(d, plan, text); err != nil {
			t.Fatal(err)
		}
		if d.Slots["target_duration_sec"] != 10 || d.Slots["aspect_ratio"] != "9:16" || d.Slots["prompt"] == "" || len(d.SlotIssues) != 0 {
			t.Fatalf("optional quality discarded demand: %#v", d)
		}
		decision, decided := creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "assistant", Content: "四条已选新闻"}, {Role: "user", Content: text}}, creativeAgentClockAt(nil, time.Now()))
		if !decided || decision.NeedsSearch {
			t.Fatal("conversion replaced selected sources")
		}
	}
	decision, _ := creativeAgentFastSearchDecision([]runtime.ChatMessage{{Role: "user", Content: "重新核验这四条新闻的最新进展再做成视频"}}, creativeAgentClockAt(nil, time.Now()))
	if !decision.NeedsSearch {
		t.Fatal("explicit verification was suppressed")
	}
}

func TestCreativeSlotRecognizesNamedOrientation(t *testing.T) {
	tests := []struct{ text, want string }{
		{"根据这份提示词给我生成竖屏抖音风格8秒短视频", "9:16"},
		{"不要横屏，要竖屏", "9:16"},
		{"不要竖屏，改成横屏", "16:9"},
		{"输出分辨率1080x1920", "9:16"},
	}
	for _, test := range tests {
		ratio, _, ok := creativeAgentRequestedAspectRatio(test.text)
		if !ok || ratio != test.want {
			t.Fatalf("text=%q ratio=%q ok=%v, want %q", test.text, ratio, ok, test.want)
		}
	}

	draft := &service.AgentDraft{}
	plan := map[string]interface{}{"intent": "workflow", "prompt": "立冬短视频", "slot_updates": map[string]interface{}{"target_duration_sec": 8}}
	if err := mergeCreativeAgentDraft(draft, plan, "根据这份提示词给我生成竖屏抖音风格8秒短视频"); err != nil {
		t.Fatal(err)
	}
	if draft.Slots["aspect_ratio"] != "9:16" {
		t.Fatalf("named orientation was not persisted: %#v", draft.Slots)
	}
}

func TestCreativeSlotIssuesPreserveDemandAcrossCorrection(t *testing.T) {
	d := &service.AgentDraft{Version: 2, Slots: map[string]interface{}{"script": "保留播报文案", "quality": "720p", "target_duration_sec": 10, "media_type": "video"}}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "video", "slot_updates": map[string]interface{}{"style": "演播室风格"}}, "改为8K画质，9:16"); err != nil {
		t.Fatal(err)
	}
	if len(d.SlotIssues) != 1 || d.SlotIssues["quality"] == "" || d.Slots["script"] != "保留播报文案" || d.Slots["style"] != "演播室风格" || d.Slots["quality"] != "720p" {
		t.Fatalf("bad partial preservation: %#v", d)
	}
	// LLM guesses and replan cannot erase unresolved explicit constraints.
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "video", "action": "new_task", "slot_updates": map[string]interface{}{"quality": "1080p"}}, "你帮我整理成正确的然后再执行"); err != nil {
		t.Fatal(err)
	}
	if len(d.SlotIssues) == 0 || d.Slots["script"] != "保留播报文案" {
		t.Fatal("repair bypassed unresolved choice or reset draft")
	}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "video"}, "使用默认画质"); err != nil {
		t.Fatal(err)
	}
	if len(d.SlotIssues) != 0 || d.Slots["quality"] != nil || d.Slots["script"] != "保留播报文案" {
		t.Fatal(d)
	}
	for _, text := range []string{"改成1000秒", "改成1.5秒", "改成-5秒"} {
		if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "video"}, text); err != nil {
			t.Fatal(err)
		}
		if d.SlotIssues["target_duration_sec"] == "" || d.Slots["target_duration_sec"] != 10 {
			t.Fatalf("silently changed unsupported duration %q: %#v", text, d)
		}
	}
	if err := mergeCreativeAgentDraft(d, map[string]interface{}{"intent": "video"}, "改成12秒"); err != nil {
		t.Fatal(err)
	}
	if len(d.SlotIssues) != 0 || d.Slots["target_duration_sec"] != 12 {
		t.Fatal(d)
	}
}

func TestCreativeSlotRepairDatabaseConfirmation(t *testing.T) {
	dsn := os.Getenv("AGENT_DRAFT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AGENT_DRAFT_TEST_DATABASE_URL for isolated SQL regression")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("slot_repair_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE conversations(public_id text PRIMARY KEY,user_id bigint,agent_state jsonb DEFAULT '{}',updated_at timestamptz DEFAULT now());
CREATE TABLE workflow_definitions(code text,name text,description text,icon text,category text,nodes jsonb,input_schema jsonb,price_rule jsonb,display_config jsonb,runtime_config jsonb,is_enabled bool,sort_order int);
CREATE TABLE models(id bigint,code text,display_name text,new_api_model text,new_api_endpoint text,request_mode text,category text,icon_url text,description text,tags jsonb,input_schema jsonb,default_params jsonb,new_api_extra_params jsonb,price_rule jsonb,runtime_rule jsonb,retention_days int,is_enabled bool,sort_order int);
INSERT INTO models VALUES (1,'video_test','Video','','','video','video',NULL,NULL,'[]','{"properties":{"duration":{"enum":[8]}}}','{}','{}','{}','{}',7,true,0);
INSERT INTO conversations(public_id,user_id) VALUES ('news',1);`)
	if err != nil {
		t.Fatal(err)
	}
	models := service.NewModelService(pool)
	chat := service.NewChatService(pool, models, nil, nil, nil)
	h := &Handler{agents: service.NewAgentService(pool, nil, nil, nil), chat: chat, models: models}
	round := func(text string, quality interface{}, base int64) *service.AgentDraft {
		t.Helper()
		d, err := chat.BeginAgentDraftTurn(ctx, 1, "news", base)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := h.finalizeCreativeAgentDraft(ctx, 1, "news", creativeAgentPlanRequest{Draft: d, VideoModelCode: "video_test"}, map[string]interface{}{"intent": "workflow", "action": "update", "prompt": "四条既定新闻的标题快报，演播室竖屏画面，不新增报道。", "slot_updates": map[string]interface{}{"quality": quality}}, text)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stringAny(plan["reply"]), "格式或范围不正确") {
			t.Fatal(plan)
		}
		saved, err := chat.GetAgentDraft(ctx, 1, "news")
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	d := round("帮我把这四条热点新闻做成一个10秒的9:16新闻资讯播报短视频", "高清", 0)
	if d.Status != "awaiting_confirmation" || d.Plan["needs_confirm"] != true || d.Slots["target_duration_sec"] != float64(10) {
		t.Fatalf("repair did not reach confirmation: %#v", d)
	}
	params := d.Plan["params"].(map[string]interface{})
	if params["storyboard_grid"] != float64(2) || params["segment_duration_sec"] != float64(8) {
		t.Fatal(params)
	}
	d = round("改成8K画质", nil, 1)
	if d.Status != "draft" || d.Plan["needs_confirm"] != false || len(d.Missing) != 1 || d.Missing[0] != "quality" {
		t.Fatalf("unsupported quality executable: %#v", d)
	}
	if chat.ClaimAgentDraft(ctx, 1, "news", 2) == nil {
		t.Fatal("unresolved draft confirmed")
	}
	// Persisted issues survive a JSON round-trip and another local replan.
	raw, _ := json.Marshal(d)
	var restored service.AgentDraft
	_ = json.Unmarshal(raw, &restored)
	preview, err := h.finalizeCreativeAgentDraft(ctx, 1, "news", creativeAgentPlanRequest{Draft: &restored, Preview: true, VideoModelCode: "video_test"}, map[string]interface{}{"intent": "workflow", "action": "update"}, "")
	if err != nil || preview["needs_confirm"] != false {
		t.Fatalf("replan bypassed issue: %#v %v", preview, err)
	}
	d = round("使用默认画质", nil, 2)
	if d.Status != "awaiting_confirmation" || d.Plan["needs_confirm"] != true || len(d.SlotIssues) != 0 || d.Slots["target_duration_sec"] != float64(10) {
		t.Fatalf("guided repair lost original request: %#v", d)
	}
	if chat.ClaimAgentDraft(ctx, 1, "news", 1) == nil {
		t.Fatal("old confirmation accepted")
	}
	// No queue/billing clients or task tables exist: these rounds must only plan.
}
