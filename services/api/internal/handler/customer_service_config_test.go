package handler

import (
	"strings"
	"testing"

	"github.com/starai/api/internal/runtime"
	"github.com/starai/api/internal/service"
)

func TestValidateCustomerServiceConfig(t *testing.T) {
	tests := []struct {
		name string
		req  map[string]interface{}
		want string
	}{
		{name: "builtin", req: map[string]interface{}{"customer_service_mode": "builtin"}},
		{name: "custom script", req: map[string]interface{}{"customer_service_mode": "custom_script", "customer_service_custom_script": "<script>window.chat = true;</script>"}},
		{name: "invalid mode", req: map[string]interface{}{"customer_service_mode": "other"}, want: "首页客服方式参数错误"},
		{name: "missing script tag", req: map[string]interface{}{"customer_service_custom_script": "window.chat = true;"}, want: "第三方客服脚本必须包含完整的 <script> 标签"},
		{name: "too large", req: map[string]interface{}{"customer_service_custom_script": "<script>" + strings.Repeat("x", 100*1024) + "</script>"}, want: "第三方客服脚本不能超过 100KB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateCustomerServiceConfig(tt.req); got != tt.want {
				t.Fatalf("validateCustomerServiceConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseCreativeAgentPlan(t *testing.T) {
	plan := parseCreativeAgentPlan("```json\n{\"intent\":\"image\",\"prompt\":\"一只猫\"}\n```")
	if plan == nil || plan["intent"] != "image" || plan["prompt"] != "一只猫" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if parseCreativeAgentPlan("not json") != nil {
		t.Fatal("expected invalid planner output to return nil")
	}
}

func TestNormalizeCreativeAgentMessages(t *testing.T) {
	messages := normalizeCreativeAgentMessages([]runtime.ChatMessage{
		{Role: "user", Content: "生成一张图片"},
		{Role: "assistant", Content: "已创建图片任务"},
		{Role: "assistant", Content: "生成完成"},
		{Role: "user", Content: "继续改成蓝色"},
	})
	if len(messages) != 3 || messages[1].Role != "assistant" || messages[1].Content != "已创建图片任务\n生成完成" || messages[2].Role != "user" {
		t.Fatalf("unexpected normalized messages: %#v", messages)
	}
}

func TestCreativeAgentSeparatesSpeechAndMusicModels(t *testing.T) {
	speech := &service.ModelFull{ModelDTO: service.ModelDTO{Code: "audio_tts", DisplayName: "Text to Speech"}, RequestMode: "audio"}
	music := &service.ModelFull{ModelDTO: service.ModelDTO{Code: "audio_music", DisplayName: "Music"}, RequestMode: "audio", RuntimeRule: map[string]interface{}{"audio": map[string]interface{}{"input_layout": "dual"}}}
	if !creativeAgentModelSupportsType(speech, "speech") || creativeAgentModelSupportsType(speech, "music") {
		t.Fatal("speech model classification failed")
	}
	if !creativeAgentModelSupportsType(music, "music") || creativeAgentModelSupportsType(music, "speech") {
		t.Fatal("music model classification failed")
	}
}
