package main

import (
	"strings"
	"testing"
)

func TestReferenceImageURLsPreservesAndDeduplicatesComicReferences(t *testing.T) {
	inputs := map[string]interface{}{
		"image_url":        "https://cdn.example/character.png",
		"reference_images": []interface{}{"https://cdn.example/character.png", "https://cdn.example/prop.png"},
		"comic_style":      map[string]interface{}{"cover_url": "https://cdn.example/style.png"},
	}
	got := referenceImageURLs(inputs)
	want := []string{"https://cdn.example/character.png", "https://cdn.example/style.png", "https://cdn.example/prop.png"}
	if len(got) != len(want) {
		t.Fatalf("reference count=%d, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("reference[%d]=%q, want %q", index, got[index], want[index])
		}
	}
}

func TestReferenceImageURLsDropsRelativeStyleCover(t *testing.T) {
	inputs := map[string]interface{}{
		"image_url":   "https://cdn.example/keyframe.png",
		"comic_style": map[string]interface{}{"cover_url": "/assets/comic-styles/cn-ancient.svg"},
	}
	got := referenceImageURLs(inputs)
	if len(got) != 1 || got[0] != "https://cdn.example/keyframe.png" {
		t.Fatalf("relative style cover leaked into upstream references: %#v", got)
	}
}

func TestComicStoryboardsInjectsLinkedAssetPrompts(t *testing.T) {
	plan := map[string]interface{}{
		"characters": []interface{}{map[string]interface{}{"code": "CHAR_01", "visual_prompt": "red-haired heroine"}},
		"props":      []interface{}{map[string]interface{}{"code": "PROP_01", "visual_prompt": "silver compass"}},
		"locations":  []interface{}{map[string]interface{}{"code": "LOC_01", "visual_prompt": "rainy old station"}},
		"storyboards": []interface{}{map[string]interface{}{
			"id": "S01", "scene": "departure", "character_codes": []interface{}{"CHAR_01"},
			"prop_codes": []interface{}{"PROP_01"}, "location_code": "LOC_01",
		}},
	}
	items := comicStoryboards(plan, map[string]interface{}{})
	if len(items) != 1 {
		t.Fatalf("storyboard count=%d", len(items))
	}
	prompt := stringAny(items[0]["keyframe_prompt"])
	for _, expected := range []string{"red-haired heroine", "silver compass", "rainy old station"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("keyframe prompt does not contain %q: %s", expected, prompt)
		}
	}
}

func TestComicStoryboardsDoesNotDuplicateAssetContext(t *testing.T) {
	plan := map[string]interface{}{
		"characters": []interface{}{map[string]interface{}{"code": "CHAR_01", "visual_prompt": "heroine"}},
		"storyboards": []interface{}{map[string]interface{}{
			"id": "S01", "character_codes": []interface{}{"CHAR_01"},
		}},
	}
	first := comicStoryboards(plan, map[string]interface{}{})
	plan["storyboards"] = []interface{}{first[0]}
	second := comicStoryboards(plan, map[string]interface{}{})
	if count := strings.Count(stringAny(second[0]["video_prompt"]), "CONSISTENCY ASSETS:"); count != 1 {
		t.Fatalf("asset context count=%d, want 1: %s", count, stringAny(second[0]["video_prompt"]))
	}
}

func TestComicMediaInvalidParameterIsNotRetried(t *testing.T) {
	if isRetryableComicMediaError("The parameter content[2].image_url specified in the request is not valid.") {
		t.Fatal("invalid upstream media parameter must fail fast")
	}
	if !isRetryableComicMediaError("生成超时，请重试") {
		t.Fatal("transient timeout should remain retryable")
	}
}

func TestComicScoresDoNotPretendQualityWasChecked(t *testing.T) {
	scores := comicPassScores(map[string]interface{}{"asset_consistency_score": 80, "logic_score": 50}, map[string]interface{}{})
	if checked, _ := scores["checked"].(bool); checked {
		t.Fatal("quality score must not be marked checked without a judge model")
	}
	if _, exists := scores["asset_consistency"]; exists {
		t.Fatal("synthetic asset consistency score must not be returned")
	}
}

func TestComicStageCompleteMatchesStoryboardIDs(t *testing.T) {
	storyboards := []map[string]interface{}{{"id": "S01"}, {"id": "S02"}}
	complete := []interface{}{
		map[string]interface{}{"id": "S01", "image_url": "one.jpg"},
		map[string]interface{}{"id": "S02", "image_url": "two.jpg"},
	}
	if !comicStageComplete(complete, storyboards, "image_url") {
		t.Fatal("stage with every storyboard output should be complete")
	}
	partial := []interface{}{
		map[string]interface{}{"id": "S01", "image_url": "one.jpg"},
		map[string]interface{}{"id": "S02", "status": "failed"},
	}
	if comicStageComplete(partial, storyboards, "image_url") {
		t.Fatal("failed or missing storyboard output must keep stage incomplete")
	}
}

