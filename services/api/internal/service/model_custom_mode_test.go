package service

import "testing"

func TestNormalizeCustomMediaRequestMode(t *testing.T) {
	tests := map[string]string{
		"image": "images",
		"video": "video",
		"audio": "audio",
		"text":  "custom",
	}
	for category, want := range tests {
		if got := normalizeCustomMediaRequestMode("custom", category); got != want {
			t.Fatalf("category %s: got %q, want %q", category, got, want)
		}
	}
	if got := normalizeCustomMediaRequestMode("images", "image"); got != "images" {
		t.Fatalf("non-custom mode changed to %q", got)
	}
}
