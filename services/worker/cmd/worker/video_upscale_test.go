package main

import "testing"

func TestNormalizeUpscaleResolution(t *testing.T) {
	tests := map[string]string{
		"720p":      "720P",
		"1280x720":  "720P",
		"1080P":     "1K",
		"1920x1080": "1K",
		"1440p":     "2K",
		"2560X1440": "2K",
		"4K":        "",
	}
	for input, expected := range tests {
		if actual := normalizeUpscaleResolution(input); actual != expected {
			t.Fatalf("normalizeUpscaleResolution(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestUpscaleResolutionAllowed(t *testing.T) {
	if !upscaleResolutionAllowed("1K", []interface{}{"720P", "1K"}) {
		t.Fatal("1K should be allowed")
	}
	if upscaleResolutionAllowed("2K", []interface{}{"720P", "1K"}) {
		t.Fatal("2K should not be allowed")
	}
}

func TestFirstWorkerURL(t *testing.T) {
	if got := firstWorkerURL([]interface{}{map[string]interface{}{"url": "https://example.com/source.mp4"}}); got != "https://example.com/source.mp4" {
		t.Fatalf("unexpected URL: %q", got)
	}
}

func TestClampFloat(t *testing.T) {
	if got := clampFloat(1.4, 0, 1); got != 1 {
		t.Fatalf("expected upper clamp, got %v", got)
	}
	if got := clampFloat(-0.2, 0, 1); got != 0 {
		t.Fatalf("expected lower clamp, got %v", got)
	}
	if got := firstPositiveFloat(0, -1, 0.65); got != 0.65 {
		t.Fatalf("expected first positive value, got %v", got)
	}
}

func TestJoinWorkflowInstruction(t *testing.T) {
	got := joinWorkflowInstruction("preserve identity", "anime style")
	if got != "preserve identity\n\nUser requirements:\nanime style" {
		t.Fatalf("unexpected combined prompt: %q", got)
	}
}
