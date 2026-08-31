package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/starai/worker/internal/storage"
)

func TestOmniReferenceUsesManagedAssetBytes(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir(), "http://127.0.0.1:1/uploads-local")
	if err != nil {
		t.Fatal(err)
	}
	previousStore := objectStore
	objectStore = store
	defer func() { objectStore = previousStore }()

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
	assetURL, err := store.Upload(context.Background(), "assets/1/ref.png", "image/png", bytes.NewReader(png), int64(len(png)))
	if err != nil {
		t.Fatal(err)
	}
	normalized := normalizeReferenceImage(context.Background(), assetURL)
	if !strings.HasPrefix(normalized, "data:image/png;base64,") {
		t.Fatalf("managed reference was not embedded: %q", normalized)
	}
	payload := map[string]interface{}{"model": "omni_flash-10s", "images": []interface{}{normalized}}
	if err := validateOmniReferencePayload(payload); err != nil {
		t.Fatal(err)
	}
	payload["images"] = []interface{}{assetURL}
	if err := validateOmniReferencePayload(payload); err == nil {
		t.Fatal("unresolved Omni reference must not silently degrade to text-only generation")
	}
}

func TestParseUpstreamMediaKeepsTaskIDWhenMediaExists(t *testing.T) {
	body := []byte(`{
		"code": "success",
		"data": {
			"id": 4219436,
			"task_id": "task_8bSnSQRDipvCCp1iqew8z1H8y0mbFqoD",
			"status": "SUCCESS",
			"result_url": "https://otuapi.com/v1/videos/task_ijlim0lsyqb6Svhqgh1VkAQf1ys1nUne/content"
		}
	}`)

	items, upstreamID := parseUpstreamMedia(body)
	if upstreamID != "task_8bSnSQRDipvCCp1iqew8z1H8y0mbFqoD" {
		t.Fatalf("upstreamID = %q", upstreamID)
	}
	if len(items) != 1 || items[0].URL != "https://otuapi.com/v1/videos/task_ijlim0lsyqb6Svhqgh1VkAQf1ys1nUne/content" {
		t.Fatalf("items = %#v", items)
	}
}

func TestParseUpstreamMediaReadsBase64Audio(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString([]byte("ID3\x04audio-audio-audio-audio-audio-audio-audio-audio"))
	body := []byte(`{
		"data": {
			"audio": "` + audio + `",
			"format": "mp3"
		}
	}`)

	items, upstreamID := parseUpstreamMedia(body)
	if upstreamID != "" {
		t.Fatalf("upstreamID = %q", upstreamID)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].B64JSON != audio {
		t.Fatalf("audio base64 = %q", items[0].B64JSON)
	}
	if items[0].MimeType != "mp3" {
		t.Fatalf("mime/format = %q", items[0].MimeType)
	}
}

func TestParseUpstreamMediaReadsHexAudio(t *testing.T) {
	audio := hex.EncodeToString([]byte("ID3\x04audio-audio-audio-audio-audio-audio-audio-audio"))
	body := []byte(`{
		"data": {
			"audio": "` + audio + `",
			"format": "mp3"
		}
	}`)

	items, _ := parseUpstreamMedia(body)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].B64JSON != audio {
		t.Fatalf("audio hex = %q", items[0].B64JSON)
	}
	data, contentType, err := decodeEncodedMedia(items[0].B64JSON, items[0].MimeType, "audio")
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:3]) != "ID3" {
		t.Fatalf("decoded head = %q", string(data[:3]))
	}
	if contentType != "audio/mpeg" {
		t.Fatalf("contentType = %q", contentType)
	}
	dataURL := normalizeAudioResultURL(items[0].B64JSON, items[0].MimeType)
	if dataURL == "" {
		t.Fatal("data url fallback is empty")
	}
	if got, want := dataURL[:22], "data:audio/mpeg;base64"; got != want {
		t.Fatalf("data url prefix = %q, want %q", got, want)
	}
}

func TestParseUpstreamMediaReadsHexAudioFromDataString(t *testing.T) {
	audio := hex.EncodeToString([]byte("ID3\x04music-music-music-music-music-music-music"))
	body := []byte(`{"data":"` + audio + `","format":"mp3"}`)

	items, _ := parseUpstreamMedia(body)
	if len(items) != 1 || items[0].B64JSON != audio {
		t.Fatalf("items = %#v", items)
	}
}

func TestParseUpstreamMediaReadsHexAudioFromAudioFile(t *testing.T) {
	audio := hex.EncodeToString([]byte("ID3\x04music-music-music-music-music-music-music"))
	body := []byte(`{"data":{"audio_file":"` + audio + `","audio_format":"mp3"}}`)

	items, _ := parseUpstreamMedia(body)
	if len(items) != 1 || items[0].B64JSON != audio || items[0].MimeType != "mp3" {
		t.Fatalf("items = %#v", items)
	}
}

