"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Check, ChevronRight, Copy, KeyRound, Plus, Search } from "lucide-react";
import { SiteBrand, useSiteBranding } from "@/components/SiteBrand";
import { ReferralShareButton } from "@/components/ReferralShareButton";
import { WorkbenchTopActions } from "@/components/WorkbenchTopActions";
import { useI18n } from "@/i18n/I18nProvider";
import { apiForLocale, API_URL } from "@/lib/api";

interface APIDoc {
  id: number;
  slug: string;
  title: string;
  summary: string;
  protocol: string;
  base_url: string;
  endpoint: string;
  auth_header: string;
  sdk: string;
  content: Record<string, any>;
  model_code: string;
  model_name: string;
  model_icon_url?: string;
  model_description: string;
  category: string;
  request_mode: string;
  new_api_model: string;
}

interface OperationVariant {
  id: string;
  title: string;
  description: string;
  method: "GET" | "POST";
  path: string;
  requestMode?: string;
  protocol?: "openai" | "anthropic" | "gemini";
  doc?: APIDoc;
  docs?: APIDoc[];
}

interface Operation extends OperationVariant {
  group: string;
  variants?: OperationVariant[];
}

const DEFAULT_BASE_URL = "https://api.your-starai-domain.com";

const GROUPS = [
  { id: "chat", label: "聊天与文本" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "audio", label: "音频" },
  { id: "platform", label: "平台" },
];

function pretty(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}

function localizeStandardExample(value: unknown, translate: (source: string) => string): unknown {
  if (typeof value === "string") return translate(value);
  if (Array.isArray(value)) return value.map((item) => localizeStandardExample(item, translate));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, localizeStandardExample(item, translate)]));
  }
  return value;
}

function CopyButton({ value, label, copiedLabel }: { value: string; label: string; copiedLabel: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      setCopied(false);
    }
  };

  return <button type="button" onClick={copy} className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2.5 py-1.5 text-xs text-gray-600 transition hover:border-gray-300 hover:text-gray-900 dark:border-white/10 dark:bg-white/5 dark:text-gray-300 dark:hover:border-white/20 dark:hover:text-white"><span>{copied ? <Check size={13} className="text-emerald-500" /> : <Copy size={13} />}</span>{copied ? copiedLabel : label}</button>;
}

function defaultRequest(operation: OperationVariant, doc: APIDoc, sample: string, translate?: (source: string) => string) {
  if (operation.protocol === "anthropic") {
    return { model: doc.model_code, max_tokens: 1024, messages: [{ role: "user", content: sample }] };
  }
  if (operation.protocol === "gemini") {
    return { contents: [{ role: "user", parts: [{ text: sample }] }] };
  }
  const fromContent = doc.content?.request_example;
  if (fromContent && typeof fromContent === "object") return translate ? localizeStandardExample(fromContent, translate) : fromContent;
  switch (doc.request_mode) {
    case "images":
      return { model: doc.model_code, prompt: sample, size: "1024x1024", n: 1 };
    case "video":
      return { model: doc.model_code, prompt: sample, size: "1280x720", duration: 5 };
    case "audio":
      return { model: doc.model_code, input: sample, voice: "alloy", format: "mp3" };
    default:
      return { model: doc.model_code, messages: [{ role: "user", content: sample }], stream: false };
  }
}

function curlExample(operation: OperationVariant, doc: APIDoc | undefined, sample: string, baseURL: string, translate?: (source: string) => string) {
  const authHeader = operation.protocol === "anthropic"
    ? "x-api-key: <API_KEY>"
    : operation.protocol === "gemini"
      ? "x-goog-api-key: <API_KEY>"
      : doc?.auth_header || "Authorization: Bearer <API_KEY>";
  const body = operation.method === "GET" || !doc
    ? ""
    : "\n  -d '" + JSON.stringify(defaultRequest(operation, doc, sample, translate)) + "'";
  return "curl " + baseURL + operation.path + " \\\n"
    + "  -H \"Content-Type: application/json\" \\\n"
    + "  -H \"" + authHeader + "\"" + body;
}

function responseExample(operation: OperationVariant, content: Record<string, any>) {
  if (operation.id === "listModels") {
    return {
      object: "list",
      data: [{ id: "model-code", object: "model", owned_by: "starai" }],
    };
  }
  if (operation.id === "getTask") {
    return {
      task_no: "task_xxx",
      status: "processing",
      progress: 42,
      result: null,
      error: null,
    };
  }
  if (operation.id === "listTaskEvents") {
    return {
      events: [
        { type: "task.queued", created_at: "2026-01-01T00:00:00Z" },
        { type: "task.progress", progress: 42, created_at: "2026-01-01T00:00:05Z" },
      ],
    };
  }
  if (operation.protocol === "anthropic") {
    return { id: "msg_xxx", type: "message", role: "assistant", content: [{ type: "text", text: "..." }], stop_reason: "end_turn" };
  }
  if (operation.protocol === "gemini") {
    return { candidates: [{ content: { role: "model", parts: [{ text: "..." }] }, finishReason: "STOP" }] };
  }
  return content.response_example || {
    id: "chatcmpl_xxx",
    object: "chat.completion",
    choices: [{ index: 0, message: { role: "assistant", content: "..." }, finish_reason: "stop" }],
  };
}

