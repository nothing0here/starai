package service

import (
	"math"
	"testing"
)

func TestEstimateMiniMaxH3Cost(t *testing.T) {
	rule := map[string]interface{}{
		"billing_type":                "dynamic",
		"strategy":                    "minimax_h3_seconds",
		"default_resolution":          "2K",
		"default_input_video_seconds": float64(4),
		"free_reference_images":       float64(5),
		"excess_image_price":          float64(0.2),
		"points_per_cny":              float64(1),
		"platform_multiplier":         float64(1),
		"rates_per_second": map[string]interface{}{
			"2k":   float64(0.8),
			"768p": float64(0.5),
		},
	}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   float64
	}{
		{
			name:   "2k output only",
			params: map[string]interface{}{"resolution": "2K", "duration": float64(5)},
			want:   4,
		},
		{
			name: "input video uses measured seconds",
			params: map[string]interface{}{
				"resolution": "2K", "duration": float64(5),
				"reference_videos":                 []interface{}{"https://example.com/ref.mp4"},
				"reference_video_duration_seconds": float64(8),
			},
			want: 10.4,
		},
		{
			name: "images over free allowance",
			params: map[string]interface{}{
				"resolution": "2K", "duration": float64(4),
				"reference_images": []interface{}{"1", "2", "3", "4", "5", "6", "7"},
			},
			want: 3.6,
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
