package main

import "testing"

func TestExtractSeedance2PollingResult(t *testing.T) {
	raw := map[string]interface{}{
		"id":     "cgt-test",
		"status": "succeeded",
		"content": map[string]interface{}{
			"video_url":      "https://example.com/result.mp4",
			"last_frame_url": "https://example.com/last.jpg",
		},
	}
	items := extractMediaItems(raw)
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one video", items)
	}
	if items[0].URL != "https://example.com/result.mp4" {
		t.Fatalf("video url = %q", items[0].URL)
	}
	if items[0].Thumbnail != "https://example.com/last.jpg" {
		t.Fatalf("last frame = %q", items[0].Thumbnail)
	}
}
