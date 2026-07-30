package videoparams

import (
	"strconv"
	"testing"
)

func TestBuildUpstreamPayloadPreservesConfiguredVideoDurations(t *testing.T) {
	for _, seconds := range []int{5, 6, 8, 10, 12, 15} {
		t.Run(strconv.Itoa(seconds)+"s", func(t *testing.T) {
			got := BuildUpstreamVideoPayload(
				"video-configured-duration",
				"video-configured-duration",
				map[string]interface{}{
					"upstream": map[string]interface{}{
						"include": []interface{}{"duration"},
					},
				},
				nil,
				map[string]interface{}{
					"prompt":   "test",
					"duration": strconv.Itoa(seconds) + "s",
				},
			)
			got = SanitizeUpstreamPayload(got, "/v1/videos")
			duration, ok := got["duration"].(float64)
			if !ok || int(duration) != seconds {
				t.Fatalf("duration = %#v, want %d", got["duration"], seconds)
			}
		})
	}
}

func TestSanitizeUpstreamPayloadUsesImagesForVeo(t *testing.T) {
	payload := map[string]interface{}{
		"model":            "veo_3_1-fast-fl",
		"first_frame":      "https://example.com/first.jpg",
		"last_frame":       "https://example.com/last.jpg",
		"reference_images": []interface{}{"https://example.com/ref.jpg"},
		"orientation":      "portrait",
	}

	got := SanitizeUpstreamPayload(payload, "/v1/videos")
	images, ok := got["images"].([]string)
	if !ok {
		t.Fatalf("images = %#v, want []string", got["images"])
	}
	want := []string{"https://example.com/first.jpg", "https://example.com/last.jpg", "https://example.com/ref.jpg"}
	if len(images) != len(want) {
		t.Fatalf("images len = %d, want %d (%#v)", len(images), len(want), images)
	}
	for i := range want {
		if images[i] != want[i] {
			t.Fatalf("images[%d] = %q, want %q", i, images[i], want[i])
		}
	}
	if _, ok := got["image_url"]; ok {
		t.Fatalf("image_url should not be sent for Veo JSON")
	}
	if got["aspect_ratio"] != "9:16" {
		t.Fatalf("aspect_ratio = %#v, want 9:16", got["aspect_ratio"])
	}
}

func TestSanitizeUpstreamPayloadUsesImageURLForSora(t *testing.T) {
	payload := map[string]interface{}{
		"model":            "sora-2-12s",
		"reference_images": []interface{}{"https://example.com/ref.jpg", "https://example.com/ignored.jpg"},
		"orientation":      "landscape",
	}

	got := SanitizeUpstreamPayload(payload, "/v1/videos")
	if got["image_url"] != "https://example.com/ref.jpg" {
		t.Fatalf("image_url = %#v", got["image_url"])
	}
	if _, ok := got["images"]; ok {
		t.Fatalf("images should not be sent for Sora")
	}
	if got["aspect_ratio"] != "16:9" {
		t.Fatalf("aspect_ratio = %#v, want 16:9", got["aspect_ratio"])
	}
}

func TestSanitizeUpstreamPayloadDropsAnalysisOnlyFields(t *testing.T) {
	payload := map[string]interface{}{
		"model":           "sora-2-12s",
		"prompt":          "product video",
		"negative_prompt": "low quality",
		"selling_points":  []interface{}{"texture"},
		"user_intent":     "main visual",
		"asset_notes":     "reference image",
	}

	got := SanitizeUpstreamPayload(payload, "/v1/videos")
	for _, key := range []string{"negative_prompt", "selling_points", "user_intent", "asset_notes"} {
		if _, ok := got[key]; ok {
			t.Fatalf("%s should not be sent to video upstream: %#v", key, got)
		}
	}
}

