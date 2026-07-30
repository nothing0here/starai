package main

import (
	"math"
	"testing"
)

func TestEstimateDynamicPriceRuleCostWorkerSeedance2(t *testing.T) {
	rule := map[string]interface{}{
		"billing_type": "dynamic",
		"strategy":     "seedance_2_tokens",
		"tokens_per_second": map[string]interface{}{
			"720p": float64(21600),
		},
		"rates_per_m_tokens": map[string]interface{}{
			"720p": map[string]interface{}{
				"without_video": float64(46),
				"with_video":    float64(28),
			},
		},
		"video_min_token_multiplier": float64(1.8),
	}

	imageCost := estimatePriceRuleCostWorker(rule, map[string]interface{}{
		"resolution":      "720p",
		"duration":        float64(5),
		"generation_mode": "image",
	}, 0, 0)
	wantImage := float64(5*21600) / 1_000_000 * 46
	if math.Abs(imageCost-wantImage) > 0.000001 {
		t.Fatalf("image cost = %f, want %f", imageCost, wantImage)
	}

	videoCost := estimatePriceRuleCostWorker(rule, map[string]interface{}{
		"resolution":                       "720p",
		"duration":                         float64(5),
		"generation_mode":                  "video",
		"reference_videos":                 []string{"https://example.test/input.mp4"},
		"reference_video_duration_seconds": float64(4),
	}, 0, 0)
	wantVideo := float64(9*21600) / 1_000_000 * 28
	if math.Abs(videoCost-wantVideo) > 0.000001 {
		t.Fatalf("video cost = %f, want %f", videoCost, wantVideo)
	}
}

func TestApplyAgentModelDefaultsInfersSeedanceMode(t *testing.T) {
	input := map[string]interface{}{"reference_images": []string{"https://example.test/frame.png"}}
	defaults := map[string]interface{}{"resolution": "720p", "generation_mode": "text"}
	runtimeRule := map[string]interface{}{
		"video": map[string]interface{}{
			"upload_profile": "seedance_2",
			"mode_param":     "generation_mode",
		},
	}

	applyAgentModelDefaults(input, defaults, runtimeRule, "video")
	if input["resolution"] != "720p" {
		t.Fatalf("resolution default not applied: %#v", input)
	}
	if input["generation_mode"] != "image" {
		t.Fatalf("generation mode = %#v, want image", input["generation_mode"])
	}

	explicit := map[string]interface{}{"generation_mode": "text", "reference_images": []string{"https://example.test/frame.png"}}
	applyAgentModelDefaults(explicit, defaults, runtimeRule, "video")
	if explicit["generation_mode"] != "text" {
		t.Fatalf("explicit generation mode should be preserved: %#v", explicit)
	}
}