func TestParseUpstreamMediaReadsNestedAudioResult(t *testing.T) {
	audio := hex.EncodeToString([]byte("ID3\x04music-music-music-music-music-music-music"))
	body := []byte(`{"data":{"audio_result":{"audio":"` + audio + `","format":"mp3"}}}`)

	items, _ := parseUpstreamMedia(body)
	if len(items) != 1 || items[0].B64JSON != audio {
		t.Fatalf("items = %#v", items)
	}
}

func TestParseUpstreamMediaReadsPluralMediaLists(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"images", `{"data":{"images":[{"url":"https://example.com/1.png"},{"image_url":"https://example.com/2.png"}]}}`, []string{"https://example.com/1.png", "https://example.com/2.png"}},
		{"videos", `{"result":{"videos":[{"video_url":"https://example.com/1.mp4"}]}}`, []string{"https://example.com/1.mp4"}},
		{"audios", `{"output":{"audios":[{"audio_url":"https://example.com/1.mp3"}]}}`, []string{"https://example.com/1.mp3"}},
		{"results", `{"results":[{"url":"https://example.com/1.webp"}]}`, []string{"https://example.com/1.webp"}},
		{"nested plural list", `{"data":[{"files":[{"uri":"https://example.com/1.wav"}]}]}`, []string{"https://example.com/1.wav"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, _ := parseUpstreamMedia([]byte(tt.body))
			if len(items) != len(tt.want) {
				t.Fatalf("items = %#v", items)
			}
			for i, want := range tt.want {
				if items[i].URL != want {
					t.Fatalf("items[%d].URL = %q, want %q", i, items[i].URL, want)
				}
			}
		})
	}
}

func TestParseUpstreamMediaReadsTaskIDInsideArray(t *testing.T) {
	items, upstreamID := parseUpstreamMedia([]byte(`{"data":[{"task_id":"task-array-1","status":"queued"}]}`))
	if len(items) != 0 || upstreamID != "task-array-1" {
		t.Fatalf("items=%#v upstreamID=%q", items, upstreamID)
	}
}

func TestApplyRequestTransformUsesEffectiveConnection(t *testing.T) {
	payload := map[string]interface{}{"n": 1, "size": "1024x1024"}
	applyRequestTransform(payload, map[string]interface{}{
		"connection": map[string]interface{}{
			"request_transform": map[string]interface{}{"n": nil, "size": nil, "num_images": float64(2)},
		},
	})
	if _, ok := payload["n"]; ok {
		t.Fatalf("n was not removed: %#v", payload)
	}
	if _, ok := payload["size"]; ok {
		t.Fatalf("size was not removed: %#v", payload)
	}
	if payload["num_images"] != float64(2) {
		t.Fatalf("num_images was not applied: %#v", payload)
	}
}

func TestNormalizeWorkerMediaRequestMode(t *testing.T) {
	for category, want := range map[string]string{"image": "images", "video": "video", "audio": "audio", "text": "custom"} {
		if got := normalizeWorkerMediaRequestMode("custom", category); got != want {
			t.Fatalf("category %s: got %q, want %q", category, got, want)
		}
	}
}

func TestParseUpstreamMediaReadsRawMP3Audio(t *testing.T) {
	body := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x21TXXX=AIGC"), []byte("audio-audio-audio-audio-audio")...)

	items, upstreamID := parseUpstreamMedia(body)
	if upstreamID != "" {
		t.Fatalf("upstreamID = %q", upstreamID)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].MimeType != "audio/mpeg" {
		t.Fatalf("mime = %q", items[0].MimeType)
	}
	data, contentType, err := decodeEncodedMedia(items[0].B64JSON, items[0].MimeType, "audio")
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "audio/mpeg" {
		t.Fatalf("contentType = %q", contentType)
	}
	if string(data[:3]) != "ID3" {
		t.Fatalf("decoded head = %q", string(data[:3]))
	}
}

func TestJoinBaseEndpointNormalizesMissingSlash(t *testing.T) {
	got := joinBaseEndpoint("https://api.minimaxi.com/", "v1/music_generation")
	if got != "https://api.minimaxi.com/v1/music_generation" {
		t.Fatalf("url = %q", got)
	}
}

func TestUnwrapUpstreamBodySupportsNestedTask(t *testing.T) {
	got := unwrapUpstreamBody(map[string]interface{}{
		"request_id": "req-1",
		"task": map[string]interface{}{
			"id":     "task-1",
			"status": "succeeded",
			"content": map[string]interface{}{
				"url": "https://example.com/result.mp4",
			},
		},
	})
	if got["status"] != "succeeded" || got["id"] != "task-1" {
		t.Fatalf("nested task was not unwrapped: %#v", got)
	}
	items := extractMediaItems(got)
	if len(items) != 1 || items[0].URL != "https://example.com/result.mp4" {
		t.Fatalf("nested task media not extracted: %#v", items)
	}
}