function protocolParameters(operation: OperationVariant, translate: (source: string) => string) {
  const tr = translate;
  if (operation.protocol === "anthropic") {
    return [
      { name: "model", type: "string", required: true, description: tr("模型名称或平台模型编码") },
      { name: "max_tokens", type: "integer", required: true, description: tr("本次请求允许生成的最大 Token 数") },
      { name: "messages", type: "array", required: true, description: tr("Anthropic Messages 内容数组") },
      { name: "system", type: "string|array", required: false, description: tr("系统提示词或系统内容块") },
      { name: "stream", type: "boolean", required: false, description: tr("是否通过 SSE 返回流式事件") },
      { name: "tools", type: "array", required: false, description: tr("工具定义，用于工具调用") },
    ];
  }
  if (operation.protocol === "gemini") {
    return [
      { name: "contents", type: "array", required: true, description: tr("Gemini contents 消息数组") },
      { name: "systemInstruction", type: "object", required: false, description: tr("系统指令内容") },
      { name: "generationConfig", type: "object", required: false, description: tr("温度、输出 Token 数等生成参数") },
      { name: "tools", type: "array", required: false, description: tr("函数声明和工具定义") },
    ];
  }
  return [
    { name: "model", type: "string", required: true, description: tr("平台模型编码或后台接入模型名") },
    { name: "messages", type: "array", required: true, description: tr("OpenAI Chat Completions 消息数组") },
    { name: "stream", type: "boolean", required: false, description: tr("是否通过 SSE 返回流式片段") },
    { name: "temperature", type: "number", required: false, description: tr("采样温度，以接入模型支持范围为准") },
    { name: "max_tokens", type: "integer", required: false, description: tr("最大输出 Token 数") },
    { name: "tools", type: "array", required: false, description: tr("工具定义，用于工具调用") },
  ];
}

function streamEventSummary(protocol: OperationVariant["protocol"], eventName: string, data: string, translate: (source: string) => string) {
  if (data === "[DONE]") return translate("流式响应结束");
  try {
    const parsed = JSON.parse(data) as any;
    if (protocol === "anthropic") {
      const delta = parsed?.delta || {};
      if (typeof delta.text === "string") return translate("文本片段：") + delta.text;
      if (typeof delta.partial_json === "string") return translate("工具参数片段：") + delta.partial_json;
      if (delta.stop_reason) return translate("结束原因：") + delta.stop_reason;
      if (parsed?.content_block?.name) return translate("工具调用：") + parsed.content_block.name;
    } else if (protocol === "gemini") {
      const part = parsed?.candidates?.[0]?.content?.parts?.[0];
      if (typeof part?.text === "string") return translate("文本片段：") + part.text;
      if (part?.functionCall?.name) return translate("函数调用：") + part.functionCall.name;
      if (parsed?.candidates?.[0]?.finishReason) return translate("结束原因：") + parsed.candidates[0].finishReason;
    } else {
      const choice = parsed?.choices?.[0] || {};
      const delta = choice.delta || {};
      if (typeof delta.content === "string") return translate("文本片段：") + delta.content;
      if (Array.isArray(delta.tool_calls) && delta.tool_calls.length > 0) return translate("工具调用片段");
      if (choice.finish_reason) return translate("结束原因：") + choice.finish_reason;
    }
  } catch {
    return "";
  }
  return eventName ? translate("协议事件：") + eventName : "";
}

function formatSSEBlock(protocol: OperationVariant["protocol"], block: string, translate: (source: string) => string) {
  const lines = block.split(/\r?\n/);
  const eventName = lines.find((line) => line.startsWith("event:"))?.slice(6).trim() || "";
  const data = lines.filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
  if (!data) return "";
  const summary = streamEventSummary(protocol, eventName, data, translate);
  return [eventName ? "event: " + eventName : "", "data: " + data, summary ? translate("解析：") + summary : ""].filter(Boolean).join("\n");
}

function formatSSEBuffer(protocol: OperationVariant["protocol"], buffer: string, translate: (source: string) => string, flush = false) {
  const blocks = buffer.split(/\r?\n\r?\n/);
  const remainder = flush ? "" : (blocks.pop() || "");
  return { remainder, output: blocks.map((block) => formatSSEBlock(protocol, block, translate)).filter(Boolean).join("\n\n") };
}

