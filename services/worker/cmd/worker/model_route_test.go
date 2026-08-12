package main

import (
	"strings"
	"testing"
)

func TestWorkerStatusCanFailover(t *testing.T) {
	for _, status := range []int{0, 401, 429, 500, 502, 503, 524} {
		if !workerStatusCanFailover(status) {
			t.Fatalf("status %d should fail over", status)
		}
	}
	for _, status := range []int{400, 422} {
		if workerStatusCanFailover(status) {
			t.Fatalf("status %d must not fail over", status)
		}
	}
}

func TestWorkerSameRouteRetryClassification(t *testing.T) {
	if workerShouldRetrySameRoute(nil, 429) {
		t.Fatal("429 should switch routes without retrying the same one")
	}
	if !workerShouldRetrySameRoute(nil, 503) {
		t.Fatal("503 should allow configured same-route retry")
	}
	if !workerShouldRetrySameRoute(assertionError("network"), 0) {
		t.Fatal("network errors should allow configured retry")
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }

func TestWorkerRouteProviderCost(t *testing.T) {
	route := workerModelRoute{CostRule: map[string]interface{}{"billing_type": "per_second", "unit_cost": 0.04}}
	if got := workerRouteProviderCost(route, map[string]interface{}{"duration": 10}, 0, 0); got != 0.4 {
		t.Fatalf("provider cost = %v, want 0.4", got)
	}
}

func TestBuildWorkerLLMRequestNativeProtocols(t *testing.T) {
	claudeBody, claudeEndpoint := buildWorkerLLMRequest(workerModelRoute{Protocol: "claude", UpstreamModel: "claude-test"}, "", "fallback", "system", "hello", 0.2)
	if claudeEndpoint != "/v1/messages" || claudeBody["system"] != "system" || claudeBody["max_tokens"] == nil {
		t.Fatalf("unexpected Claude request: endpoint=%s body=%#v", claudeEndpoint, claudeBody)
	}
	geminiBody, geminiEndpoint := buildWorkerLLMRequest(workerModelRoute{Protocol: "gemini", UpstreamModel: "gemini-test"}, "", "fallback", "system", "hello", 0.2)
	if !strings.Contains(geminiEndpoint, "{model}:generateContent") || geminiBody["contents"] == nil || geminiBody["systemInstruction"] == nil {
		t.Fatalf("unexpected Gemini request: endpoint=%s body=%#v", geminiEndpoint, geminiBody)
	}
}

func TestExtractNativeLLMResponsesAndUsage(t *testing.T) {
	claude := []byte(`{"content":[{"type":"text","text":"Claude answer"}],"usage":{"input_tokens":10,"output_tokens":3}}`)
	if got := extractLLMText(claude); got != "Claude answer" {
		t.Fatalf("Claude text = %q", got)
	}
	if prompt, output := chatUsageTokens(claude); prompt != 10 || output != 3 {
		t.Fatalf("Claude usage = %d/%d", prompt, output)
	}
	gemini := []byte(`{"candidates":[{"content":{"parts":[{"text":"Gemini answer"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":4}}`)
	if got := extractLLMText(gemini); got != "Gemini answer" {
		t.Fatalf("Gemini text = %q", got)
	}
	if prompt, output := chatUsageTokens(gemini); prompt != 11 || output != 4 {
		t.Fatalf("Gemini usage = %d/%d", prompt, output)
	}
}
