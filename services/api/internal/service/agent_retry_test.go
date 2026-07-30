package service

import "testing"

func TestNormalizeComicAudioStrategy(t *testing.T) {
	if got := normalizeComicAudioStrategy("", "tts_model"); got != "hybrid" {
		t.Fatalf("strategy=%q, want hybrid", got)
	}
	if got := normalizeComicAudioStrategy("", ""); got != "video_native" {
		t.Fatalf("strategy=%q, want video_native", got)
	}
	if got := normalizeComicAudioStrategy("tts_only", "tts_model"); got != "tts_only" {
		t.Fatalf("strategy=%q, want tts_only", got)
	}
}

func TestPruneWorkflowOutputsForRetry(t *testing.T) {
	tests := []struct {
		node        string
		removed     []string
		preserved   []string
		currentStep string
	}{
		{node: "keyframes", removed: []string{"segments", "final_video_url", "media_tasks"}, preserved: []string{"comic_drama", "keyframes"}, currentStep: "keyframes"},
		{node: "video_segments", removed: []string{"final_video_url", "media_tasks"}, preserved: []string{"keyframes", "segments"}, currentStep: "video_segments"},
		{node: "narrations", removed: []string{"final_video_url", "thumbnail", "media_tasks"}, preserved: []string{"segments", "narrations"}, currentStep: "narrations"},
		{node: "compose", removed: []string{"final_video_url", "thumbnail", "media_tasks"}, preserved: []string{"segments", "narrations"}, currentStep: "compose"},
	}
	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			outputs := map[string]interface{}{
				"comic_drama": map[string]interface{}{},
				"keyframes": []interface{}{
					map[string]interface{}{"id": "S01", "image_url": "keyframe.jpg"},
					map[string]interface{}{"id": "S02", "status": "failed", "error_message": "failed"},
				},
				"segments": []interface{}{
					map[string]interface{}{"id": "S01", "video_url": "segment.mp4"},
					map[string]interface{}{"id": "S02", "status": "failed", "error_message": "failed"},
				},
				"narrations": []interface{}{
					map[string]interface{}{"id": "S01", "audio_url": "voice.mp3", "status": "succeeded"},
					map[string]interface{}{"id": "S02", "audio_url": "", "status": "skipped"},
					map[string]interface{}{"id": "S03", "status": "failed", "error_message": "failed"},
				},
				"final_video_url": "video", "thumbnail": "thumb", "media_tasks": []interface{}{1}, "current_step": "result",
			}
			pruneWorkflowOutputsForRetry(outputs, tt.node)
			for _, key := range tt.removed {
				if _, ok := outputs[key]; ok {
					t.Fatalf("expected %s to be removed", key)
				}
			}
			for _, key := range tt.preserved {
				if _, ok := outputs[key]; !ok {
					t.Fatalf("expected %s to be preserved", key)
				}
			}
			if outputs["current_step"] != tt.currentStep {
				t.Fatalf("current_step=%v, want %s", outputs["current_step"], tt.currentStep)
			}
			if tt.node == "keyframes" {
				items, _ := outputs["keyframes"].([]interface{})
				if len(items) != 1 {
					t.Fatalf("successful keyframes=%d, want 1", len(items))
				}
			}
			if tt.node == "video_segments" {
				items, _ := outputs["segments"].([]interface{})
				if len(items) != 1 {
					t.Fatalf("successful segments=%d, want 1", len(items))
				}
			}
			if tt.node == "narrations" {
				items, _ := outputs["narrations"].([]interface{})
				if len(items) != 2 {
					t.Fatalf("successful or skipped narrations=%d, want 2", len(items))
				}
			}
		})
	}
}

func TestComicAutoProjectNameUsesRuneSafeEllipsis(t *testing.T) {
	got := comicAutoProjectName("  一个   自动创建的漫剧项目名称，内容很长很长很长很长很长很长很长很长很长  ")
	runes := []rune(got)
	if len(runes) != 33 || runes[len(runes)-1] != '…' {
		t.Fatalf("unexpected auto project name %q (%d runes)", got, len(runes))
	}
}

func TestNormalizeComicAssetCodeUsesStableASCIIFallback(t *testing.T) {
	code := normalizeComicAssetCode("", "character_cda_123456789")
	if code != "CHARACTER_CDA_123456789" {
		t.Fatalf("code=%q", code)
	}
	if got := normalizeComicAssetCode("主角-01", "fallback"); got != "-01" && got != "01" {
		// Chinese labels are not used as identifiers; the explicit numeric suffix remains stable.
		t.Fatalf("unexpected normalized code %q", got)
	}
}
