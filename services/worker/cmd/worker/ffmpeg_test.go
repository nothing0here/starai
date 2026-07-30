package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegBinaryPathUsesConfiguredFile(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, ffmpegExecutableName())
	if err := os.WriteFile(binary, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", binary)
	got, err := ffmpegBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("ffmpeg path=%q, want %q", got, binary)
	}
}

func TestFFmpegBinaryPathUsesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, ffmpegExecutableName())
	if err := os.WriteFile(binary, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", dir)
	got, err := ffmpegBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("ffmpeg path=%q, want %q", got, binary)
	}
}
