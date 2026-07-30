package service

import (
	"math"
	"testing"
)

func TestEstimateSeedance2TokenCost(t *testing.T) {
	rule := map[string]interface{}{
		"billing_type":                "dynamic",
		"strategy":                    "seedance_2_tokens",
		"points_per_cny":              float64(1),
		"platform_multiplier":         float64(1),
		"default_input_video_seconds": float64(4),
		"video_min_token_multiplier":  float64(1.8),
		"tokens_per_second": map[string]interface{}{
			"480p":  float64(10044),
			"720p":  float64(21600),
			"1080p": float64(48600),
			"4k":    float64(194400),
		},
		"rates_per_m_tokens": map[string]interface{}{
			"480p":  map[string]interface{}{"without_video": float64(46), "with_video": float64(28)},
			"720p":  map[string]interface{}{"without_video": float64(46), "with_video": float64(28)},
			"1080p": map[string]interface{}{"without_video": float64(51), "with_video": float64(31)},
			"4k":    map[string]interface{}{"without_video": float64(26), "with_video": float64(16)},
		},
	}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   float64
	}{
		{
			name:   "720p five seconds without video",
			params: map[string]interface{}{"resolution": "720p", "duration": float64(5), "generation_mode": "image"},
			want:   4.968,
		},
		{
			name:   "480p five seconds without video",
			params: map[string]interface{}{"resolution": "480p", "duration": float64(5), "generation_mode": "text"},
			want:   2.31012,
		},
		{
			name: "720p video input applies official minimum token floor",
			params: map[string]interface{}{
				"resolution": "720p", "duration": float64(5), "generation_mode": "video",
				"reference_video_duration_seconds": float64(2),
			},
			want: 5.4432,
		},
		{
			name: "720p fifteen second video input includes its duration",
			params: map[string]interface{}{
				"resolution": "720p", "duration": float64(5), "generation_mode": "video",
				"reference_video_duration_seconds": float64(15),
			},
			want: 12.096,
		},
		{
			name:   "1080p uses its own token rate",
			params: map[string]interface{}{"resolution": "1080p", "duration": float64(5), "generation_mode": "image_audio"},
			want:   12.393,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateDynamicCost(rule, tt.params)
			if math.Abs(got-tt.want) > 0.000001 {
				t.Fatalf("estimateDynamicCost() = %.6f, want %.6f", got, tt.want)
			}
		})
	}
}
