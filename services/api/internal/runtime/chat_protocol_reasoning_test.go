package runtime

import "testing"

func TestDecodeChatStreamEventPreservesOpenAIReasoningAndAnswer(t *testing.T) {
	event, err := decodeChatStreamEvent("openai", "", []byte(`{"choices":[{"delta":{"reasoning_content":"Consider the constraints. ","content":"The answer."}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.ReasoningContent != "Consider the constraints. " {
		t.Fatalf("reasoning=%q", event.ReasoningContent)
	}
	if event.Content != "The answer." {
		t.Fatalf("content=%q", event.Content)
	}
}

func TestOpenAIChatResponsePreservesReasoningContent(t *testing.T) {
	response, err := decodeChatResponse("openai", []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"Analyze first.","content":"Final answer."}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.Choices[0].Message.ReasoningContent != "Analyze first." {
		t.Fatalf("reasoning=%q", response.Choices[0].Message.ReasoningContent)
	}
}
