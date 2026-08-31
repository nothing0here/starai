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
	tests := []struct {
		name    string
		content string
		intent  string
		reply   string
	}{
		{
			name:    "fenced json",
			content: "```json\n{\"intent\":\"image\",\"prompt\":\"一只猫\"}\n```",
			intent:  "image",
		},
		{
			name:    "preamble and fenced json",
			content: "下面是执行计划：\n```JSON\n{\"intent\":\"chat\",\"reply\":\"## 科技资讯\\n- 结论 [1]\"}\n```\n",
			intent:  "chat",
			reply:   "## 科技资讯\n- 结论 [1]",
		},
		{
			name:    "quoted json",
			content: `"{\"intent\":\"chat\",\"reply\":\"含有 {花括号} 的回答\"}"`,
			intent:  "chat",
			reply:   "含有 {花括号} 的回答",
		},
		{
			name:    "json followed by explanation",
			content: "{\"intent\":\"chat\",\"reply\":\"已整理\"}\n以上为搜索结果。",
			intent:  "chat",
			reply:   "已整理",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := parseCreativeAgentPlan(tt.content)
			if plan == nil || plan["intent"] != tt.intent {
				t.Fatalf("unexpected plan: %#v", plan)
			}
			if tt.reply != "" && plan["reply"] != tt.reply {
				t.Fatalf("reply = %#v, want %#v", plan["reply"], tt.reply)
			}
		})
	}
	if parseCreativeAgentPlan("not json") != nil {
		t.Fatal("expected invalid planner output to return nil")
	}
	if parseCreativeAgentPlan("{\"sources\":[1,2]}") != nil {
		t.Fatal("expected unrelated JSON object to return nil")
	}
}

func TestCreativeAgentPlanFromStream(t *testing.T) {
	chat := creativeAgentPlanFromStream("CHAT\n## 今日资讯\n- 第一条 [1]")
	if chat["intent"] != "chat" || chat["reply"] != "## 今日资讯\n- 第一条 [1]" {
		t.Fatalf("unexpected streamed chat plan: %#v", chat)
	}
	image := creativeAgentPlanFromStream("PLAN\n{\"intent\":\"image\",\"reply\":\"正在生成\",\"prompt\":\"一只猫\",\"params\":{}}")
	if image["intent"] != "image" || image["prompt"] != "一只猫" {
		t.Fatalf("unexpected streamed generation plan: %#v", image)
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
