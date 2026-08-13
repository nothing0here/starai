package handler

import "testing"

func TestBuildOpenAIStreamPayloadPreservesReasoningDelta(t *testing.T) {
	payload := buildOpenAIStreamPayload("req_1", "nvidia/nemotron", map[string]interface{}{
		"reasoning_content": "Analyze the request first.",
		"content":           "Final answer.",
	}, "", nil)
	choices, ok := payload["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("choices=%#v", payload["choices"])
	}
	choice := choices[0].(map[string]interface{})
	delta := choice["delta"].(map[string]interface{})
	if delta["reasoning_content"] != "Analyze the request first." {
		t.Fatalf("reasoning delta=%#v", delta)
	}
	if delta["content"] != "Final answer." {
		t.Fatalf("content delta=%#v", delta)
	}
}
