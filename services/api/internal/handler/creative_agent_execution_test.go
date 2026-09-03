package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/service"
)

func TestCreativeAgentIntentBoundary(t *testing.T) {
	for _, text := range []string{
		"开学季，做为一个程序员，给我一段程序开发人员15秒的短视频文案",
		"生成一段15秒的短视频文案", "这是什么？", "为什么又生成了", "解释一下", "先别生成视频，先写文案", "取消生成", "修改一下脚本",
	} {
		plan := guardCreativeAgentIntent(map[string]interface{}{"intent": "video", "prompt": "生成一段视频文案", "needs_confirm": false}, text)
		if plan["intent"] != "chat" || plan["needs_confirm"] != false || plan["prompt"] != "" {
			t.Fatalf("%q must not generate: %#v", text, plan)
		}
	}
	for _, text := range []string{"根据这个文案生成15秒视频", "帮我写文案然后生成视频", "生成一张写着为什么的图片", "文案不用改，直接生成视频", "做一段15秒视频"} {
		plan := guardCreativeAgentIntent(map[string]interface{}{"intent": "video", "needs_confirm": false}, text)
		if plan["intent"] != "video" || plan["needs_confirm"] != true {
			t.Fatalf("%q must require confirmation: %#v", text, plan)
		}
	}
}

func TestCreativeAgentVideoPlanUsesModelCapabilities(t *testing.T) {
	for _, tc := range []struct {
		target, maximum, count, seconds int
		intent                          string
	}{
		{15, 8, 2, 8, "workflow"}, {16, 8, 2, 8, "workflow"}, {22, 8, 3, 8, "workflow"},
		{48, 8, 6, 8, "workflow"}, {15, 15, 1, 15, "video"}, {30, 30, 1, 30, "video"}, {5, 8, 1, 8, "workflow"},
	} {
		model := &service.ModelFull{ModelDTO: service.ModelDTO{Code: "selected", InputSchema: map[string]interface{}{"properties": map[string]interface{}{"duration": map[string]interface{}{"enum": []interface{}{tc.maximum}}}}}}
		plan := prepareCreativeAgentVideoPlan(map[string]interface{}{"intent": "video", "params": map[string]interface{}{"target_duration_sec": tc.target}}, "按这个制作视频", model)
		params, _ := plan["params"].(map[string]interface{})
		if plan["intent"] != tc.intent || plan["needs_confirm"] != true || params["duration"] != tc.seconds || params["target_duration_sec"] != tc.target {
			t.Fatalf("target=%d maximum=%d: %#v", tc.target, tc.maximum, plan)
		}
		if tc.intent == "workflow" && params["storyboard_grid"] != tc.count {
			t.Fatalf("wrong segment count: %#v", plan)
		}
		if err := validateCreativeVideoExecution(model, params, tc.intent == "workflow"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreativeAgentRejectsStaleDurationAndUnknownCapabilities(t *testing.T) {
	model := &service.ModelFull{ModelDTO: service.ModelDTO{Code: "eight", InputSchema: map[string]interface{}{"properties": map[string]interface{}{"duration": map[string]interface{}{"enum": []interface{}{8}}}}}}
	plan := prepareCreativeAgentVideoPlan(map[string]interface{}{"intent": "video", "params": map[string]interface{}{"target_duration_sec": 8}}, "生成15秒视频", model)
	params := plan["params"].(map[string]interface{})
	if params["target_duration_sec"] != 15 || params["storyboard_grid"] != 2 {
		t.Fatalf("stale planner duration won: %#v", plan)
	}
	if err := validateCreativeVideoExecution(model, params, false); err == nil {
		t.Fatal("direct generation must not silently reduce 15 seconds to 8")
	}
	params["storyboard_grid"] = 4
	if err := validateCreativeVideoExecution(model, params, true); err == nil {
		t.Fatal("changed confirmation plan accepted")
	}
	model.InputSchema = nil
	if got := prepareCreativeAgentVideoPlan(plan, "生成15秒视频", model); got["intent"] != "clarify" {
		t.Fatalf("unknown capability guessed: %#v", got)
	}
}

func TestCreativeAgentVideoParametersFollowSelectedModel(t *testing.T) {
	model := &service.ModelFull{ModelDTO: service.ModelDTO{
		Code: "capability-model",
		InputSchema: map[string]interface{}{"properties": map[string]interface{}{
			"duration":   map[string]interface{}{"enum": []interface{}{8}},
			"resolution": map[string]interface{}{"enum": []interface{}{"480P", "720P"}},
			"ratio":      map[string]interface{}{"enum": []interface{}{"16:9", "9:16"}},
		}},
	}}
	valid := map[string]interface{}{"target_duration_sec": 8, "quality": "720p", "aspect_ratio": "9:16"}
	plan := prepareCreativeAgentVideoPlan(map[string]interface{}{"intent": "video", "params": valid}, "", model)
	if plan["intent"] != "video" || plan["needs_confirm"] != true {
		t.Fatalf("supported parameters rejected: %#v", plan)
	}
	if err := validateCreativeVideoExecution(model, plan["params"].(map[string]interface{}), false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value, label string }{
		{"quality", "1080p", "画质"},
		{"aspect_ratio", "1:1", "画幅"},
	} {
		params := map[string]interface{}{"target_duration_sec": 8, tc.key: tc.value}
		got := prepareCreativeAgentVideoPlan(map[string]interface{}{"intent": "video", "params": params}, "", model)
		if got["intent"] != "clarify" || got["needs_confirm"] != false || got["invalid_field"] != tc.key || !strings.Contains(stringAny(got["reply"]), tc.label) {
			t.Fatalf("unsupported %s executable: %#v", tc.key, got)
		}
		if err := validateCreativeVideoExecution(model, params, false); err == nil {
			t.Fatalf("execution bypassed unsupported %s", tc.key)
		}
	}
}

func TestCreativeAgentRequiresConfirmationBeforeSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{} // Any service access would panic: guards must run first.
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{"generate", h.CreativeAgentGenerate, `{"media_type":"video","prompt":"test"}`},
		{"workflow", h.CreativeAgentRunWorkflow, `{"workflow_code":"ai_comic_drama","prompt":"test"}`},
		{"retry", h.RetryAgentProject, `{"conversation_id":"conv_test"}`},
		{"retry-node", h.RetryAgentProjectNode, `{"conversation_id":"conv_test","node_id":"compose"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/test", tc.handler)
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != 400 || !strings.Contains(response.Body.String(), "确认") {
				t.Fatalf("unconfirmed request was not rejected: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
