package handler

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/service"
)

type openAPIImageEditStore struct {
	objects []string
}

func (s *openAPIImageEditStore) Upload(_ context.Context, objectName, _ string, r io.Reader, _ int64) (string, error) {
	if _, err := io.ReadAll(r); err != nil {
		return "", err
	}
	s.objects = append(s.objects, objectName)
	return "https://cdn.example/" + objectName, nil
}
func (*openAPIImageEditStore) ReadAll(context.Context, string, int64) ([]byte, error) {
	return nil, nil
}
func (*openAPIImageEditStore) Delete(context.Context, string) error { return nil }
func (*openAPIImageEditStore) ObjectKeyFromURL(string) string       { return "" }
func (*openAPIImageEditStore) PublicURL(objectName string) string   { return objectName }

func openAPIMediaModel(profile string) *service.ModelFull {
	return &service.ModelFull{RuntimeRule: map[string]interface{}{
		"video": map[string]interface{}{"upload_profile": profile, "mode_param": "generation_mode"},
	}}
}

func TestNormalizeOpenAPIMediaParamsAcrossProviders(t *testing.T) {
	tests := []struct {
		name, profile string
		body          map[string]interface{}
		wantMode      string
		wantKey       string
	}{
		{"veo reference", "veo_reference", map[string]interface{}{"model": "veo", "prompt": "go", "image": "https://cdn/ref.png", "wait": true}, "reference", "reference_images"},
		{"veo frame pair", "veo_frame_pair", map[string]interface{}{"model": "veo-fl", "prompt": "go", "images": []interface{}{"https://cdn/first.png", "https://cdn/last.png"}}, "", "first_frame"},
		{"seedance mixed", "seedance_2", map[string]interface{}{"model": "seedance", "prompt": "go", "reference_image": "https://cdn/ref.png", "reference_audio": "https://cdn/ref.mp3", "aspect_ratio": "9:16", "audio": true}, "image_audio", "reference_images"},
		{"minimax first frame", "minimax_h3", map[string]interface{}{"model": "minimax", "prompt": "go", "first_frame_image": "https://cdn/first.png", "duration_seconds": 8}, "first_frame", "first_frame"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := normalizeOpenAPIMediaParams(test.body, openAPIMediaModel(test.profile), "video", "prompt")
			if test.wantMode != "" && params["generation_mode"] != test.wantMode {
				t.Fatalf("generation_mode = %#v, want %q", params["generation_mode"], test.wantMode)
			}
			if _, ok := params[test.wantKey]; !ok {
				t.Fatalf("missing canonical %s: %#v", test.wantKey, params)
			}
			if _, leaked := params["wait"]; leaked {
				t.Fatalf("platform wait leaked into task params: %#v", params)
			}
		})
	}
}

func TestNormalizeOpenAPIImageAliases(t *testing.T) {
	params := normalizeOpenAPIMediaParams(map[string]interface{}{
		"model": "image", "prompt": "cat", "n": 2, "image_url": "https://cdn/cat.png", "response_format": "sync",
	}, &service.ModelFull{}, "images", "prompt")
	if params["count"] != 2 || len(params["reference_images"].([]string)) != 1 {
		t.Fatalf("image aliases were not normalized: %#v", params)
	}
	if _, leaked := params["response_format"]; leaked {
		t.Fatalf("platform response_format leaked into task params: %#v", params)
	}
}

func TestOpenAPIImageEditBodyUploadsImageArrayAsTaskReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("prompt", "keep the subject")
	_ = writer.WriteField("quality", "high")
	_ = writer.WriteField("wait", "true")
	for _, name := range []string{"first.png", "second.png"} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="image[]"; filename="`+name+`"`)
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("png"))
	}
	_ = writer.Close()

	request := httptest.NewRequest("POST", "/v1/images/edits", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	c.Set("user_id", int64(42))
	store := &openAPIImageEditStore{}
	body, ok := (&Handler{storage: store}).openAPIImageEditBody(c)
	if !ok {
		t.Fatalf("multipart edit was rejected: %s", recorder.Body.String())
	}
	references, ok := body["reference_images"].([]string)
	if !ok || len(references) != 2 || len(store.objects) != 2 {
		t.Fatalf("reference images were not introduced into the task body: body=%#v uploads=%#v", body, store.objects)
	}
	if body["quality"] != "high" || body["wait"] != true || !strings.Contains(store.objects[0], "openapi/image-edits/42/") {
		t.Fatalf("multipart fields or upload ownership were lost: body=%#v uploads=%#v", body, store.objects)
	}
}

func TestNormalizeOpenAIAudioVoiceAlias(t *testing.T) {
	model := &service.ModelFull{ModelDTO: service.ModelDTO{InputSchema: map[string]interface{}{
		"properties": map[string]interface{}{"voice_id": map[string]interface{}{"type": "string"}},
	}}}
	params := normalizeOpenAPIMediaParams(map[string]interface{}{
		"model": "speech", "input": "hello", "voice": "narrator", "format": "mp3", "wait": true,
	}, model, "audio", "input")
	if params["voice_id"] != "narrator" {
		t.Fatalf("OpenAI voice alias was not mapped to voice_id: %#v", params)
	}
	if _, leaked := params["voice"]; leaked {
		t.Fatalf("voice alias leaked alongside voice_id: %#v", params)
	}
}

func TestOpenAPIMediaPromptRequiredUsesModelRuntime(t *testing.T) {
	optionalAudio := &service.ModelFull{RuntimeRule: map[string]interface{}{
		"audio": map[string]interface{}{"prompt_required": false},
	}}
	if openAPIMediaPromptRequired(optionalAudio, "audio") {
		t.Fatal("audio input should be optional when the selected model says so")
	}
	if !openAPIMediaPromptRequired(&service.ModelFull{}, "audio") {
		t.Fatal("audio input should remain required by default")
	}
}

func TestOpenAPIAudioPrimaryInputSupportsConfiguredLyricsAlias(t *testing.T) {
	model := &service.ModelFull{RuntimeRule: map[string]interface{}{
		"upstream": map[string]interface{}{"map": map[string]interface{}{"prompt": "lyrics"}},
	}}
	got := openAPIAudioPrimaryInput(map[string]interface{}{"lyrics": "[Verse] hello"}, model, "")
	if got != "[Verse] hello" {
		t.Fatalf("lyrics alias was not promoted to primary input: %q", got)
	}
	if got := openAPIAudioPrimaryInput(map[string]interface{}{"lyrics": "ignored"}, model, "explicit"); got != "explicit" {
		t.Fatalf("explicit input must win: %q", got)
	}
}
