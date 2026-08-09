package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatCompletionStreamRequestsUsage(t *testing.T) {
	var request map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 5, 5)
	chunks, err := client.ChatCompletionStreamWithConfig(context.Background(), "/chat", ChatRequest{Model: "test", Messages: []ChatMessage{{Role: "user", Content: "hello"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var usage *ChatUsage
	for chunk := range chunks {
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	options, _ := request["stream_options"].(map[string]interface{})
	if options["include_usage"] != true {
		t.Fatalf("stream_options = %#v, want include_usage=true", options)
	}
	if usage == nil || usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v, want total_tokens=15", usage)
	}
}
