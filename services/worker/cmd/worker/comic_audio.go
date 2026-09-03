package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const maxComicNarrationTempo = 2.0

func probeComicAudioDuration(ctx context.Context, source string) (float64, error) {
	probe := "ffprobe"
	if ffmpeg, err := ffmpegBinaryPath(); err == nil {
		candidate := filepath.Join(filepath.Dir(ffmpeg), ffprobeExecutableName())
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			probe = candidate
		}
	}
	data, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", source).Output()
	if err != nil {
		return 0, fmt.Errorf("无法读取配音真实时长：%w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, fmt.Errorf("配音时长无效，不能直接裁剪")
	}
	return duration, nil
}

func comicNarrationTempo(actual, allotted float64) (float64, error) {
	if actual <= 0 || allotted <= 0 || math.IsNaN(actual) || math.IsInf(actual, 0) || math.IsNaN(allotted) || math.IsInf(allotted, 0) {
		return 0, fmt.Errorf("配音或分镜时长无效")
	}
	// Reserve an 80ms tail for resampling/codec boundaries; preserve every spoken
	// word without changing the user's requested film duration or voice pitch.
	tempo := math.Max(1, actual/math.Max(0.1, allotted-0.08))
	if tempo > maxComicNarrationTempo {
		return 0, fmt.Errorf("配音实际%.2f秒，分镜仅%.2f秒；自动加速到%.1f倍仍无法完整容纳，请精简该段台词后重试配音，或确认延长成片。原音频和视频素材已保留，未截断发布", actual, allotted, maxComicNarrationTempo)
	}
	return tempo, nil
}

func normalizeComicNarration(ctx context.Context, source, output string, allotted float64) error {
	actual, err := probeComicAudioDuration(ctx, source)
	if err != nil {
		return err
	}
	tempo, err := comicNarrationTempo(actual, allotted)
	if err != nil {
		return err
	}
	filter := fmt.Sprintf("aresample=44100,asetpts=PTS-STARTPTS,atempo=%.8f,apad", tempo)
	return runFFmpeg(ctx, "-y", "-i", source, "-vn", "-af", filter, "-t", strconv.FormatFloat(allotted, 'f', 6, 64), "-ar", "44100", "-ac", "2", "-c:a", "pcm_s16le", output)
}

// Download once and measure before deciding the video cuts. These are temporary
// composition copies; original audio/video checkpoints remain reusable.
func loadComicNarrationSources(ctx context.Context, dir string, narrations []interface{}) ([]interface{}, error) {
	result := make([]interface{}, 0, len(narrations))
	for i, raw := range narrations {
		item, _ := raw.(map[string]interface{})
		item = copyMap(item)
		if audioURL := stringAny(item["audio_url"]); audioURL != "" {
			data, _, err := downloadAuthenticatedMedia(ctx, connectionConfig{}, audioURL, 100<<20)
			if err != nil {
				return nil, fmt.Errorf("下载分镜%d配音失败：%w", i+1, err)
			}
			path := filepath.Join(dir, fmt.Sprintf("narration_source_%03d", i+1))
			if err := os.WriteFile(path, data, 0600); err != nil {
				return nil, err
			}
			actual, err := probeComicAudioDuration(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("分镜%d：%w", i+1, err)
			}
			item["_source_path"], item["actual_duration_sec"] = path, actual
		}
		result = append(result, item)
	}
	return result, nil
}

// Allocate a shared frame-accurate timeline, using spare time from short lines
// before speeding up a long one. Never exceed the available source-video budget
// or alter the requested film duration. No model calls are needed for this repair.
func alignComicNarrationTimeline(segments, narrations []interface{}) ([]interface{}, []interface{}, error) {
	if len(narrations) == 0 {
		return segments, narrations, nil
	}
	if len(segments) != len(narrations) {
		return nil, nil, fmt.Errorf("配音与分镜数量不匹配")
	}
	n := len(segments)
	base, caps, actual := make([]int, n), make([]int, n), make([]float64, n)
	total, needsAdjustment := 0, false
	for i := 0; i < n; i++ {
		s, _ := segments[i].(map[string]interface{})
		a, _ := narrations[i].(map[string]interface{})
		base[i] = int(math.Round(floatAny(s["duration_sec"]) * 30))
		caps[i] = int(math.Round(math.Max(floatAny(s["source_duration_sec"]), floatAny(s["duration_sec"])) * 30))
		actual[i] = floatAny(a["actual_duration_sec"])
		if base[i] <= 0 || actual[i] < 0 || math.IsNaN(actual[i]) || math.IsInf(actual[i], 0) {
			return nil, nil, fmt.Errorf("分镜时长无效")
		}
		total += base[i]
		if actual[i] > math.Max(0.1, float64(base[i])/30-0.08)*1.10 {
			needsAdjustment = true
		}
	}
	frames := append([]int(nil), base...)
	if needsAdjustment {
		minimum := func(tempo float64) ([]int, bool) {
			allocation, sum := make([]int, n), 0
			for i := 0; i < n; i++ {
				allocation[i] = min(base[i], 30)
				if actual[i] > 0 {
					allocation[i] = max(allocation[i], int(math.Ceil((actual[i]/tempo+0.08)*30)))
				}
				if allocation[i] > caps[i] {
					return nil, false
				}
				sum += allocation[i]
			}
			return allocation, sum <= total
		}
		if _, ok := minimum(maxComicNarrationTempo); !ok {
			return nil, nil, fmt.Errorf("配音无法在现有分镜与总时长内完整容纳（已尝试共享空余时间及最多%.1f倍自动语速）。请精简超长段台词后重试配音，或确认延长成片；原素材保留，未截断发布", maxComicNarrationTempo)
		}
		lo, hi := 1.0, maxComicNarrationTempo
		if allocation, ok := minimum(lo); ok {
			frames = allocation
		} else {
			for attempt := 0; attempt < 24; attempt++ {
				mid := (lo + hi) / 2
				if _, ok := minimum(mid); ok {
					hi = mid
				} else {
					lo = mid
				}
			}
			frames, _ = minimum(hi)
		}
		left := total
		for _, f := range frames {
			left -= f
		}
		for i := 0; i < n && left > 0; i++ {
			add := min(left, max(0, base[i]-frames[i]))
			frames[i] += add
			left -= add
		}
		for i := 0; i < n && left > 0; i++ {
			add := min(left, caps[i]-frames[i])
			frames[i] += add
			left -= add
		}
		if left != 0 {
			return nil, nil, fmt.Errorf("无法对齐音画时间线")
		}
	}
	video, audio := make([]interface{}, n), make([]interface{}, n)
	for i := 0; i < n; i++ {
		s, _ := segments[i].(map[string]interface{})
		a, _ := narrations[i].(map[string]interface{})
		s, a = copyMap(s), copyMap(a)
		duration := float64(frames[i]) / 30
		s["duration_sec"], a["duration_sec"] = duration, duration
		if actual[i] > 0 {
			tempo, err := comicNarrationTempo(actual[i], duration)
			if err != nil {
				return nil, nil, err
			}
			a["tempo"] = tempo
		}
		video[i], audio[i] = s, a
	}
	return video, audio, nil
}
