import assert from "node:assert/strict";
import test from "node:test";
import { supportsVideoAnalysis } from "./canvasModelCapabilities.ts";

test("video analysis support uses explicit capabilities and known multimodal model names", () => {
  assert.equal(supportsVideoAnalysis({ code: "custom", runtime_rule: { capabilities: { video_analysis: true } } }), true);
  assert.equal(supportsVideoAnalysis({ code: "gemini-3-5-flash" }), true);
  assert.equal(supportsVideoAnalysis({ code: "text-only-chat" }), false);
});