func TestUnwrapUpstreamBodyPrefersDeepTaskStatus(t *testing.T) {
	got := unwrapUpstreamBody(map[string]interface{}{
		"status":   "processing",
		"progress": 28,
		"data": map[string]interface{}{
			"task": map[string]interface{}{
				"status":    "completed",
				"progress":  100,
				"video_url": "https://example.com/result.mp4",
			},
		},
	})
	if got["status"] != "completed" || got["progress"] != 100 {
		t.Fatalf("deep task status was not preferred: %#v", got)
	}
	if got["video_url"] != "https://example.com/result.mp4" {
		t.Fatalf("deep task media was not unwrapped: %#v", got)
	}
}

func TestUpstreamRequestTimeoutSupportsAudioAndOverride(t *testing.T) {
	if got := upstreamRequestTimeout(nil, true); got != 15*time.Minute {
		t.Fatalf("audio timeout = %s", got)
	}
	got := upstreamRequestTimeout(map[string]interface{}{
		"upstream": map[string]interface{}{"request_timeout_sec": float64(900)},
	}, true)
	if got != 15*time.Minute {
		t.Fatalf("override timeout = %s", got)
	}
}

func TestFirstSuccessMediaURLAcceptsSameOriginContentFromFailReason(t *testing.T) {
	raw := map[string]interface{}{
		"status":      "SUCCESS",
		"fail_reason": "https://otuapi.com/v1/videos/task_content/content",
	}
	conn := connectionConfig{BaseURL: "https://otuapi.com"}

	got := firstSuccessMediaURL(raw, "task_original", conn)
	if got != "https://otuapi.com/v1/videos/task_original/content" {
		t.Fatalf("media url = %q", got)
	}
}

func TestBuildMediaDownloadCandidatesPreferOriginalTaskID(t *testing.T) {
	conn := connectionConfig{BaseURL: "https://otuapi.com"}
	got := buildMediaDownloadCandidates(conn, "https://otuapi.com/v1/videos/task_wrong/content", "task_original")

	want := []string{
		"https://otuapi.com/v1/videos/task_original/content",
		"https://otuapi.com/v1/videos/task_wrong/content",
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestShouldNotMirrorPublicCDNMediaURL(t *testing.T) {
	conn := connectionConfig{BaseURL: "https://otuapi.com"}

	if shouldMirrorMediaURL("https://oss-us.file-download.life/2026/06/13/video.mp4", conn) {
		t.Fatal("public CDN mp4 should be used directly instead of mirrored through /content")
	}
	if !shouldMirrorMediaURL("https://otuapi.com/v1/videos/task_original/content", conn) {
		t.Fatal("same-origin /content url should be mirrored")
	}
}

func TestHardDownload404(t *testing.T) {
	body := []byte(`{"error":{"message":"Task not found","type":"invalid_request_error"}}`)
	if !isHardDownload404(404, body) {
		t.Fatal("Task not found 404 should switch candidates without retrying")
	}
	if isHardDownload404(404, []byte(`{"error":{"message":"not ready"}}`)) {
		t.Fatal("generic 404 can still be transient")
	}
}

func TestParsePollConfigCorrectsLegacySoraVideosPath(t *testing.T) {
	cfg := parsePollConfig(map[string]interface{}{
		"upstream": map[string]interface{}{
			"poll_path": "/v1/video/generations/{id}",
		},
	}, "/v1/videos")

	if cfg.Path != "/v1/videos/{id}" {
		t.Fatalf("poll path = %q", cfg.Path)
	}
}

func TestUpstreamErrorMessageHumanizesUnsafePrompt(t *testing.T) {
	body := []byte(`{"code":"upstream_error","message":"The provided prompt is considered unsafe and it cannot be used to generate content."}`)

	got := upstreamErrorMessage(body)
	if got != "生成内容未通过上游安全审核，请修改提示词或参考素材后重试（避免武器、暴力、敏感人物、侵权或受限内容）" {
		t.Fatalf("message = %q", got)
	}
}

func TestHumanizeUpstreamFailureHandlesAsyncModerationBlock(t *testing.T) {
	got := humanizeUpstreamFailure("content blocked by moderation")
	want := "生成内容未通过上游安全审核，请修改提示词或参考素材后重试（避免武器、暴力、敏感人物、侵权或受限内容）"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestUpstreamErrorMessageReadsBaseResp(t *testing.T) {
	body := []byte(`{"data":null,"trace_id":"x","base_resp":{"status_code":1008,"status_msg":"insufficient balance"}}`)

	got := upstreamErrorMessage(body)
	want := "上游模型账户余额不足，请检查或更换可用渠道"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
