package service

import (
	"strings"
	"testing"
)

func TestValidateSeedance2InputCombinations(t *testing.T) {
	cfg := videoRuntimeConfig{
		UploadProfile:      "seedance_2",
		MaxReferenceImages: 9,
		FirstFrameKey:      "first_frame",
		LastFrameKey:       "last_frame",
		ReferenceImagesKey: "reference_images",
		ReferenceVideosKey: "reference_videos",
		MaxReferenceVideos: 3,
		ReferenceAudiosKey: "reference_audios",
		MaxReferenceAudios: 3,
		ModeParam:          "generation_mode",
	}
	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr string
	}{
		{
			name:   "text",
			params: map[string]interface{}{"generation_mode": "text", "prompt": "一只猫跑过街道"},
		},
		{
			name: "all multimodal",
			params: map[string]interface{}{
				"generation_mode":  "image_video_audio",
				"reference_images": []interface{}{"https://example.com/a.jpg"},
				"reference_videos": []interface{}{"https://example.com/a.mp4"},
				"reference_audios": []interface{}{"https://example.com/a.mp3"},
			},
		},
		{
			name: "authorized portrait can satisfy image input",
			params: map[string]interface{}{
				"generation_mode":     "image",
				"portrait_asset_id":   "asset://authorized-person",
				"portrait_asset_type": "image",
			},
		},
		{
			name: "portrait must use asset ID",
			params: map[string]interface{}{
				"generation_mode":     "image",
				"portrait_asset_id":   "https://example.com/person.jpg",
				"portrait_asset_type": "image",
			},
			wantErr: "asset://",
		},
		{
			name: "audio cannot stand alone",
			params: map[string]interface{}{
				"generation_mode":  "video_audio",
				"reference_audios": []interface{}{"https://example.com/a.mp3"},
			},
			wantErr: "同时上传视频和音频",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVideoUpload(cfg, tt.params)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVideoParamsPreservesProviderDurationEnumType(t *testing.T) {
	veo := &ModelFull{ModelDTO: ModelDTO{
		InputSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"duration": map[string]interface{}{"enum": []interface{}{"4s", "8s", "12s"}},
			},
		},
	}}
	veoParams := map[string]interface{}{"duration": float64(4)}
	if err := ValidateVideoParams(veo, veoParams); err != nil {
		t.Fatalf("Veo equivalent duration should be normalized: %v", err)
	}
	if got := veoParams["duration"]; got != "4s" {
		t.Fatalf("Veo duration=%#v, want string 4s", got)
	}

	seedance := &ModelFull{ModelDTO: ModelDTO{
		InputSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"duration": map[string]interface{}{"enum": []interface{}{float64(5), float64(8), float64(10)}},
			},
		},
	}}
	seedanceParams := map[string]interface{}{"duration": "8s"}
	if err := ValidateVideoParams(seedance, seedanceParams); err != nil {
		t.Fatalf("Seedance equivalent duration should be normalized: %v", err)
	}
	if got := seedanceParams["duration"]; got != float64(8) {
		t.Fatalf("Seedance duration=%#v, want numeric 8", got)
	}
}

func TestValidateVeoReferenceModes(t *testing.T) {
	cfg := videoRuntimeConfig{
		UploadProfile:      "veo_reference",
		MaxReferenceImages: 3,
		ReferenceImagesKey: "reference_images",
		ModeParam:          "generation_mode",
	}
	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr string
	}{
		{
			name:   "text",
			params: map[string]interface{}{"generation_mode": "text", "prompt": "海边日落"},
		},
		{
			name: "reference",
			params: map[string]interface{}{
				"generation_mode":  "reference",
				"prompt":           "让画面产生自然运动",
				"reference_images": []interface{}{"https://example.com/a.jpg"},
			},
		},
		{
			name:    "reference requires image",
			params:  map[string]interface{}{"generation_mode": "reference", "prompt": "animate"},
			wantErr: "至少需要 1 张",
		},
		{
			name: "reference max three",
			params: map[string]interface{}{
				"generation_mode": "reference",
				"reference_images": []interface{}{
					"https://example.com/1.jpg",
					"https://example.com/2.jpg",
					"https://example.com/3.jpg",
					"https://example.com/4.jpg",
				},
			},
			wantErr: "最多支持 3 张",
		},
		{
			name:    "unsupported mode",
			params:  map[string]interface{}{"generation_mode": "first_last", "prompt": "animate"},
			wantErr: "仅支持文生或参考图",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVideoUpload(cfg, tt.params)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateOmniReferenceModes(t *testing.T) {
	cfg := videoRuntimeConfig{
		UploadProfile:      "omni_reference",
		MaxReferenceImages: 7,
		ReferenceImagesKey: "reference_images",
		ModeParam:          "generation_mode",
	}
	seven := []interface{}{
		"https://example.com/1.jpg", "https://example.com/2.jpg", "https://example.com/3.jpg",
		"https://example.com/4.jpg", "https://example.com/5.jpg", "https://example.com/6.jpg",
		"https://example.com/7.jpg",
	}
	if err := validateVideoUpload(cfg, map[string]interface{}{
		"generation_mode": "reference", "reference_images": seven,
	}); err != nil {
		t.Fatalf("seven Omni references should be valid: %v", err)
	}
	tooMany := append(append([]interface{}{}, seven...), "https://example.com/8.jpg")
	err := validateVideoUpload(cfg, map[string]interface{}{
		"generation_mode": "reference", "reference_images": tooMany,
	})
	if err == nil || !strings.Contains(err.Error(), "最多支持 7 张") {
		t.Fatalf("error = %v, want Omni seven-image limit", err)
	}
	err = validateVideoUpload(cfg, map[string]interface{}{"generation_mode": "first_last"})
	if err == nil || !strings.Contains(err.Error(), "暂不支持首尾帧") {
		t.Fatalf("error = %v, want first/last rejection", err)
	}
}

func TestValidateVeoFramePairRejectsReferenceImages(t *testing.T) {
	cfg := videoRuntimeConfig{
		UploadProfile:      "veo_frame_pair",
		FirstFrameKey:      "first_frame",
		LastFrameKey:       "last_frame",
		ReferenceImagesKey: "reference_images",
	}
	if err := validateVideoUpload(cfg, map[string]interface{}{
		"first_frame": "https://example.com/first.jpg",
	}); err != nil {
		t.Fatalf("first-frame-only request should be valid: %v", err)
	}
	if err := validateVideoUpload(cfg, map[string]interface{}{
		"first_frame": "https://example.com/first.jpg",
		"last_frame":  "https://example.com/last.jpg",
	}); err != nil {
		t.Fatalf("first/last-frame request should be valid: %v", err)
	}
	err := validateVideoUpload(cfg, map[string]interface{}{
		"first_frame":      "https://example.com/first.jpg",
		"reference_images": []interface{}{"https://example.com/reference.jpg"},
	})
	if err == nil || !strings.Contains(err.Error(), "不支持参考图") {
		t.Fatalf("error = %v, want reference-image rejection", err)
	}
	err = validateVideoUpload(cfg, map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "首帧") {
		t.Fatalf("error = %v, want required first frame", err)
	}
}