function operationCatalog(docs: APIDoc[], translate: (source: string) => string): Operation[] {
  const byMode = (mode: string) => docs.filter((item) => item.request_mode === mode);
  const tr = translate;
  const chatDocs = docs.filter((item) => item.request_mode === "chat_completions" || item.request_mode === "responses");
  const chatDoc = chatDocs[0];
  const imageDocs = byMode("images");
  const videoDocs = byMode("video");
  const audioDocs = byMode("audio");
  const chatVariants: OperationVariant[] = [
    {
      id: "openai-chat",
      title: tr("OpenAI Chat Completions"),
      description: tr("OpenAI 兼容的聊天补全格式，支持普通响应与 SSE 流式响应。"),
      method: "POST",
      path: "/v1/chat/completions",
      requestMode: "chat_completions",
      protocol: "openai",
      doc: chatDoc,
      docs: chatDocs,
    },
    {
      id: "anthropic-messages",
      title: tr("Anthropic Messages"),
      description: tr("Anthropic 原生 Messages 格式，支持内容块、工具调用与流式事件。"),
      method: "POST",
      path: "/v1/messages",
      requestMode: "chat_completions",
      protocol: "anthropic",
      doc: chatDoc,
      docs: chatDocs,
    },
    {
      id: "gemini-generate-content",
      title: tr("Gemini generateContent"),
      description: tr("Gemini 原生 contents/parts 格式，支持 candidates 响应与 SSE 流式输出。"),
      method: "POST",
      path: "/v1beta/models/{model}:generateContent",
      requestMode: "chat_completions",
      protocol: "gemini",
      doc: chatDoc,
      docs: chatDocs,
    },
  ];
  const platformVariants: OperationVariant[] = [
    {
      id: "listModels",
      title: tr("模型列表"),
      description: tr("列出当前 API Key 可用的已启用模型。"),
      method: "GET",
      path: "/v1/models",
    },
    {
      id: "getTask",
      title: tr("任务查询"),
      description: tr("查询图片、视频、音频等异步任务的状态和结果。"),
      method: "GET",
      path: "/v1/tasks/{task_no}",
    },
    {
      id: "listTaskEvents",
      title: tr("任务事件"),
      description: tr("读取异步任务的进度事件，适合构建实时进度展示。"),
      method: "GET",
      path: "/v1/tasks/{task_no}/events",
    },
  ];
  return [
    {
      id: "chat",
      group: "chat",
      title: tr("聊天与文本"),
      description: tr("一个页面覆盖 OpenAI 兼容、Anthropic 原生和 Gemini 原生三种聊天协议。"),
      method: chatVariants[0].method,
      path: chatVariants[0].path,
      requestMode: chatVariants[0].requestMode,
      protocol: chatVariants[0].protocol,
      doc: chatVariants[0].doc,
      variants: chatVariants,
    },
    {
      id: "createImageGeneration",
      group: "image",
      title: tr("生成图片"),
      description: tr("根据文本提示词创建一张或多张图片。"),
      method: "POST",
      path: "/v1/images/generations",
      requestMode: "images",
      protocol: "openai",
      doc: imageDocs[0],
      docs: imageDocs,
    },
    {
      id: "createVideo",
      group: "video",
      title: tr("创建视频"),
      description: tr("启动一个异步视频生成任务，并通过任务接口查询进度。"),
      method: "POST",
      path: "/v1/video/generations",
      requestMode: "video",
      protocol: "openai",
      doc: videoDocs[0],
      docs: videoDocs,
    },
    {
      id: "createSpeech",
      group: "audio",
      title: tr("生成音频"),
      description: tr("生成语音或音乐，并返回异步任务信息。"),
      method: "POST",
      path: "/v1/audio/speech",
      requestMode: "audio",
      protocol: "openai",
      doc: audioDocs[0],
      docs: audioDocs,
    },
    {
      id: "platform",
      group: "platform",
      title: tr("平台工具"),
      description: tr("一个页面统一查看模型列表、异步任务状态与任务进度事件。"),
      method: platformVariants[0].method,
      path: platformVariants[0].path,
      variants: platformVariants,
    },
  ];
}

function protocolLabel(operation: OperationVariant, translate: (source: string) => string) {
  if (operation.protocol === "anthropic") return translate("Anthropic 原生");
  if (operation.protocol === "gemini") return translate("Gemini 原生");
  if (operation.protocol === "openai") return translate("OpenAI 兼容");
  return translate("平台接口");
}

