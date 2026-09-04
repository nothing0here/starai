package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/starai/api/internal/runtime"
	"strings"
)

func agentMemoryText(role, content string) string {
	if role != "assistant" {
		return content
	}
	var plan map[string]interface{}
	if json.Unmarshal([]byte(content), &plan) == nil {
		if reply, ok := plan["reply"].(string); ok {
			return reply
		}
	}
	return content
}

func agentMemoryClip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

// ponytail: bounded extractive memory avoids a second paid model call. Exact
// requirements live in slots; add semantic summaries only if excerpts fall short.
func buildAgentMemory(history []runtime.ChatMessage, latest string, p AgentPolicy) ([]runtime.ChatMessage, string) {
	dialogue := []runtime.ChatMessage{}
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			m.Content = agentMemoryText(m.Role, m.Content)
			dialogue = append(dialogue, m)
		}
	}
	split := len(dialogue) - p.RecentMessages
	if split < 0 {
		split = 0
	}
	excerpts := []string{}
	for _, m := range dialogue[:split] {
		excerpts = append(excerpts, m.Role+": "+agentMemoryClip(m.Content, 180))
	}
	summary := strings.Join(excerpts, "\n")
	if r := []rune(summary); len(r) > p.SummaryChars {
		summary = "…\n" + string(r[len(r)-p.SummaryChars:])
	}
	recent := append([]runtime.ChatMessage{}, dialogue[split:]...)
	for i := range recent {
		recent[i].Content = agentMemoryClip(recent[i].Content, 12000)
	}
	if len(recent) == 0 || recent[len(recent)-1].Role != "user" || strings.TrimSpace(recent[len(recent)-1].Content) != strings.TrimSpace(latest) {
		recent = append(recent, runtime.ChatMessage{Role: "user", Content: latest})
	}
	return recent, summary
}

func (s *ChatService) AgentContext(ctx context.Context, userID int64, conversationID, latest string, p AgentPolicy) ([]runtime.ChatMessage, string, error) {
	// Ownership check is independent of whether this conversation has any messages.
	draft, err := s.GetAgentDraft(ctx, userID, conversationID)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.db.Query(ctx, `SELECT role,content FROM (SELECT cm.id,cm.role,cm.content FROM conversation_messages cm JOIN conversations c ON c.id=cm.conversation_id WHERE c.public_id=$1 AND c.user_id=$2 ORDER BY cm.id DESC LIMIT 128) recent ORDER BY id`, conversationID, userID)
	if err != nil {
		return nil, "", err
	}
	history := []runtime.ChatMessage{}
	kind, ref := draft.ExecutionKind, draft.ExecutionRef
	for rows.Next() {
		var role, content string
		if err = rows.Scan(&role, &content); err != nil {
			rows.Close()
			return nil, "", err
		}
		history = append(history, runtime.ChatMessage{Role: role, Content: content})
		if role == "system" {
			var event map[string]interface{}
			if json.Unmarshal([]byte(content), &event) == nil {
				if event["type"] == "creative_agent_workflow" {
					kind = "workflow"
					ref = stringValue(event["project_id"])
				}
				if event["type"] == "creative_agent_generation" {
					kind = "generation"
					ref = stringValue(event["task_no"])
				}
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	messages, summary := buildAgentMemory(history, latest, p)
	if ref != "" {
		var status, step string
		query := `SELECT status,'' FROM tasks WHERE task_no=$1 AND user_id=$2`
		if kind == "workflow" {
			query = `SELECT status,COALESCE(outputs->>'current_step','') FROM workflow_projects WHERE public_id=$1 AND user_id=$2`
		}
		if err = s.db.QueryRow(ctx, query, ref, userID).Scan(&status, &step); err == nil {
			summary += fmt.Sprintf("\n服务端最新任务：%s %s，状态=%s，步骤=%s。失败时应续接原项目，不得声称已重新开始或已完成。", kind, ref, status, step)
		}
	}
	return messages, summary, nil
}
