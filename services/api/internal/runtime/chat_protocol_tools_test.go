package runtime

import "testing"

func TestDecodeChatStreamEventPreservesOpenAIToolCalls(t *testing.T) {
	_, calls, _, _, err := decodeChatStreamEvent("openai", "", []byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"weather","arguments":"{\"city\":\"Sh"}}]}}]}`))
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls=%#v err=%v", calls, err)
	}
	if calls[0]["id"] != "call_1" {
		t.Fatalf("unexpected call: %#v", calls[0])
	}
}

func TestDecodeChatStreamEventPreservesNativeToolDeltas(t *testing.T) {
	_, claudeCalls, _, _, err := decodeChatStreamEvent("claude", "content_block_delta", []byte(`{"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Sh"}}`))
	if err != nil || len(claudeCalls) != 1 || claudeCalls[0]["partial_json"] != `{"city":"Sh` {
		t.Fatalf("claude calls=%#v err=%v", claudeCalls, err)
	}
	_, geminiCalls, _, _, err := decodeChatStreamEvent("gemini", "", []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{"city":"Shanghai"}}}]}}]}`))
	if err != nil || len(geminiCalls) != 1 {
		t.Fatalf("gemini calls=%#v err=%v", geminiCalls, err)
	}
}
