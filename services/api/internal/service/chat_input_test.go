package service

import (
	"testing"

	"github.com/starai/api/internal/runtime"
)

func TestCompletionInputModelIdentifier(t *testing.T) {
	tests := []struct {
		name string
		in   CompletionInput
		want string
	}{
		{
			name: "model code has priority",
			in:   CompletionInput{Model: "upstream-model", ModelCode: "platform-code"},
			want: "platform-code",
		},
		{
			name: "openai compatible model fallback",
			in:   CompletionInput{Model: "deepseek-v4-flash"},
			want: "deepseek-v4-flash",
		},
		{
			name: "trim spaces",
			in:   CompletionInput{Model: "  deepseek-v4-flash  "},
			want: "deepseek-v4-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.modelIdentifier(); got != tt.want {
				t.Fatalf("modelIdentifier()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatRequestMessagesAddsMultimodalReferencesToLastUserMessage(t *testing.T) {
	result := chatRequestMessages(
		[]runtime.ChatMessage{
			{Role: "system", Content: "analyze"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "ok"},
			{Role: "user", Content: "inspect these"},
		},
		map[string]interface{}{
			"reference_images": []interface{}{"https://example.com/a.png"},
			"reference_videos": []interface{}{"https://example.com/a.mp4"},
		},
	)
	messages, ok := result.([]map[string]interface{})
	if !ok || len(messages) != 4 {
		t.Fatalf("unexpected multimodal messages: %#v", result)
	}
	content, ok := messages[3]["content"].([]map[string]interface{})
	if !ok || len(content) != 3 {
		t.Fatalf("unexpected user content: %#v", messages[3]["content"])
	}
	if content[0]["type"] != "text" || content[1]["type"] != "image_url" || content[2]["type"] != "video_url" {
		t.Fatalf("unexpected content order: %#v", content)
	}
	if messages[1]["content"] != "first" {
		t.Fatalf("earlier user message should remain text: %#v", messages[1])
	}
}
