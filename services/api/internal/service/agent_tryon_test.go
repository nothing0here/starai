package service

import "testing"

func TestVirtualTryOnModelCompatible(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		upstream   string
		runtime    map[string]interface{}
		configured string
		want       bool
	}{
		{name: "nano banana", code: "nano_banana_pro-2K", want: true},
		{name: "gpt image 2 upstream", code: "custom", upstream: "gpt-image-2", want: true},
		{name: "gemini", code: "gemini-3.1-flash-image-preview", want: true},
		{name: "declared multi reference", code: "custom", runtime: map[string]interface{}{"image": map[string]interface{}{"max_reference_images": float64(5)}}, want: true},
		{name: "configured admin default", code: "custom", configured: "custom", want: true},
		{name: "plain text to image", code: "dall-e-3", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := virtualTryOnModelCompatible(tt.code, tt.upstream, tt.runtime, tt.configured); got != tt.want {
				t.Fatalf("compatible = %v, want %v", got, tt.want)
			}
		})
	}
}
