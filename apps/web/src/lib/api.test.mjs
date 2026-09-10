import assert from "node:assert/strict";
import test from "node:test";

import { streamChatCompletion } from "./api.ts";

const originalFetch = globalThis.fetch;

function sseResponse(events) {
  const encoder = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      for (const event of events) controller.enqueue(encoder.encode(event));
      controller.close();
    },
  });
  return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } });
}

function restoreFetch() {
  globalThis.fetch = originalFetch;
}

test("accumulates streamed chat content, reasoning, and the settled cost", async () => {
  globalThis.fetch = async () =>
    sseResponse([
      'data: {"choices":[{"delta":{"role":"assistant"}}]}\n\n',
      'data: {"choices":[{"delta":{"reasoning_content":"想想"}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"你"}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"好"}}]}\n\n',
      'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"cost":0.25}\n\n',
      "data: [DONE]\n\n",
    ]);

  try {
    const seen = [];
    const result = await streamChatCompletion(
      "/api/chat/completions",
      { model_code: "chat_demo_v1" },
      { onContent: (_delta, accumulated) => seen.push(accumulated) }
    );
    assert.equal(result.content, "你好");
    assert.equal(result.reasoning, "想想");
    assert.equal(result.cost, 0.25);
    assert.deepEqual(seen, ["你", "你好"]);
  } finally {
    restoreFetch();
  }
});

test("surfaces stream error events instead of failing silently", async () => {
  globalThis.fetch = async () =>
    sseResponse(['data: {"error":{"type":"server_error","message":"模型服务异常"}}\n\n', "data: [DONE]\n\n"]);

  try {
    await assert.rejects(
      () => streamChatCompletion("/api/chat/completions", { model_code: "chat_demo_v1" }),
      /模型服务异常/
    );
  } finally {
    restoreFetch();
  }
});

test("replaces gateway HTML error pages with a readable retry hint", async () => {
  globalThis.fetch = async () =>
    new Response(
      "<html><head><title>geek-yo.com | 502: Bad gateway</title></head><body>Error code 502 Visit cloudflare.com for more information.</body></html>",
      { status: 502, headers: { "Content-Type": "text/html" } }
    );

  try {
    await assert.rejects(
      () => streamChatCompletion("/api/chat/completions", { model_code: "chat_demo_v1" }),
      (error) => {
        assert.ok(error instanceof Error);
        assert.match(error.message, /服务暂时不可用/);
        assert.doesNotMatch(error.message, /Bad gateway|cloudflare/i);
        return true;
      }
    );
  } finally {
    restoreFetch();
  }
});