func TestBuildUpstreamPayloadSupportsNestedMap(t *testing.T) {
	got := BuildUpstreamVideoPayload(
		"audio_minimax_speech_28_hd",
		"speech-2.8-hd",
		map[string]interface{}{
			"upstream": map[string]interface{}{
				"include": []interface{}{"voice_id", "speed", "format"},
				"map": map[string]interface{}{
					"prompt":   "text",
					"voice_id": "voice_setting.voice_id",
					"speed":    "voice_setting.speed",
					"format":   "audio_setting.format",
				},
				"static": map[string]interface{}{
					"stream":          false,
					"output_format":   "hex",
					"subtitle_enable": false,
					"voice_setting":   map[string]interface{}{"vol": float64(1), "pitch": float64(0)},
					"audio_setting":   map[string]interface{}{"sample_rate": float64(32000), "bitrate": float64(128000), "channel": float64(1)},
				},
			},
		},
		nil,
		map[string]interface{}{
			"prompt":   "hello",
			"voice_id": "male-qn-qingse",
			"speed":    1.15,
			"format":   "mp3",
		},
	)
	if got["text"] != "hello" || got["model"] != "speech-2.8-hd" || got["stream"] != false {
		t.Fatalf("unexpected top-level payload: %#v", got)
	}
	voice, ok := got["voice_setting"].(map[string]interface{})
	if !ok {
		t.Fatalf("voice_setting missing: %#v", got)
	}
	if voice["voice_id"] != "male-qn-qingse" || voice["speed"] != 1.15 {
		t.Fatalf("unexpected voice_setting: %#v", voice)
	}
	if voice["vol"] != float64(1) || voice["pitch"] != float64(0) {
		t.Fatalf("missing MiniMax official voice defaults: %#v", voice)
	}
	audio, ok := got["audio_setting"].(map[string]interface{})
	if !ok || audio["format"] != "mp3" {
		t.Fatalf("unexpected audio_setting: %#v", got)
	}
	if audio["sample_rate"] != float64(32000) || audio["bitrate"] != float64(128000) || audio["channel"] != float64(1) {
		t.Fatalf("missing MiniMax official audio defaults: %#v", audio)
	}
	if got["output_format"] != "hex" || got["subtitle_enable"] != false {
		t.Fatalf("missing MiniMax official response defaults: %#v", got)
	}
	if _, ok := got["response_format"]; ok {
		t.Fatalf("response_format should not be sent: %#v", got)
	}
}

func TestBuildUpstreamPayloadSupportsMinimaxMusicTemplate(t *testing.T) {
	got := BuildUpstreamVideoPayload(
		"audio_minimax_music_26",
		"music-2.6",
		map[string]interface{}{
			"upstream": map[string]interface{}{
				"include": []interface{}{"model_version", "music_prompt", "output_format", "format", "sample_rate", "bitrate"},
				"map": map[string]interface{}{
					"prompt":        "lyrics",
					"music_prompt":  "prompt",
					"model_version": "model",
					"format":        "audio_setting.format",
					"sample_rate":   "audio_setting.sample_rate",
					"bitrate":       "audio_setting.bitrate",
				},
				"static": map[string]interface{}{"stream": false},
			},
		},
		nil,
		map[string]interface{}{
			"prompt":        "[Verse] hello",
			"music_prompt":  "upbeat pop",
			"model_version": "music-2.6",
			"output_format": "hex",
			"format":        "mp3",
			"sample_rate":   44100,
			"bitrate":       256000,
		},
	)
	if got["model"] != "music-2.6" || got["lyrics"] != "[Verse] hello" || got["prompt"] != "upbeat pop" {
		t.Fatalf("unexpected MiniMax music payload: %#v", got)
	}
	if got["output_format"] != "hex" || got["stream"] != false {
		t.Fatalf("missing MiniMax music output settings: %#v", got)
	}
	audio, ok := got["audio_setting"].(map[string]interface{})
	if !ok || audio["format"] != "mp3" || audio["sample_rate"] != float64(44100) || audio["bitrate"] != float64(256000) {
		t.Fatalf("unexpected MiniMax music audio_setting: %#v", got)
	}
}

