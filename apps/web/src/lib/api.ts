const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export function hasUserSession() {
  if (typeof window === "undefined") return false;
  return localStorage.getItem("starai_session") === "1" || !!localStorage.getItem("token");
}

export function legacyAuthHeaders(): Record<string, string> {
  if (typeof window === "undefined") return {};
  const token = localStorage.getItem("token");
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function localeHeaders(): Record<string, string> {
  if (typeof window === "undefined") return {};
  const locale = localStorage.getItem("site_locale") || "zh-CN";
  return { "X-Locale": locale, "Accept-Language": locale };
}

function responseMessage(value: unknown): string {
  if (!value || typeof value !== "object") return "";
  const body = value as { message?: unknown; error?: unknown };
  if (typeof body.message === "string") return body.message.trim();
  if (body.error && typeof body.error === "object") {
    const message = (body.error as { message?: unknown }).message;
    if (typeof message === "string") return message.trim();
  }
  return "";
}

function plainText(raw: string): string {
  return raw
    .replace(/<(script|style)[^>]*>[\s\S]*?<\/\1>/gi, " ")
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

const GATEWAY_ERROR_PATTERN = /<(!doctype|html)|cloudflare|bad gateway|gateway time-?out|error code \d{3}/i;

/**
 * Build a user-facing message for a failed HTTP response. Gateway pages
 * (Cloudflare/nginx 5xx HTML, empty bodies from a proxy that gave up on a slow
 * origin) are replaced by a short retry hint instead of dumping raw markup into
 * the UI.
 */
function httpFailureMessage(status: number, raw: string, fallback: string): string {
  let json: unknown = null;
  try {
    json = raw ? JSON.parse(raw) : null;
  } catch {
    json = null;
  }
  const message = responseMessage(json);
  if (message) return message;
  if (status >= 500 || GATEWAY_ERROR_PATTERN.test(raw)) {
    return `${fallback}（HTTP ${status}），服务暂时不可用，请稍后重试`;
  }
  const detail = plainText(raw).slice(0, 160);
  return `${fallback}（HTTP ${status}）${detail ? `：${detail}` : ""}`;
}

async function parseResponse<T>(res: Response, fallback: string): Promise<T> {
  const raw = await res.text();
  let json: unknown;
  try {
    json = raw ? JSON.parse(raw) : null;
  } catch {
    json = null;
  }

  if (!res.ok) {
    throw new Error(responseMessage(json) || httpFailureMessage(res.status, raw, fallback));
  }

  if (json === null && raw) {
    throw new Error(httpFailureMessage(res.status, raw, fallback));
  }

  if (json && typeof json === "object" && "code" in json) {
    const envelope = json as { code?: unknown; message?: unknown; data?: unknown };
    if (typeof envelope.code === "number" && envelope.code !== 0) {
      throw new Error(responseMessage(json) || fallback);
    }
    return envelope.data as T;
  }

  return json as T;
}

export type ChatStreamHandlers = {
  onContent?: (delta: string, accumulated: string) => void;
  onReasoning?: (delta: string, accumulated: string) => void;
  signal?: AbortSignal;
};

export type ChatStreamResult = {
  content: string;
  reasoning: string;
  cost: number;
};

/**
 * Run a chat completion over SSE and accumulate the answer on the client.
 *
 * Long multimodal calls (video analysis, long structured answers) can easily
 * take one to three minutes. A buffered request stays silent for that whole
 * time, which proxies in front of the API eventually abort with their own
 * gateway error page; streaming returns bytes within the first seconds and
 * keeps the connection alive until the model finishes.
 */
export async function streamChatCompletion(
  path: string,
  payload: Record<string, unknown>,
  handlers: ChatStreamHandlers = {}
): Promise<ChatStreamResult> {
  const res = await fetch(`${API_URL}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...localeHeaders(),
      ...legacyAuthHeaders(),
    },
    credentials: "include",
    body: JSON.stringify({ ...payload, stream: true }),
    signal: handlers.signal,
  });

  if (!res.ok) {
    throw new Error(httpFailureMessage(res.status, await res.text(), "请求失败"));
  }

  const contentType = res.headers.get("Content-Type") || "";
  if (!res.body || !contentType.includes("text/event-stream")) {
    const raw = await res.text();
    let json: unknown = null;
    try {
      json = raw ? JSON.parse(raw) : null;
    } catch {
      json = null;
    }
    const body = (json || {}) as { content?: unknown; reasoning_content?: unknown; cost?: unknown };
    const message = responseMessage(json);
    if (message && !body.content) throw new Error(message);
    return {
      content: typeof body.content === "string" ? body.content : "",
      reasoning: typeof body.reasoning_content === "string" ? body.reasoning_content : "",
      cost: Number(body.cost || 0),
    };
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let content = "";
  let reasoning = "";
  let cost = 0;

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
    const events = buffer.split("\n\n");
    buffer = events.pop() || "";
    for (const event of events) {
      let dataText = "";
      for (const line of event.split("\n")) {
        if (line.startsWith("data:")) dataText += line.slice(5).trim();
      }
      if (!dataText || dataText === "[DONE]") continue;
      let data: Record<string, unknown> | null = null;
      try {
        data = JSON.parse(dataText) as Record<string, unknown>;
      } catch {
        continue;
      }
      const streamError = data?.error;
      if (streamError && typeof streamError === "object") {
        const message = (streamError as { message?: unknown }).message;
        throw new Error(typeof message === "string" && message.trim() ? message : "模型服务异常");
      }
      const choice = Array.isArray(data?.choices) ? (data?.choices as Array<Record<string, unknown>>)[0] : undefined;
      const delta = (choice?.delta || {}) as { content?: unknown; reasoning_content?: unknown };
      if (typeof delta.content === "string" && delta.content) {
        content += delta.content;
        handlers.onContent?.(delta.content, content);
      }
      if (typeof delta.reasoning_content === "string" && delta.reasoning_content) {
        reasoning += delta.reasoning_content;
        handlers.onReasoning?.(delta.reasoning_content, reasoning);
      }
      const streamCost = Number(data?.cost);
      if (Number.isFinite(streamCost) && streamCost > 0) cost = streamCost;
    }
  }

  return { content, reasoning, cost };
}

export async function api<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...localeHeaders(),
    ...legacyAuthHeaders(),
    ...(options.headers as Record<string, string>),
  };

  const res = await fetch(`${API_URL}${path}`, { ...options, headers, credentials: "include" });
  return parseResponse<T>(res, "请求失败");
}

/**
 * Fetch localized content with the locale captured by the React render that
 * started the request. Reading localStorage inside api() alone is not enough:
 * an older request may finish after a language switch and overwrite the newer
 * result. Callers should also pass an AbortSignal when the locale can change.
 */
export function apiForLocale<T>(
  path: string,
  locale: string,
  options: RequestInit = {}
): Promise<T> {
  return api<T>(path, {
    ...options,
    headers: {
      "X-Locale": locale,
      "Accept-Language": locale,
      ...(options.headers as Record<string, string>),
    },
  });
}

export async function uploadFile(file: File): Promise<string> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`${API_URL}/api/upload`, {
    method: "POST",
    headers: { ...legacyAuthHeaders(), ...localeHeaders() },
    credentials: "include",
    body: form,
  });
  const data = await parseResponse<{ url: string }>(res, "上传失败");
  return data.url;
}

export async function uploadAsset(
  file: File,
  meta?: { name?: string; description?: string; kind?: string; asset_type?: string }
): Promise<{ public_id: string; url: string; name?: string; kind?: string; asset_type?: string; mime_type?: string; size_bytes?: number }> {
  const form = new FormData();
  form.append("file", file);
  if (meta?.name) form.append("name", meta.name);
  if (meta?.description) form.append("description", meta.description);
  if (meta?.kind) form.append("kind", meta.kind);
  if (meta?.asset_type) form.append("asset_type", meta.asset_type);
  const res = await fetch(`${API_URL}/api/assets/upload`, {
    method: "POST",
    headers: { ...legacyAuthHeaders(), ...localeHeaders() },
    credentials: "include",
    body: form,
  });
  return parseResponse<{ public_id: string; url: string; name?: string; kind?: string; asset_type?: string; mime_type?: string; size_bytes?: number }>(res, "上传失败");
}

export async function importAssetFromURL(url: string, name?: string) {
  return api<{ public_id: string; work_public_id: string; url: string; name?: string; kind: string; asset_type: string; mime_type?: string; size_bytes?: number; duration_seconds?: number }>("/api/assets/import-url", {
    method: "POST",
    body: JSON.stringify({ url, ...(name ? { name } : {}) }),
  });
}

export async function listAssets(params: { q?: string; tag?: string; kind?: string; type?: string; page?: number; page_size?: number } = {}) {
  const sp = new URLSearchParams();
  if (params.q) sp.set("q", params.q);
  if (params.tag) sp.set("tag", params.tag);
  if (params.kind) sp.set("kind", params.kind);
  if (params.type) sp.set("type", params.type);
  if (params.page) sp.set("page", String(params.page));
  if (params.page_size) sp.set("page_size", String(params.page_size));
  const suffix = sp.toString() ? `?${sp.toString()}` : "";
  return api<{ items: any[]; total: number }>(`/api/assets${suffix}`);
}

export async function deleteAsset(publicId: string) {
  return api<null>(`/api/assets/${encodeURIComponent(publicId)}`, { method: "DELETE" });
}

export async function listRoles() {
  return api<{ items: any[] }>("/api/roles");
}

export async function createRole(payload: { name: string; description?: string; system_prompt: string; icon_url?: string; is_default?: boolean }) {
  return api(`/api/roles`, { method: "POST", body: JSON.stringify(payload) });
}

export async function listRoleTemplates() {
  return api<{ items: any[] }>("/api/role-templates");
}

export async function listChannelPresets() {
  return api<{ items: any[] }>("/api/channel-presets");
}

export { API_URL };