export default function ApiDocsPage() {
  const { t, ts, td, locale } = useI18n();
  const translateDocText = (value: unknown) => {
    const text = String(value ?? "");
    const modelPrefix = "平台模型编码或后台接入模型名，例如 ";
    if (text.startsWith(modelPrefix)) {
      return ts("平台模型编码或后台接入模型名") + text.slice(modelPrefix.length);
    }
    return ts(text);
  };
  const { site_api_tagline, site_base_url, api_docs_enabled, api_docs_operations } = useSiteBranding();
  const [docs, setDocs] = useState<APIDoc[]>([]);
  const [activeOperationId, setActiveOperationId] = useState("chat");
  const [activeVariantId, setActiveVariantId] = useState("openai-chat");
  const [activeDocSlug, setActiveDocSlug] = useState("");
  const [search, setSearch] = useState("");
  const [debugApiKey, setDebugApiKey] = useState("");
  const [debugEndpoint, setDebugEndpoint] = useState("");
  const [debugBody, setDebugBody] = useState("{}");
  const [debugStream, setDebugStream] = useState(false);
  const [debugResult, setDebugResult] = useState("");
  const [debugLoading, setDebugLoading] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    if (api_docs_enabled === false) return () => controller.abort();
    apiForLocale<{ items: APIDoc[] }>("/api/api-docs", locale, { signal: controller.signal })
      .then((result) => setDocs(result.items || []))
      .catch((error) => { if (error?.name !== "AbortError") setDocs([]); });
    return () => controller.abort();
  }, [locale, api_docs_enabled]);

  const operations = useMemo(() => operationCatalog(docs, ts).filter((item) => api_docs_operations?.[item.group] !== false), [docs, ts, api_docs_operations]);
  const visibleOperations = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return operations;
    return operations.filter((item) => {
      const variants = item.variants || [];
      const text = [item.title, item.path, item.description]
        .concat(variants.flatMap((variant) => [variant.title, variant.path, variant.description]))
        .join(" ")
        .toLowerCase();
      return text.includes(query);
    });
  }, [operations, search]);
  const active = operations.find((item) => item.id === activeOperationId) || visibleOperations[0] || operations[0];
  const activeVariant = active?.variants?.find((variant) => variant.id === activeVariantId) || active?.variants?.[0] || active;
  const availableDocs = activeVariant?.docs?.length ? activeVariant.docs : activeVariant?.doc ? [activeVariant.doc] : [];
  const activeDoc = availableDocs.find((doc) => doc.slug === activeDocSlug) || availableDocs[0];
  const content = activeDoc?.content || {};
  const sample = active?.group === "image"
    ? ts("一只赛博朋克风格的猫")
    : active?.group === "audio"
      ? ts("你好，欢迎使用")
      : active?.group === "video"
        ? ts("一段城市夜景视频")
        : ts("你好，请介绍你的能力");
  const parameters = Array.isArray(content.parameters) && content.parameters.length > 0
    ? content.parameters
    : active?.group === "chat" && activeVariant
      ? protocolParameters(activeVariant, ts)
      : [];
  const responseDefinitions = content.responses && typeof content.responses === "object" && !Array.isArray(content.responses) ? content.responses as Record<string, any> : {};
  const errors = Array.isArray(content.errors) && content.errors.length > 0
    ? content.errors
    : Object.entries(responseDefinitions)
      .filter(([status]) => status !== "200" && !status.startsWith("2"))
      .map(([status, value]) => ({ status, code: value?.body?.code || status, description: value?.description || "" }));
  const capabilities = Array.isArray(content.capabilities || content.features) ? (content.capabilities || content.features) : [];
  const responseMode = String(content.response_mode || (activeVariant?.method === "GET" ? "sync" : active?.group === "chat" ? "sync_or_stream" : "async_task"));
  const rateLimit = content.rate_limit && typeof content.rate_limit === "object" ? content.rate_limit as Record<string, any> : {};
  const idempotency = content.idempotency && typeof content.idempotency === "object" ? content.idempotency as Record<string, any> : {};
  const deprecated = content.deprecated === true;
  const docSummary = activeDoc
    ? td(`model.${activeDoc.model_code}.description`, activeDoc.summary || activeDoc.model_description || "")
    : "";
  const docVersion = String(content.version || "v1");
  const docsSubtitle = locale === "zh-CN" ? "API 文档中心" : (site_api_tagline || ts("API 文档中心"));
  const baseURL = site_base_url?.trim().replace(/\/+$/, "") || activeDoc?.base_url || DEFAULT_BASE_URL;
  const endpointExample = baseURL + activeVariant.path;
  const authExample = activeVariant.protocol === "anthropic"
    ? "x-api-key: <API_KEY>"
    : activeVariant.protocol === "gemini"
      ? "x-goog-api-key: <API_KEY>"
      : activeDoc?.auth_header || "Authorization: Bearer <API_KEY>";
  const requestExampleText = activeVariant.method === "GET" ? "" : curlExample(activeVariant, activeDoc, sample, baseURL, ts);
  const requestBodyText = activeDoc ? pretty(defaultRequest(activeVariant, activeDoc, sample, ts)) : "{}";
  const responseBodyText = pretty(responseExample(activeVariant, content));
  const streamExampleText = activeVariant.protocol === "anthropic"
    ? `event: content_block_delta\ndata: {\n  "type": "content_block_delta",\n  "delta": { "type": "text_delta", "text": ${JSON.stringify(sample)} }\n}`
    : activeVariant.protocol === "gemini"
      ? `data: {\n  "candidates": [{\n    "content": { "role": "model", "parts": [{ "text": ${JSON.stringify(sample)} }] }\n  }]\n}`
      : `data: {\n  "id": "chatcmpl_xxx",\n  "object": "chat.completion.chunk",\n  "choices": [{ "index": 0, "delta": { "content": ${JSON.stringify(sample)} }, "finish_reason": null }]\n}\n\ndata: [DONE]`;
  const requestHeaders = activeVariant.protocol === "anthropic"
    ? [["x-api-key", "API Key"], ["anthropic-version", "2023-06-01"], ["content-type", "application/json"]]
    : activeVariant.protocol === "gemini"
      ? [["x-goog-api-key", "API Key"], ["content-type", "application/json"]]
      : [["Authorization", "Bearer <API_KEY>"], ["Content-Type", "application/json"]];

  useEffect(() => {
    const debugPath = activeVariant.path.replace(
      "{model}",
      encodeURIComponent(activeDoc?.new_api_model || activeDoc?.model_code || "")
    );
    setDebugEndpoint(API_URL.replace(/\/+$/, "") + debugPath);
    setDebugBody(requestBodyText);
    setDebugStream(false);
    setDebugResult("");
  }, [activeVariant.path, activeDoc?.new_api_model, activeDoc?.model_code, requestBodyText]);

  useEffect(() => {
    setActiveDocSlug("");
  }, [activeOperationId, activeVariantId]);

  const selectOperation = (item: Operation) => {
    setActiveOperationId(item.id);
    setActiveVariantId(item.variants?.[0]?.id || "");
  };

  const runDebugRequest = async () => {
    if (!debugApiKey.trim()) {
      setDebugResult(ts("请先填写 API Key。"));
      return;
    }
    if (!debugEndpoint.trim()) {
      setDebugResult(ts("请填写接口地址。"));
      return;
    }
    setDebugLoading(true);
    setDebugResult("");
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (activeVariant.protocol === "anthropic") {
        headers["x-api-key"] = debugApiKey.trim();
        headers["anthropic-version"] = "2023-06-01";
      } else if (activeVariant.protocol === "gemini") {
        headers["x-goog-api-key"] = debugApiKey.trim();
      } else {
        headers.Authorization = "Bearer " + debugApiKey.trim();
      }
      let payload: any = undefined;
      if (activeVariant.method !== "GET") {
        try {
          payload = JSON.parse(debugBody || "{}");
        } catch {
          setDebugResult(ts("请求体不是有效 JSON。"));
          return;
        }
        if (debugStream) payload.stream = true;
      }
      let requestURL = debugEndpoint.trim();
      if (debugStream && activeVariant.protocol === "gemini" && requestURL.includes(":generateContent")) {
        requestURL = requestURL.replace(":generateContent", ":streamGenerateContent");
        requestURL += requestURL.includes("?") ? "&alt=sse" : "?alt=sse";
      }
      const response = await fetch(requestURL, {
        method: activeVariant.method,
        headers,
        body: activeVariant.method === "GET" ? undefined : JSON.stringify(payload),
      });
      const responseType = response.headers.get("content-type") || "";
      if (debugStream && response.body && responseType.includes("text/event-stream")) {
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let streamOutput = "HTTP " + response.status + "\n\n";
        setDebugResult(streamOutput);
        while (true) {
          const chunk = await reader.read();
          if (chunk.done) break;
          buffer += decoder.decode(chunk.value, { stream: true });
          const parsed = formatSSEBuffer(activeVariant.protocol, buffer, ts);
          buffer = parsed.remainder;
          if (parsed.output) {
            streamOutput += parsed.output + "\n\n";
            setDebugResult(streamOutput);
          }
        }
        buffer += decoder.decode();
        const final = formatSSEBuffer(activeVariant.protocol, buffer, ts, true);
        if (final.output) streamOutput += final.output + "\n\n";
        setDebugResult(streamOutput.trimEnd());
      } else {
        const raw = await response.text();
        if (/<html[\s>]/i.test(raw)) {
          setDebugResult(
            "HTTP " + response.status + "\n\n" +
            ts("当前调试地址返回的是前端 HTML，不是 API 响应。请使用 API 服务地址（本地通常为 http://localhost:8080），不要使用前台页面地址。")
          );
          return;
        }
        let output: unknown = raw;
        try {
          output = JSON.parse(raw);
        } catch {
          output = raw;
        }
        setDebugResult("HTTP " + response.status + "\n\n" + (typeof output === "string" ? output : pretty(output)));
      }
    } catch (error) {
      setDebugResult(error instanceof Error ? error.message : ts("请求失败，请检查接口地址和跨域配置。"));
    } finally {
      setDebugLoading(false);
    }
  };

  const operationButton = (item: Operation) => (
    <button
      key={item.id}
      onClick={() => selectOperation(item)}
      className={"group relative w-full rounded-xl px-3 py-2.5 text-left transition " + (
        active?.id === item.id
          ? "bg-gray-900 text-white shadow-sm dark:bg-white/10"
          : "text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/5"
      )}
    >
      <div className="flex items-center gap-2">
        <span className={"rounded px-1.5 py-0.5 font-mono text-[9px] font-bold " + (
          active?.id === item.id
            ? "bg-white/15 text-primary"
            : item.method === "GET"
              ? "bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300"
              : "bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300"
        )}>{item.method}</span>
        <span className="truncate text-[13px] font-semibold">{item.title}</span>
      </div>
      <div className={"mt-1.5 truncate pl-[42px] text-[10px] " + (active?.id === item.id ? "text-white/60" : "text-gray-400")}>
        {item.variants ? ts(item.id === "chat" ? "3 种协议" : "3 个平台接口") : item.path}
      </div>
    </button>
  );

  if (api_docs_enabled === false) {
    return <div className="flex h-full items-center justify-center bg-[#F5F7FB] px-4 dark:bg-[#070B14]"><div className="rounded-2xl border border-dashed bg-white px-8 py-12 text-center text-sm text-gray-400 dark:border-white/10 dark:bg-[#101827]">{ts("API 文档暂未开放")}</div></div>;
  }

  if (!active || !activeVariant) {
    return <div className="flex h-full items-center justify-center bg-[#F5F7FB] px-4 dark:bg-[#070B14]"><div className="text-sm text-gray-400">{t("apiDocs.noDocs")}</div></div>;
  }

  return (
    <div className="flex h-full min-h-0 overflow-hidden bg-[#F5F7FB] dark:bg-[#070B14]">
      <aside className="hidden h-full w-[280px] shrink-0 flex-col border-r border-gray-200/80 bg-[#FAFBFD] dark:border-white/10 dark:bg-[#0D1422] lg:flex">
        <div className="border-b border-gray-200/80 px-4 pb-4 pt-5 dark:border-white/10">
          <SiteBrand href="/app" subtitle={docsSubtitle} nameClassName="font-bold text-gray-900 dark:text-gray-100" subtitleClassName="text-[11px] text-gray-500 dark:text-gray-400" />
          <div className="mt-5 flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2.5 shadow-sm dark:border-white/10 dark:bg-white/5">
            <Search size={14} className="text-gray-400" />
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("apiDocs.search")} className="min-w-0 flex-1 bg-transparent text-xs text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100" />
          </div>
        </div>
        <div className="flex-1 overflow-y-auto px-3 py-4">
          {GROUPS.map((group) => {
            const items = visibleOperations.filter((item) => item.group === group.id);
            if (!items.length) return null;
            return <div key={group.id} className="mb-6 last:mb-0"><div className="mb-2 flex items-center gap-2 px-2 text-[10px] font-bold uppercase tracking-[0.16em] text-gray-400"><span className="h-1.5 w-1.5 rounded-full bg-gray-300 dark:bg-gray-600" />{ts(group.label)}<span className="h-px flex-1 bg-gray-200 dark:bg-white/10" /></div><div className="space-y-1">{items.map(operationButton)}</div></div>;
          })}
        </div>
        <div className="border-t border-gray-200/80 px-4 py-3 text-[11px] leading-5 text-gray-400 dark:border-white/10">
          {ts("接口路径、请求字段和响应字段保持标准协议命名。")}
        </div>
      </aside>

      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <div className="sticky top-0 z-20 hidden items-center justify-between border-b border-gray-200/80 bg-white/90 px-5 py-3 backdrop-blur dark:border-white/10 dark:bg-[#070B14]/90 lg:flex">
          <Link href="/app" className="flex items-center gap-1.5 rounded-xl bg-primary px-3 py-1.5 text-[13px] font-semibold text-dark"><Plus size={15} />{t("apiDocs.backWorkspace")}</Link>
          <WorkbenchTopActions />
        </div>
        <div className="mx-auto max-w-5xl px-4 py-6 sm:px-8 sm:py-10">
          <ReferralShareButton variant="card" className="mb-5" />
          <div className="mb-6 lg:hidden">
            <div className="mb-3 flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2.5 dark:border-white/10 dark:bg-[#101827]">
              <Search size={14} className="text-gray-400" />
              <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("apiDocs.search")} className="min-w-0 flex-1 bg-transparent text-xs text-gray-800 outline-none dark:text-gray-100" />
            </div>
            <select value={active.id} onChange={(event) => { const item = operations.find((operation) => operation.id === event.target.value); if (item) selectOperation(item); }} className="w-full rounded-xl border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-800 dark:border-white/10 dark:bg-[#101827] dark:text-gray-100">
              {visibleOperations.map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}
            </select>
          </div>

          <div className="mb-4 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400"><span>{ts("API 文档参考")}</span><ChevronRight size={13} /><span>{ts(GROUPS.find((group) => group.id === active.group)?.label || "平台")}</span></div>
          <div className="mb-7">
            <div className="mb-3 flex flex-wrap items-center gap-3">
              <h1 className="text-3xl font-bold tracking-tight text-gray-900 dark:text-gray-100">{active.title}</h1>
              <span className="rounded-md bg-indigo-50 px-2 py-1 text-xs font-semibold text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300">{protocolLabel(activeVariant, ts)}</span>
              <span className="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-white/10 dark:text-gray-300">{docVersion}</span>
            </div>
            <p className="max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300">{ts(activeVariant.description)}</p>
            {docSummary && <p className="mt-2 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">{ts(docSummary)}</p>}
          </div>

          {active.variants && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-2 shadow-sm dark:border-white/10 dark:bg-[#101827]">
            <div className="grid gap-2 sm:grid-cols-3">
              {active.variants.map((variant) => <button key={variant.id} onClick={() => setActiveVariantId(variant.id)} className={"rounded-xl px-4 py-3 text-left transition " + (activeVariant.id === variant.id ? "bg-gray-900 text-white dark:bg-white/10" : "text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-white/5")}>
                <div className="flex items-center gap-2 text-sm font-semibold">{activeVariant.id === variant.id && <Check size={15} className="text-primary" />}{variant.title}</div>
                <code className={"mt-1.5 block truncate text-[10px] " + (activeVariant.id === variant.id ? "text-white/60" : "text-gray-400")}>{variant.path}</code>
              </button>)}
            </div>
          </section>}

          <section className="mb-5 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-white/10 dark:bg-[#101827]">
            <div className="flex flex-wrap items-center gap-3 border-b border-gray-200 bg-gray-50 px-4 py-3 dark:border-white/10 dark:bg-white/5">
              <span className={"rounded-md px-2 py-1 text-xs font-bold " + (activeVariant.method === "GET" ? "bg-emerald-600 text-white" : "bg-blue-600 text-white")}>{activeVariant.method}</span>
              <code className="min-w-0 flex-1 break-all text-sm text-gray-800 dark:text-gray-200">{endpointExample}</code>
              <CopyButton value={endpointExample} label={ts("复制地址")} copiedLabel={ts("已复制")} />
            </div>
            <div className="grid gap-4 p-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("请求格式")}</div><code className="text-gray-800 dark:text-gray-200">{String(content.request_content_type || "application/json")}</code></div>
              <div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("响应模式")}</div><code className="text-gray-800 dark:text-gray-200">{responseMode}</code></div>
              <div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("鉴权方式")}</div><code className="text-gray-800 dark:text-gray-200">{activeVariant.protocol === "anthropic" ? "x-api-key" : activeVariant.protocol === "gemini" ? "x-goog-api-key" : "Bearer"}</code></div>
              <div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("接口协议")}</div><span className="text-gray-800 dark:text-gray-200">{protocolLabel(activeVariant, ts)}</span></div>
            </div>
          </section>

          <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]"><div className="mb-3 flex flex-wrap items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("调用约定")}</h2>{deprecated && <span className="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-semibold text-amber-700 dark:bg-amber-500/15 dark:text-amber-200">{ts("已弃用")}</span>}</div><div className="grid gap-4 text-sm sm:grid-cols-3"><div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("文档版本")}</div><code className="text-gray-800 dark:text-gray-200">{docVersion}</code></div><div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("限流")}</div><span className="text-gray-800 dark:text-gray-200">{rateLimit.requests_per_minute ? String(rateLimit.requests_per_minute) + " RPM" : ts("以平台配置为准")}</span></div><div><div className="mb-1 text-xs text-gray-500 dark:text-gray-400">{ts("幂等键")}</div><span className="text-gray-800 dark:text-gray-200">{idempotency.supported ? String(idempotency.header || "Idempotency-Key") : ts("暂未启用")}</span></div></div>{idempotency.note && <p className="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">{ts(String(idempotency.note))}</p>}</section>

          {active.group === "platform" ? <section className="mb-5 rounded-2xl border border-blue-200 bg-blue-50/80 p-5 dark:border-blue-400/20 dark:bg-blue-500/10">
            <h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("平台能力")}</h2>
            <div className="grid gap-3 sm:grid-cols-3">
              {[["模型列表", "查看当前 API Key 可用模型"], ["任务查询", "查询异步生成任务状态"], ["任务事件", "读取实时进度事件"]].map(([title, description]) => <div key={title} className="rounded-xl border border-blue-100 bg-white/80 p-3 dark:border-white/10 dark:bg-white/5"><div className="mb-1 text-sm font-semibold text-gray-900 dark:text-gray-100">{ts(title)}</div><div className="text-xs leading-5 text-gray-600 dark:text-gray-300">{ts(description)}</div></div>)}
            </div>
          </section> : <section className="mb-5 rounded-2xl border border-emerald-200 bg-emerald-50/80 p-5 dark:border-emerald-400/20 dark:bg-emerald-500/10">
            <h2 className="mb-2 font-semibold text-gray-900 dark:text-gray-100">{ts("支持的模型")}</h2>
            {availableDocs.length > 0 ? <div className="flex flex-wrap items-center gap-2">{availableDocs.map((doc) => <button type="button" key={doc.slug} onClick={() => setActiveDocSlug(doc.slug)} className={"rounded-lg px-3 py-1.5 text-left font-mono text-xs transition " + (activeDoc?.slug === doc.slug ? "bg-emerald-700 text-white dark:bg-emerald-500" : "bg-white text-gray-800 hover:bg-emerald-100 dark:bg-white/10 dark:text-gray-100 dark:hover:bg-white/20")}>{doc.new_api_model || doc.model_code}</button>)}</div> : <p className="text-sm text-gray-600 dark:text-gray-300">{ts("当前没有已发布的模型文档。")}</p>}
            {capabilities.length > 0 && <div className="mt-3 flex flex-wrap gap-2">{capabilities.map((item: unknown, index: number) => <span key={String(item) + index} className="rounded-full bg-white px-2.5 py-1 text-xs text-emerald-800 dark:bg-white/10 dark:text-emerald-200">{ts(String(item))}</span>)}</div>}
          </section>}

          <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("鉴权")}</h2><div className="flex items-center gap-3"><CopyButton value={authExample} label={ts("复制鉴权")} copiedLabel={ts("已复制")} /><Link href="/app/settings" className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"><KeyRound size={13} />{ts("管理 API Key")}</Link></div></div>
            <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{authExample}</pre>
          </section>

          <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]">
            <h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("请求头")}</h2>
            <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/10"><table className="w-full text-left text-sm"><thead className="bg-gray-50 text-xs text-gray-600 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-4 py-3">{ts("名称")}</th><th className="px-4 py-3">{ts("值")}</th><th className="px-4 py-3">{ts("说明")}</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-white/10">{requestHeaders.map(([name, value]) => <tr key={name}><td className="px-4 py-3 font-mono text-xs text-gray-900 dark:text-gray-100">{name}</td><td className="px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300">{value}</td><td className="px-4 py-3 text-xs text-gray-700 dark:text-gray-300">{name.toLowerCase().includes("key") || name.toLowerCase() === "authorization" ? ts("用于 API 鉴权") : name.toLowerCase().includes("version") ? ts("指定原生协议版本") : ts("请求体使用 JSON 格式")}</td></tr>)}</tbody></table></div>
          </section>

          {activeVariant.method !== "GET" && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]">
            <div className="mb-3 flex items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("请求示例")}</h2><CopyButton value={requestExampleText} label={ts("复制示例")} copiedLabel={ts("已复制")} /></div>
            <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100">{requestExampleText}</pre>
          </section>}

          <section className="mb-5 rounded-2xl border border-violet-200 bg-violet-50/70 p-5 dark:border-violet-400/20 dark:bg-violet-500/10">
            <div className="mb-1 flex flex-wrap items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("在线调试")}</h2><span className="rounded-full bg-white px-2.5 py-1 text-[11px] text-violet-700 dark:bg-white/10 dark:text-violet-200">{ts("仅当前页面使用，不保存 API Key")}</span></div>
            <p className="mb-4 text-xs leading-5 text-gray-600 dark:text-gray-300">{ts("填写 API Key 后发送真实请求。接口地址可以按部署环境修改；任务查询接口需要先替换路径中的 task_no。")}</p>
            <div className="grid gap-3 lg:grid-cols-[1fr_240px_auto]">
              <input value={debugEndpoint} onChange={(event) => setDebugEndpoint(event.target.value)} className="min-w-0 rounded-xl border border-violet-200 bg-white px-3 py-2.5 text-xs text-gray-800 outline-none focus:border-violet-400 dark:border-white/10 dark:bg-[#101827] dark:text-gray-100" aria-label={ts("接口地址")} />
              <input value={debugApiKey} onChange={(event) => setDebugApiKey(event.target.value)} type="password" autoComplete="off" placeholder={ts("填写 API Key")} className="min-w-0 rounded-xl border border-violet-200 bg-white px-3 py-2.5 text-xs text-gray-800 outline-none focus:border-violet-400 dark:border-white/10 dark:bg-[#101827] dark:text-gray-100" aria-label={ts("API Key")} />
              <button type="button" onClick={runDebugRequest} disabled={debugLoading} className="rounded-xl bg-violet-600 px-4 py-2.5 text-xs font-semibold text-white transition hover:bg-violet-700 disabled:cursor-not-allowed disabled:opacity-50">{debugLoading ? ts("请求中...") : ts("发送请求")}</button>
            </div>
            {active.group === "chat" && activeVariant.method === "POST" && <label className="mt-3 flex items-center gap-2 text-xs text-gray-700 dark:text-gray-300"><input type="checkbox" checked={debugStream} onChange={(event) => setDebugStream(event.target.checked)} className="h-4 w-4 rounded border-gray-300 text-violet-600 focus:ring-violet-500" />{ts("启用实时流式响应")}</label>}
            {activeVariant.method !== "GET" && <textarea value={debugBody} onChange={(event) => setDebugBody(event.target.value)} className="mt-3 min-h-32 w-full rounded-xl border border-violet-200 bg-white p-3 font-mono text-xs leading-5 text-gray-800 outline-none focus:border-violet-400 dark:border-white/10 dark:bg-[#101827] dark:text-gray-100" aria-label={ts("请求体 JSON")} />}
            {debugResult && <pre className="mt-3 max-h-96 overflow-auto rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100">{debugResult}</pre>}
          </section>

          {active.group === "chat" && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]">
            <h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("流式事件")}</h2>
            <div className="mb-3 flex items-center justify-between gap-2"><p className="text-xs leading-5 text-gray-600 dark:text-gray-300">{activeVariant.protocol === "anthropic" ? ts("设置 stream=true 后接收 Anthropic Messages 事件。") : activeVariant.protocol === "gemini" ? ts("使用 streamGenerateContent 操作通过 SSE 接收 Gemini candidates。") : ts("设置 stream=true 后接收 OpenAI 兼容的 SSE 事件。")}</p><CopyButton value={streamExampleText} label={ts("复制事件")} copiedLabel={ts("已复制")} /></div>
            <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100">{streamExampleText}</pre>
          </section>}

          {parameters.length > 0 && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]">
            <h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("请求参数")}</h2>
            <div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/10"><table className="w-full text-left text-sm"><thead className="bg-gray-50 text-xs text-gray-600 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-4 py-3">{ts("名称")}</th><th className="px-4 py-3">{ts("类型")}</th><th className="px-4 py-3">{ts("必填")}</th><th className="px-4 py-3">{ts("说明")}</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-white/10">{parameters.map((item: any, index: number) => <tr key={String(item.name || "param") + index}><td className="px-4 py-3 font-mono text-xs text-gray-900 dark:text-gray-100">{item.name}</td><td className="px-4 py-3 text-xs text-gray-600 dark:text-gray-300">{item.type || "-"}</td><td className="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">{item.required ? ts("是") : ts("否")}</td><td className="px-4 py-3 text-xs leading-5 text-gray-700 dark:text-gray-300">{item.description ? translateDocText(item.description) : "-"}</td></tr>)}</tbody></table></div>
          </section>}

          <section className={"mb-5 grid gap-5 " + (activeVariant.method === "GET" ? "" : "lg:grid-cols-2")}>
            {activeVariant.method !== "GET" && <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]"><div className="mb-3 flex items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("请求体")}</h2><CopyButton value={requestBodyText} label={ts("复制 JSON")} copiedLabel={ts("已复制")} /></div><pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100">{requestBodyText}</pre></div>}
            <div className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]"><div className="mb-3 flex items-center justify-between gap-2"><h2 className="font-semibold text-gray-900 dark:text-gray-100">{ts("响应示例")}</h2><CopyButton value={responseBodyText} label={ts("复制 JSON")} copiedLabel={ts("已复制")} /></div><pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs leading-5 text-gray-100">{responseBodyText}</pre></div>
          </section>

          {Object.keys(responseDefinitions).length > 0 && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]"><h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("响应状态")}</h2><div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/10"><table className="w-full text-left text-sm"><thead className="bg-gray-50 text-xs text-gray-600 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-4 py-3">HTTP</th><th className="px-4 py-3">{ts("说明")}</th><th className="px-4 py-3">{ts("响应体")}</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-white/10">{Object.entries(responseDefinitions).map(([status, value]) => <tr key={status}><td className="px-4 py-3 font-mono text-xs text-gray-900 dark:text-gray-100">{status}</td><td className="px-4 py-3 text-xs text-gray-700 dark:text-gray-300">{value?.description ? translateDocText(value.description) : "-"}</td><td className="px-4 py-3 text-xs text-gray-600 dark:text-gray-400">{value?.body ? ts("JSON") : "-"}</td></tr>)}</tbody></table></div></section>}

          {errors.length > 0 && <section className="mb-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/10 dark:bg-[#101827]"><h2 className="mb-3 font-semibold text-gray-900 dark:text-gray-100">{ts("错误码")}</h2><div className="overflow-hidden rounded-xl border border-gray-200 dark:border-white/10"><table className="w-full text-left text-sm"><thead className="bg-gray-50 text-xs text-gray-600 dark:bg-white/5 dark:text-gray-400"><tr><th className="px-4 py-3">{ts("HTTP 状态")}</th><th className="px-4 py-3">{ts("错误标识")}</th><th className="px-4 py-3">{ts("说明")}</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-white/10">{errors.map((item: any, index: number) => <tr key={String(item.code || "error") + index}><td className="px-4 py-3 font-mono text-xs text-gray-800 dark:text-gray-200">{item.status || "-"}</td><td className="px-4 py-3 font-mono text-xs text-red-600 dark:text-red-300">{item.code || "-"}</td><td className="px-4 py-3 text-xs text-gray-700 dark:text-gray-300">{item.description ? translateDocText(item.description) : "-"}</td></tr>)}</tbody></table></div></section>}

          {(activeVariant.requestMode === "images" || activeVariant.requestMode === "video" || activeVariant.requestMode === "audio") && <section className="rounded-2xl border border-blue-200 bg-blue-50/80 p-5 text-sm text-gray-700 dark:border-blue-400/20 dark:bg-blue-500/10 dark:text-gray-300"><h2 className="mb-2 font-semibold text-gray-900 dark:text-gray-100">{ts("异步任务流程")}</h2><p>{ts("生成接口返回任务状态后，请使用")} <code className="rounded bg-white px-1.5 py-0.5 text-xs dark:bg-white/10">/v1/tasks/&#123;task_no&#125;</code> {ts("查询结果；进度事件使用")} <code className="rounded bg-white px-1.5 py-0.5 text-xs dark:bg-white/10">/v1/tasks/&#123;task_no&#125;/events</code>。{ts("当模型轮询预算超过 10 分钟时，即使传入 wait=true 也会返回异步任务。参考视频和参考音频必须使用上游服务可访问的公网 URL。")}</p></section>}
        </div>
      </main>
    </div>
  );
}
