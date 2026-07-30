package main

import (
	"context"
	"strings"
	"testing"
)

func TestComposeOutputDimensionsKeepUsesSource(t *testing.T) {
	width, height := composeOutputDimensions("keep")
	if width != 0 || height != 0 {
		t.Fatalf("keep dimensions = %dx%d, want source dimensions marker", width, height)
	}
	width, height = composeOutputDimensions("1080x1920")
	if width != 1080 || height != 1920 {
		t.Fatalf("explicit dimensions = %dx%d", width, height)
	}
}

func TestComposeCanvasMediaRejectsInvalidModeInputsBeforeFFmpeg(t *testing.T) {
	_, _, _, err := composeCanvasMedia(
		context.Background(),
		t.TempDir(),
		[]composeSource{
			{Kind: "video", Path: "video.mp4"},
			{Kind: "audio", Path: "audio.mp3"},
		},
		"concat",
		"keep",
	)
	if err == nil || !strings.Contains(err.Error(), "同类型") {
		t.Fatalf("expected mixed concat validation error, got %v", err)
	}

	_, _, _, err = composeCanvasMedia(
		context.Background(),
		t.TempDir(),
		[]composeSource{{Kind: "image", Path: "image.png"}, {Kind: "video", Path: "video.mp4"}},
		"auto",
		"keep",
	)
	if err == nil || !strings.Contains(err.Error(), "图片不能") {
		t.Fatalf("expected ignored image validation error, got %v", err)
	}
}