func TestComicNarrationStageCompleteAllowsSilentShots(t *testing.T) {
	storyboards := []map[string]interface{}{
		{"id": "S01", "dialogue": "第一句对白", "duration_sec": 5},
		{"id": "S02", "dialogue": "", "duration_sec": 6},
	}
	items := []interface{}{
		map[string]interface{}{"id": "S01", "status": "succeeded", "audio_url": "one.mp3"},
		map[string]interface{}{"id": "S02", "status": "skipped", "audio_url": ""},
	}
	if !comicNarrationStageComplete(items, storyboards) {
		t.Fatal("silent storyboard marked skipped should complete the narration stage")
	}
	items[0] = map[string]interface{}{"id": "S01", "status": "succeeded", "audio_url": ""}
	if comicNarrationStageComplete(items, storyboards) {
		t.Fatal("storyboard with dialogue must have a generated audio URL")
	}
}

func TestComicAudioStrategyDefaultsAndOverrides(t *testing.T) {
	if got := comicAudioStrategy(map[string]interface{}{}, map[string]interface{}{}); got != "video_native" {
		t.Fatalf("strategy=%q, want video_native", got)
	}
	if got := comicAudioStrategy(map[string]interface{}{}, map[string]interface{}{"narration_model_code": "tts"}); got != "hybrid" {
		t.Fatalf("strategy=%q, want hybrid when TTS is configured", got)
	}
	if got := comicAudioStrategy(map[string]interface{}{"audio_strategy": "tts_only"}, map[string]interface{}{"narration_model_code": "tts"}); got != "tts_only" {
		t.Fatalf("strategy=%q, want explicit tts_only", got)
	}
}

func TestComicNarrationPerspectiveAndSpeechSelection(t *testing.T) {
	storyboard := map[string]interface{}{"dialogue": "角色对白", "narration": "画外旁白"}
	firstText, firstType := comicStoryboardSpeech(storyboard, map[string]interface{}{"narration_perspective": "first_person"}, map[string]interface{}{})
	if firstText != "画外旁白" || firstType != "narration" {
		t.Fatalf("first-person speech=%q/%q, want narration", firstText, firstType)
	}
	dialogueText, dialogueType := comicStoryboardSpeech(storyboard, map[string]interface{}{"narration_perspective": "character_dialogue"}, map[string]interface{}{})
	if dialogueText != "角色对白" || dialogueType != "dialogue" {
		t.Fatalf("character speech=%q/%q, want dialogue", dialogueText, dialogueType)
	}
	if got := comicNarrationPerspective(map[string]interface{}{"narration_perspective": "unknown"}, map[string]interface{}{}); got != "smart" {
		t.Fatalf("invalid narration perspective=%q, want smart", got)
	}
}

func TestComicIdentityPromptTreatsFirstReferenceAsIdentitySource(t *testing.T) {
	prompt := comicIdentityPrompt(map[string]interface{}{
		"reference_images": []interface{}{"https://cdn.example/hero.png"},
	}, "scene prompt")
	for _, expected := range []string{"immutable identity source", "facial geometry", "scene prompt"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("identity prompt does not contain %q: %s", expected, prompt)
		}
	}
}

func TestComicAssetReferenceURLs(t *testing.T) {
	inputs := map[string]interface{}{
		"comic_assets": []interface{}{
			map[string]interface{}{"metadata": map[string]interface{}{"reference_urls": []interface{}{"https://cdn.example/role-a.png", "https://cdn.example/role-b.png"}}},
			map[string]interface{}{"metadata": map[string]interface{}{"reference_urls": []string{"https://cdn.example/role-a.png", "https://cdn.example/prop.png"}}},
		},
	}
	got := comicAssetReferenceURLs(inputs)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique comic asset references, got %#v", got)
	}
	if got[0] != "https://cdn.example/role-a.png" || got[2] != "https://cdn.example/prop.png" {
		t.Fatalf("unexpected comic asset reference order: %#v", got)
	}
}

func TestComicDialogueModelCandidatesKeepsPrimaryAndFallbackOrder(t *testing.T) {
	inputs := map[string]interface{}{
		"dialogue_model_codes": []interface{}{"chat_primary", "chat_backup", "chat_primary"},
	}
	runtimeCfg := map[string]interface{}{
		"dialogue_model_codes": []string{"chat_backup", "chat_last"},
		"analysis_model_code":  "chat_analysis",
	}
	got := comicDialogueModelCandidates(inputs, runtimeCfg)
	want := []string{"chat_primary", "chat_backup", "chat_last", "chat_analysis"}
	if len(got) != len(want) {
		t.Fatalf("candidate count=%d, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("candidate[%d]=%q, want %q", index, got[index], want[index])
		}
	}
}

func TestSetLLMRequestContentSupportsResponsesAPI(t *testing.T) {
	body := map[string]interface{}{"messages": "stale"}
	setLLMRequestContent(body, "responses", "system", "user")
	if _, exists := body["messages"]; exists {
		t.Fatal("responses request must not retain messages")
	}
	input, ok := body["input"].([]map[string]string)
	if !ok || len(input) != 2 || input[0]["role"] != "system" || input[1]["content"] != "user" {
		t.Fatalf("unexpected responses input: %#v", body["input"])
	}
}
