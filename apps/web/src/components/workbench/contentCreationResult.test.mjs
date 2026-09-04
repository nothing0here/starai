import assert from "node:assert/strict";
import test from "node:test";
import { socialPublishText } from "./contentCreationResult.ts";

test("keeps publishable copy and removes the image-generation plan", () => {
  assert.equal(
    socialPublishText("标题：秋日旅行\n\n正文：现在出发。\n\n标签：#旅行\n---配图规划---\n配图1：金色树林"),
    "标题：秋日旅行\n\n正文：现在出发。\n\n标签：#旅行",
  );
});

test("formats legacy structured results for direct social-editor paste", () => {
  assert.equal(
    socialPublishText('```json\n{"content_post":{"title":"新品发布","body":"今天正式上线。","cta":"欢迎体验","hashtags":["#新品","#科技"]},"images":[]}\n```'),
    "新品发布\n\n今天正式上线。\n\n欢迎体验\n\n#新品 #科技",
  );
});

test("keeps ordinary copy unchanged", () => {
  assert.equal(socialPublishText("一段可以直接发布的正文。"), "一段可以直接发布的正文。");
  assert.equal(socialPublishText("  "), "");
});
