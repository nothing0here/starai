import assert from "node:assert/strict";
import test from "node:test";

import { canvasTemplateEnabled, changedStoryboardIndexes, muteVideoNativeAudio, storyStoryboardSegments } from "./videoCreationWorkflow.ts";

const shot = (index) => ({
  segment_index: index,
  scene: `scene ${index}`,
  camera: "medium shot",
  image_prompt: `image ${index}`,
  video_prompt: `video ${index}`,
});

test("accepts a fenced, continuously numbered storyboard", () => {
  const result = storyStoryboardSegments(`\`\`\`json\n${JSON.stringify([shot(1), shot(2)])}\n\`\`\``, 2);
  assert.equal(result.length, 2);
});

test("accepts a storyboard wrapped in a segments object", () => {
  assert.equal(storyStoryboardSegments(JSON.stringify({ segments: [shot(1)] }), 1).length, 1);
});

test("rejects wrong counts, numbering, and missing generation prompts", () => {
  assert.deepEqual(storyStoryboardSegments(JSON.stringify([shot(1)]), 2), []);
  assert.deepEqual(storyStoryboardSegments(JSON.stringify([shot(2)]), 1), []);
  assert.deepEqual(storyStoryboardSegments(JSON.stringify([{ ...shot(1), image_prompt: "" }]), 1), []);
});

test("separate voiceover disables only declared native-audio switches", () => {
  const original = { audio: true, generate_audio: true, resolution: "720p" };
  assert.deepEqual(muteVideoNativeAudio(original, true), { audio: false, generate_audio: false, resolution: "720p" });
  assert.equal(original.audio, true);
  assert.equal(muteVideoNativeAudio(original, false), original);
});

test("detects only storyboard segments whose structured content changed", () => {
  const before = JSON.stringify([shot(1), shot(2), shot(3)]);
  const after = JSON.stringify([shot(1), { ...shot(2), camera: "close-up" }, shot(3)]);
  assert.deepEqual(changedStoryboardIndexes(before, after, 3), [2]);
  assert.deepEqual(changedStoryboardIndexes("invalid", after, 3), [1, 2, 3]);
  assert.deepEqual(changedStoryboardIndexes(before, "invalid", 3), []);
});

test("hides managed canvas workflows that are disabled", () => {
  const enabled = new Set(["video_creation", "content_image_post"]);
  assert.equal(canvasTemplateEnabled("story-short-video", enabled), true);
  assert.equal(canvasTemplateEnabled("viral-remake", enabled), false);
  assert.equal(canvasTemplateEnabled("text-image", enabled), true);
  assert.equal(canvasTemplateEnabled("viral-remake", null), true);
});