func TestBuildUpstreamPayloadSupportsCompatibleSpeechMusicTemplate(t *testing.T) {
	got := BuildUpstreamVideoPayload(
		"music-2-6-openai",
		"music-2.6",
		map[string]interface{}{
			"upstream": map[string]interface{}{
				"include": []interface{}{"music_prompt", "format", "sample_rate", "bitrate"},
				"map": map[string]interface{}{
					"prompt":       "metadata.lyrics",
					"music_prompt": "input",
					"format":       "response_format",
					"sample_rate":  "metadata.sample_rate",
					"bitrate":      "metadata.bitrate",
				},
			},
		},
		nil,
		map[string]interface{}{
			"prompt":       "[Chorus] hello",
			"music_prompt": "Mandopop, upbeat",
			"format":       "mp3",
			"sample_rate":  44100,
			"bitrate":      256000,
		},
	)
	if got["input"] != "Mandopop, upbeat" || got["response_format"] != "mp3" {
		t.Fatalf("unexpected compatible music payload: %#v", got)
	}
	metadata, ok := got["metadata"].(map[string]interface{})
	if !ok || metadata["lyrics"] != "[Chorus] hello" || metadata["sample_rate"] != float64(44100) || metadata["bitrate"] != float64(256000) {
		t.Fatalf("unexpected compatible music metadata: %#v", got)
	}
}

func TestBuildUpstreamPayloadSupportsVolcengineSeedance2(t *testing.T) {
	got := BuildUpstreamVideoPayload(
		"doubao-seedance-2",
		"doubao-seedance-2-0-260128",
		map[string]interface{}{
			"upstream": map[string]interface{}{
				"adapter": "volcengine_seedance_2",
				"include": []interface{}{"generation_mode", "duration", "ratio", "generate_audio", "portrait_asset_id", "portrait_asset_type", "reference_images", "reference_videos", "reference_audios"},
			},
		},
		nil,
		map[string]interface{}{
			"prompt":              "使用图片1的主体和视频1的运镜",
			"generation_mode":     "image_video_audio",
			"duration":            "8s",
			"ratio":               "16:9",
			"generate_audio":      true,
			"portrait_asset_id":   "asset://authorized-person",
			"portrait_asset_type": "image",
			"reference_images":    []interface{}{"https://example.com/a.jpg"},
			"reference_videos":    []interface{}{"https://example.com/a.mp4"},
			"reference_audios":    []interface{}{"https://example.com/a.mp3"},
		},
	)
	if got["model"] != "doubao-seedance-2-0-260128" || got["duration"] != float64(8) || got["ratio"] != "16:9" {
		t.Fatalf("unexpected Seedance payload: %#v", got)
	}
	content, ok := got["content"].([]interface{})
	if !ok || len(content) != 5 {
		t.Fatalf("content = %#v, want text + portrait + image + video + audio", got["content"])
	}
	portrait, _ := content[1].(map[string]interface{})
	image, _ := content[2].(map[string]interface{})
	video, _ := content[3].(map[string]interface{})
	audio, _ := content[4].(map[string]interface{})
	portraitURL, _ := portrait["image_url"].(map[string]interface{})
	if portraitURL["url"] != "asset://authorized-person" {
		t.Fatalf("unexpected portrait asset: %#v", portrait)
	}
	if image["role"] != "reference_image" || video["role"] != "reference_video" || audio["role"] != "reference_audio" {
		t.Fatalf("unexpected Seedance roles: %#v", content)
	}
	if _, ok := got["generation_mode"]; ok {
		t.Fatalf("generation_mode must not be sent upstream: %#v", got)
	}
}

func TestBuildSeedancePayloadDropsRelativeAndDuplicateMediaReferences(t *testing.T) {
	got := BuildUpstreamVideoPayload(
		"doubao-seedance-2",
		"doubao-seedance-2-0-260128",
		map[string]interface{}{
			"upstream": map[string]interface{}{
				"adapter": "volcengine_seedance_2",
				"include": []interface{}{"generation_mode", "reference_images"},
			},
		},
		nil,
		map[string]interface{}{
			"prompt":          "test",
			"generation_mode": "image",
			"reference_images": []interface{}{
				"https://cdn.example/keyframe.png",
				"/assets/comic-styles/cn-ancient.svg",
				"https://cdn.example/keyframe.png",
			},
		},
	)
	content, ok := got["content"].([]interface{})
	if !ok || len(content) != 2 {
		t.Fatalf("content=%#v, want text plus one valid image", got["content"])
	}
	image, _ := content[1].(map[string]interface{})
	imageURL, _ := image["image_url"].(map[string]interface{})
	if imageURL["url"] != "https://cdn.example/keyframe.png" {
		t.Fatalf("unexpected Seedance image reference: %#v", image)
	}
}
