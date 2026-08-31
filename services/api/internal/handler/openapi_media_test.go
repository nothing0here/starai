package handler

import (
	"testing"
	"time"

	"github.com/starai/api/internal/service"
)

func TestOpenAPITaskToStandardFormatForMedia(t *testing.T) {
	tests := []struct {
		mode   string
		output map[string]interface{}
		want   []string
	}{
		{"images", map[string]interface{}{"images": []interface{}{map[string]interface{}{"url": "https://example.com/1.png"}, map[string]interface{}{"url": "https://example.com/2.png"}}}, []string{"https://example.com/1.png", "https://example.com/2.png"}},
		{"video", map[string]interface{}{"videos": []interface{}{map[string]interface{}{"url": "https://example.com/1.mp4"}}}, []string{"https://example.com/1.mp4"}},
		{"audio", map[string]interface{}{"audio_url": "https://example.com/1.mp3"}, []string{"https://example.com/1.mp3"}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := openAPITaskToStandardFormat(&service.TaskDTO{Output: tt.output}, tt.mode)
			data, ok := got["data"].([]map[string]interface{})
			if !ok || len(data) != len(tt.want) {
				t.Fatalf("data = %#v", got["data"])
			}
			for i, want := range tt.want {
				if data[i]["url"] != want {
					t.Fatalf("data[%d] = %#v, want %q", i, data[i], want)
				}
			}
		})
	}
}

func TestOpenAPISyncWaitBudgetUsesModelPollLimit(t *testing.T) {
	budget, allowed := openAPISyncWaitBudget(map[string]interface{}{
		"upstream": map[string]interface{}{"poll_timeout_sec": float64(120)},
	}, "video")
	if !allowed || budget != 2*time.Minute {
		t.Fatalf("budget=%s allowed=%v", budget, allowed)
	}
	budget, allowed = openAPISyncWaitBudget(map[string]interface{}{
		"upstream": map[string]interface{}{"poll_timeout_sec": float64(7200)},
	}, "video")
	if allowed || budget != 10*time.Minute {
		t.Fatalf("long budget=%s allowed=%v", budget, allowed)
	}
}

func TestOpenAPIAsyncTaskResponseKeepsGeneratedOutput(t *testing.T) {
	response := openAPITaskResponse(&service.TaskDTO{
		TaskNo: "task-1", Type: "image", Status: "succeeded",
		Output: map[string]interface{}{"image_url": "https://example.com/1.png"},
	})
	output, ok := response["output"].(map[string]interface{})
	if !ok || output["image_url"] != "https://example.com/1.png" {
		t.Fatalf("output = %#v", response["output"])
	}
	if response["poll_url"] != "/v1/tasks/task-1" {
		t.Fatalf("poll_url = %#v", response["poll_url"])
	}
}
