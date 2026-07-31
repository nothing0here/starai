package service

import "testing"

func TestNormalizeAgentUpscaleResolution(t *testing.T) {
	if got := normalizeAgentUpscaleResolution("1920x1080"); got != "1K" {
		t.Fatalf("expected 1K, got %q", got)
	}
	if got := normalizeAgentUpscaleResolution("4K"); got != "" {
		t.Fatalf("unsupported resolution should be empty, got %q", got)
	}
}

func TestMergeVideoUpscaleRuntimeDefaults(t *testing.T) {
	inputs := mergeVideoUpscaleRuntimeDefaults(map[string]interface{}{
		"default_target_resolution": "2K",
		"preserve_audio":            false,
		"default_enhancement_mode":  "detail",
	}, map[string]interface{}{})
	if inputs["target_resolution"] != "2K" || inputs["preserve_audio"] != false || inputs["enhancement_mode"] != "detail" {
		t.Fatalf("unexpected defaults: %#v", inputs)
	}
	if inputs["count"] != 1 || inputs["n"] != 1 || inputs["_mode"] != "auto" {
		t.Fatalf("workflow control defaults missing: %#v", inputs)
	}
}

func TestMergeVideoRedrawRuntimeDefaults(t *testing.T) {
	inputs := mergeVideoRedrawRuntimeDefaults(map[string]interface{}{
		"default_style_strength": 0.7,
		"preserve_motion":        false,
	}, map[string]interface{}{})
	if inputs["style_strength"] != 0.7 || inputs["preserve_motion"] != false {
		t.Fatalf("unexpected redraw defaults: %#v", inputs)
	}
	if inputs["preserve_identity"] != true || inputs["preserve_audio"] != true {
		t.Fatalf("safe redraw defaults missing: %#v", inputs)
	}
}

func TestMergeSubtitleRemovalRuntimeDefaults(t *testing.T) {
	inputs := mergeSubtitleRemovalRuntimeDefaults(map[string]interface{}{
		"default_subtitle_mode":   "soft_track",
		"default_subtitle_region": "bottom_15",
		"protect_watermark":       false,
	}, map[string]interface{}{})
	if inputs["subtitle_mode"] != "soft_track" || inputs["subtitle_region"] != "bottom_15" || inputs["protect_watermark"] != false {
		t.Fatalf("unexpected subtitle defaults: %#v", inputs)
	}
}
