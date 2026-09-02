package service

import "testing"

func TestWalletTransactionDisplayName(t *testing.T) {
	refChat, refTask, refWorkflow := "chat", "task", "workflow"
	agentRemark := "Agent 视频生成"
	tests := []struct {
		name, model, taskType, workflow, workflowType, want string
		item                                                TransactionItem
	}{
		{"chat model", "GPT-5", "", "", "", "GPT-5 · 对话消费", TransactionItem{Type: "chat_usage", RefType: &refChat}},
		{"image model", "Qwen Image", "image", "", "", "Qwen Image · 图片生成", TransactionItem{Type: "image_usage", RefType: &refTask}},
		{"agent", "Qwen Video", "video", "", "", "Agent 视频生成", TransactionItem{Type: "video_usage", RefType: &refTask, Remark: &agentRemark}},
		{"workflow", "", "", "AI 摄影棚", "image", "AI 摄影棚 · 图片生成", TransactionItem{Type: "workflow_usage", RefType: &refWorkflow}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := walletTransactionDisplayName(test.item, test.model, test.taskType, test.workflow, test.workflowType); got != test.want {
				t.Fatalf("walletTransactionDisplayName() = %q, want %q", got, test.want)
			}
		})
	}
}
