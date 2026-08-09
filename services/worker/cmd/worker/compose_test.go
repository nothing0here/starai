package main

import (
	"context"
	"os"
	"path/filepath"
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

func TestComposeCanvasMediaAutoConcatsVideosAndMuxesAudio(t *testing.T) {
	if _, err := ffmpegBinaryPath(); err != nil {
		t.Skipf("ffmpeg unavailable: %v", err)
	}
	ctx := context.Background()
	tmpDir := t.TempDir()
	firstVideo := filepath.Join(tmpDir, "first.mp4")
	secondVideo := filepath.Join(tmpDir, "second.mp4")
	audioTrack := filepath.Join(tmpDir, "source.m4a")
	for _, item := range []struct {
		path  string
		color string
	}{{firstVideo, "red"}, {secondVideo, "blue"}} {
		if err := runFFmpeg(ctx, "-y", "-f", "lavfi", "-i", "color=c="+item.color+":s=160x90:d=0.4", "-c:v", "libx264", "-pix_fmt", "yuv420p", item.path); err != nil {
			t.Fatalf("create fixture video: %v", err)
		}
	}
	if err := runFFmpeg(ctx, "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "aac", audioTrack); err != nil {
		t.Fatalf("create fixture audio: %v", err)
	}

	outputPath, kind, contentType, err := composeCanvasMedia(ctx, tmpDir, []composeSource{
		{Kind: "video", Path: firstVideo},
		{Kind: "video", Path: secondVideo},
		{Kind: "audio", Path: audioTrack},
	}, "auto", "keep")
	if err != nil {
		t.Fatalf("compose video remake output: %v", err)
	}
	if kind != "video" || contentType != "video/mp4" {
		t.Fatalf("output = %s %s, want video video/mp4", kind, contentType)
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("missing composed output: %v", err)
	}
	if !mediaHasAudio(ctx, outputPath) {
		t.Fatal("composed video is missing the source audio track")
	}
}
