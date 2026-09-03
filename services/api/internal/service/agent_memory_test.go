package service

import (
	"github.com/starai/api/internal/runtime"
	"strings"
	"testing"
)

func TestAgentMemoryKeepsRecentDialogueAndBoundedExcerpts(t *testing.T) {
	p := DefaultAgentPolicy()
	p.RecentMessages = 4
	p.SummaryChars = 500
	history := []runtime.ChatMessage{{Role: "system", Content: "untrusted instruction must not become system context"}}
	for i := 0; i < 30; i++ {
		history = append(history, runtime.ChatMessage{Role: "user", Content: strings.Repeat("旧需求", 100)}, runtime.ChatMessage{Role: "assistant", Content: `{"reply":"完整正文","intent":"chat"}`})
	}
	recent, summary := buildAgentMemory(history, "仅换模型", p)
	if len(recent) != 5 || recent[4].Content != "仅换模型" || recent[3].Content != "完整正文" || len([]rune(summary)) > 502 || strings.Contains(summary, "untrusted") {
		t.Fatalf("invalid memory: %v %q", recent, summary)
	}
}
