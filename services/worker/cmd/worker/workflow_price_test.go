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

func TestEstimateDynamicPriceRuleCostWorkerMiniMaxH3(t *testing.T) {
	rule := map[string]interface{}{
		"billing_type":                "dynamic",
		"strategy":                    "minimax_h3_seconds",
		"default_resolution":          "2K",
		"default_input_video_seconds": float64(4),
		"free_reference_images":       float64(5),
		"excess_image_price":          float64(0.2),
		"rates_per_second": map[string]interface{}{
			"2k": float64(0.8),
		},
	}
	got := estimatePriceRuleCostWorker(rule, map[string]interface{}{
		"resolution":                       "2K",
		"duration":                         float64(5),
		"reference_videos":                 []interface{}{"https://example.test/input.mp4"},
		"reference_video_duration_seconds": float64(8),
		"reference_images":                 []interface{}{"1", "2", "3", "4", "5", "6", "7"},
	}, 0, 0)
	want := float64(13)*0.8 + float64(2)*0.2
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("MiniMax-H3 cost = %f, want %f", got, want)
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

func TestSelectWorkflowActualCostDoesNotDoubleChargeFlatPrice(t *testing.T) {
	if got := selectWorkflowActualCost(0.15, 0.15, 0.12); got != 0.15 {
		t.Fatalf("cost = %v, want flat price 0.15", got)
	}
}

func TestWorkerTokenReservationUsesRequestedMaxTokens(t *testing.T) {
	rule := map[string]interface{}{
		"billing_type":       "per_token",
		"input_price_per_m":  float64(2),
		"output_price_per_m": float64(8),
	}
	got := estimatePriceRuleCostWorker(rule, map[string]interface{}{
		"_estimated_input_tokens": float64(1000),
		"max_tokens":              float64(4000),
	}, 0, 0)
	want := float64(1000)*2/1_000_000 + float64(4000)*8/1_000_000
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("cost = %.9f, want %.9f", got, want)
	}
}

func TestIncrementalWorkflowChargeDoesNotDoubleChargeSettledCost(t *testing.T) {
	if got := incrementalChargeAmount(1.25, 0.4); math.Abs(got-0.85) > 0.000000001 {
		t.Fatalf("incremental charge = %v, want 0.85", got)
	}
	if got := incrementalChargeAmount(0.4, 0.4); got != 0 {
		t.Fatalf("incremental charge = %v, want 0", got)
	}
}

func TestWorkerPerSecondPricingSupportsSecondsString(t *testing.T) {
	rule := map[string]interface{}{"billing_type": "per_second", "unit_price": float64(0.5)}
	if got := estimatePriceRuleCostWorker(rule, map[string]interface{}{"seconds": "8s"}, 0, 0); got != 4 {
		t.Fatalf("cost = %v, want 4", got)
	}
}

func TestUpstreamUsageTokensSupportsNestedUsage(t *testing.T) {
	prompt, output := upstreamUsageTokens([]byte(`{"data":{"result":{"usage":{"input_tokens":120,"output_tokens":45}}}}`))
	if prompt != 120 || output != 45 {
		t.Fatalf("usage = %d/%d, want 120/45", prompt, output)
	}
}
