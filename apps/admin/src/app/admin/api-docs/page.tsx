"use client";

import { useEffect, useMemo, useState } from "react";
import { adminApi } from "@/lib/api";
import { AdminPagination } from "@/components/AdminPagination";

interface AdminModel {
  id: number;
  code: string;
  display_name: string;
  category: string;
  request_mode: string;
  new_api_model: string;
  new_api_endpoint: string;
  is_enabled: boolean;
}

interface APIDoc {
  id: number;
  model_id: number;
  model_code: string;
  model_name: string;
  category: string;
  request_mode: string;
  new_api_model: string;
  slug: string;
  title: string;
  summary: string;
  protocol: string;
  base_url: string;
  endpoint: string;
  auth_header: string;
  sdk: string;
  content: Record<string, unknown>;
  is_published: boolean;
  sort_order: number;
}

interface FormState {
  id?: number;
  model_id: number;
  slug: string;
  title: string;
  summary: string;
  protocol: string;
  base_url: string;
  endpoint: string;
  auth_header: string;
  sdk: string;
  content: string;
  is_published: boolean;
  sort_order: number;
}

const EMPTY: FormState = {
  model_id: 0,
  slug: "",
  title: "",
  summary: "",
  protocol: "openai-compatible",
  base_url: "https://api.your-starai-domain.com",
  endpoint: "/v1/chat/completions",
  auth_header: "Authorization: Bearer <API_KEY>",
  sdk: "openai (Node/Python), curl",
  content: JSON.stringify(
    {
      features: ["OpenAI 兼容", "Bearer 鉴权", "标准 JSON 响应"],
      request_example: {
        model: "MODEL_CODE",
        messages: [{ role: "user", content: "你好，请介绍你的能力" }],
      },
      status_code: 200,
      http_status: 200,
      response_status: 200,
      response_example: {
        code: 0,
        message: "ok",
        data: {
          request_id: "req_xxx",
          conversation_id: "conv_xxx",
          content: "这是模型响应内容",
          cost: 0.01,
        },
      },
      responses: {
        "200": {
          description: "请求成功",
          body: {
            code: 0,
            message: "ok",
            data: {
              request_id: "req_xxx",
              conversation_id: "conv_xxx",
              content: "这是模型响应内容",
              cost: 0.01,
            },
          },
        },
        "400": { description: "请求参数错误", body: { code: 400, message: "模型不存在或未启用" } },
        "401": { description: "API Key 无效或已停用", body: { code: 401, message: "API Key 无效或已停用" } },
        "502": { description: "上游模型服务异常", body: { code: 502, message: "模型服务异常" } },
      },
      notes: ["请使用平台 API Key 调用", "model 字段填写平台模型编码"],
    },
    null,
    2
  ),
  is_published: true,
  sort_order: 0,
};

const PAGE_SIZE = 10;

const DOC_SECTIONS = [
  { key: "chat", label: "聊天与文本", hint: "OpenAI、Anthropic、Gemini 三种聊天协议" },
  { key: "image", label: "图片", hint: "图片生成接口" },
  { key: "video", label: "视频", hint: "视频生成与异步任务接口" },
  { key: "audio", label: "音频", hint: "语音生成接口" },
  { key: "platform", label: "平台", hint: "模型列表、任务查询和任务事件" },
] as const;

function defaultEndpoint(mode: string, upstream?: string) {
  if (mode === "responses") return "/v1/responses";
  if (mode === "images") return "/v1/images/generations";
  if (mode === "video") return "/v1/video/generations";
  if (mode === "audio") return "/v1/audio/speech";
  if (mode === "custom" && upstream) return upstream;
  return "/v1/chat/completions";
}

function defaultProtocol(mode: string) {
  if (mode === "images") return "openai-compatible-image";
  if (mode === "video") return "new-api-compatible-video";
  if (mode === "audio") return "openai-compatible-audio";
  if (mode === "custom") return "custom-compatible";
  return "openai-compatible";
}

function validateContent(content: Record<string, unknown>) {
  const errors: string[] = [];
  if (!content.request_example || typeof content.request_example !== "object" || Array.isArray(content.request_example)) errors.push("content.request_example 必须是 JSON 对象");
  if (!content.response_example || typeof content.response_example !== "object" || Array.isArray(content.response_example)) errors.push("content.response_example 必须是 JSON 对象");
  if (content.parameters !== undefined) {
    if (!Array.isArray(content.parameters)) errors.push("content.parameters 必须是数组");
    else content.parameters.forEach((item, index) => {
      if (!item || typeof item !== "object" || !(item as Record<string, unknown>).name) errors.push(`content.parameters[${index}] 缺少 name`);
    });
  }
  if (content.responses !== undefined && (!content.responses || typeof content.responses !== "object" || Array.isArray(content.responses))) errors.push("content.responses 必须是按 HTTP 状态码组织的 JSON 对象");
  if (content.version !== undefined && typeof content.version !== "string") errors.push("content.version 必须是字符串，例如 v1");
  return errors;
}

export default function AdminApiDocsPage() {
  const [docs, setDocs] = useState<APIDoc[]>([]);
  const [models, setModels] = useState<AdminModel[]>([]);
  const [form, setForm] = useState<FormState>(EMPTY);
  const [showForm, setShowForm] = useState(false);
  const [msg, setMsg] = useState("");
  const [saving, setSaving] = useState(false);
  const [page, setPage] = useState(1);
  const [apiDocsEnabled, setApiDocsEnabled] = useState(true);
  const [siteBaseURL, setSiteBaseURL] = useState("");
  const [apiDocsSections, setApiDocsSections] = useState<Record<string, boolean>>(
    Object.fromEntries(DOC_SECTIONS.map((item) => [item.key, true]))
  );
  const [switchSaving, setSwitchSaving] = useState(false);

  const load = () => {
    adminApi<{ items: APIDoc[] }>("/api-docs").then((r) => setDocs(r.items || []));
    adminApi<AdminModel[]>("/models").then(setModels);
    adminApi<Record<string, unknown>>("/system-configs").then((cfg) => {
      if (typeof cfg.site_base_url === "string") setSiteBaseURL(cfg.site_base_url.trim().replace(/\/+$/, ""));
      setApiDocsEnabled(cfg.api_docs_enabled !== false);
      if (cfg.api_docs_operations && typeof cfg.api_docs_operations === "object") {
        setApiDocsSections({
          ...Object.fromEntries(DOC_SECTIONS.map((item) => [item.key, true])),
          ...(cfg.api_docs_operations as Record<string, boolean>),
        });
      }
    });
  };

  useEffect(() => {
    load();
  }, []);

  const usedModelIDs = useMemo(() => new Set(docs.filter((d) => d.id !== form.id).map((d) => d.model_id)), [docs, form.id]);
  const selectableModels = useMemo(() => models.filter((m) => !usedModelIDs.has(m.id)), [models, usedModelIDs]);
  const paginatedDocs = useMemo(() => docs.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE), [docs, page]);

  const selectModel = (modelID: number) => {
    const m = models.find((x) => x.id === modelID);
    if (!m) {
      setForm({ ...form, model_id: modelID });
      return;
    }
    const nextContent = JSON.parse(form.content || "{}");
    nextContent.request_example = {
      ...(nextContent.request_example || {}),
      model: m.code,
    };
    setForm({
      ...form,
      model_id: m.id,
      slug: form.slug || m.code,
      title: form.title || m.display_name,
      summary: form.summary || `${m.display_name} 标准兼容调用文档`,
      protocol: defaultProtocol(m.request_mode),
      endpoint: defaultEndpoint(m.request_mode, m.new_api_endpoint),
      content: JSON.stringify(nextContent, null, 2),
    });
  };

  const openCreate = () => {
    setForm({ ...EMPTY, base_url: siteBaseURL || EMPTY.base_url });
    setMsg("");
    setShowForm(true);
  };

  const openEdit = (doc: APIDoc) => {
    setForm({
      id: doc.id,
      model_id: doc.model_id,
      slug: doc.slug,
      title: doc.title,
      summary: doc.summary,
      protocol: doc.protocol,
      base_url: doc.base_url,
      endpoint: doc.endpoint,
      auth_header: doc.auth_header,
      sdk: doc.sdk,
      content: JSON.stringify(doc.content || {}, null, 2),
      is_published: doc.is_published,
      sort_order: doc.sort_order,
    });
    setMsg("");
    setShowForm(true);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setMsg("");
    let content: Record<string, unknown>;
    try {
      content = JSON.parse(form.content || "{}");
    } catch {
      setMsg("content JSON 格式错误");
      return;
    }
    const contentErrors = validateContent(content);
    if (contentErrors.length > 0) {
      setMsg(contentErrors[0]);
      return;
    }
    if (!form.model_id) {
      setMsg("请选择平台已接入模型");
      return;
    }
    setSaving(true);
    const payload = { ...form, content, id: undefined };
    try {
      if (form.id) {
        await adminApi(`/api-docs/${form.id}`, { method: "PATCH", body: JSON.stringify(payload) });
      } else {
        await adminApi("/api-docs", { method: "POST", body: JSON.stringify(payload) });
      }
      setShowForm(false);
      load();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  };

  const standardizeContent = () => {
    try {
      const current = JSON.parse(form.content || "{}");
      const model = models.find((item) => item.id === form.model_id);
      const modelCode = model?.code || "MODEL_CODE";
      const next = {
        operation_id: current.operation_id || form.slug || modelCode,
        version: current.version || "v1",
        request_content_type: current.request_content_type || "application/json",
        response_content_type: current.response_content_type || "application/json",
        ...current,
        request_example: current.request_example || { model: modelCode, messages: [{ role: "user", content: "你好，请介绍你的能力" }], stream: false },
        response_example: current.response_example || { id: "chatcmpl_xxx", object: "chat.completion", choices: [{ index: 0, message: { role: "assistant", content: "这是模型响应内容" }, finish_reason: "stop" }] },
        responses: current.responses || {
          "200": { description: "请求成功", body: { id: "chatcmpl_xxx", object: "chat.completion", choices: [] } },
          "400": { description: "请求参数错误", body: { error: { type: "invalid_request_error", code: "invalid_request_error", message: "请求参数错误" } } },
          "401": { description: "API Key 无效或已停用", body: { error: { type: "invalid_api_key", code: "invalid_api_key", message: "API Key 无效或已停用" } } },
        },
      };
      setForm({ ...form, content: JSON.stringify(next, null, 2) });
      setMsg("已补齐标准字段，请检查后保存");
    } catch {
      setMsg("content JSON 格式错误，无法标准化");
    }
  };

  const remove = async (doc: APIDoc) => {
    if (!confirm(`确认删除「${doc.title}」的 API 文档？`)) return;
    await adminApi(`/api-docs/${doc.id}`, { method: "DELETE" });
    load();
  };

  const togglePublicDocs = async (enabled: boolean) => {
    setSwitchSaving(true);
    try {
      await adminApi("/system-configs", { method: "PATCH", body: JSON.stringify({ api_docs_enabled: enabled }) });
      setApiDocsEnabled(enabled);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "保存显示开关失败");
    } finally {
      setSwitchSaving(false);
    }
  };

  const toggleDocSection = async (key: string, enabled: boolean) => {
    const next = { ...apiDocsSections, [key]: enabled };
    setSwitchSaving(true);
    try {
      await adminApi("/system-configs", { method: "PATCH", body: JSON.stringify({ api_docs_operations: next }) });
      setApiDocsSections(next);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "保存接口显示开关失败");
    } finally {
      setSwitchSaving(false);
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">API 文档管理</h1>
          <p className="text-sm text-gray-500 mt-1">
            文档必须选择平台已有模型；未接入模型不会出现在下拉列表中。
          </p>
        </div>
        <button onClick={openCreate} className="px-4 py-2 rounded-xl bg-primary text-dark font-semibold text-sm">
          新增 API 文档
        </button>
      </div>

      <div className="mb-6 flex items-center justify-between gap-4 rounded-2xl border border-indigo-100 bg-indigo-50/60 px-4 py-3">
        <div>
          <div className="text-sm font-semibold text-gray-900">前台 API 文档中心</div>
          <div className="mt-1 text-xs text-gray-500">关闭后会隐藏前台导航、阻止公开文档接口，并保留后台编辑数据。</div>
        </div>
        <button
          type="button"
          disabled={switchSaving}
          onClick={() => togglePublicDocs(!apiDocsEnabled)}
          className={`relative h-7 w-12 rounded-full transition ${apiDocsEnabled ? "bg-emerald-500" : "bg-gray-300"} disabled:opacity-50`}
          aria-label={apiDocsEnabled ? "隐藏 API 文档中心" : "显示 API 文档中心"}
        >
          <span className={`absolute top-1 h-5 w-5 rounded-full bg-white shadow transition ${apiDocsEnabled ? "left-6" : "left-1"}`} />
        </button>
      </div>
      <div className="mb-6 rounded-2xl border border-gray-200 bg-white p-4">
        <div className="mb-3">
          <div className="text-sm font-semibold text-gray-900">文档分组显示</div>
          <div className="mt-1 text-xs text-gray-500">可单独隐藏某一类接口；隐藏只影响文档中心展示，不会停用实际 API。</div>
        </div>
        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-5">
          {DOC_SECTIONS.map((section) => {
            const enabled = apiDocsSections[section.key] !== false;
            return <button key={section.key} type="button" disabled={switchSaving || !apiDocsEnabled} onClick={() => toggleDocSection(section.key, !enabled)} className={`rounded-xl border px-3 py-3 text-left transition ${enabled ? "border-emerald-200 bg-emerald-50/60" : "border-gray-200 bg-gray-50"} disabled:cursor-not-allowed disabled:opacity-50`}>
              <div className="flex items-center justify-between gap-2"><span className="text-sm font-semibold text-gray-900">{section.label}</span><span className={`relative h-5 w-9 rounded-full transition ${enabled ? "bg-emerald-500" : "bg-gray-300"}`}><span className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition ${enabled ? "left-4" : "left-0.5"}`} /></span></div>
              <div className="mt-1 text-[11px] leading-4 text-gray-500">{section.hint}</div>
            </button>;
          })}
        </div>
      </div>
      {msg && !showForm && <p className="mb-4 text-sm text-red-500">{msg}</p>}

      {showForm && (
        <form onSubmit={submit} className="bg-white rounded-2xl p-6 border mb-6 grid grid-cols-2 gap-4">
          <div className="col-span-2 flex items-center justify-between">
            <h2 className="font-semibold">{form.id ? "编辑 API 文档" : "新增 API 文档"}</h2>
            <button type="button" onClick={() => setShowForm(false)} className="text-sm text-gray-400 hover:text-gray-600">
              取消
            </button>
          </div>

          <div>
            <label className="text-xs text-gray-500">绑定模型（仅平台已接入）</label>
            <select
              value={form.model_id}
              onChange={(e) => selectModel(Number(e.target.value))}
              className="w-full mt-1 px-3 py-2 rounded-lg border text-sm bg-white"
              required
            >
              <option value={0}>请选择模型</option>
              {selectableModels.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.display_name}（{m.code} / {m.request_mode}）
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs text-gray-500">Slug</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value })} />
          </div>
          <div>
            <label className="text-xs text-gray-500">标题</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required />
          </div>
          <div>
            <label className="text-xs text-gray-500">协议类型</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.protocol} onChange={(e) => setForm({ ...form, protocol: e.target.value })} />
          </div>
          <div className="col-span-2">
            <label className="text-xs text-gray-500">简介</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.summary} onChange={(e) => setForm({ ...form, summary: e.target.value })} />
          </div>
          <div>
            <label className="text-xs text-gray-500">Base URL</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.base_url} onChange={(e) => setForm({ ...form, base_url: e.target.value })} />
          </div>
          <div>
            <label className="text-xs text-gray-500">Endpoint</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} />
          </div>
          <div>
            <label className="text-xs text-gray-500">鉴权 Header</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.auth_header} onChange={(e) => setForm({ ...form, auth_header: e.target.value })} />
          </div>
          <div>
            <label className="text-xs text-gray-500">SDK 建议</label>
            <input className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.sdk} onChange={(e) => setForm({ ...form, sdk: e.target.value })} />
          </div>
          <div className="col-span-2">
            <div className="flex items-center justify-between gap-3"><label className="text-xs text-gray-500">标准文档 JSON（operation_id / version / request_example / response_example / parameters / responses / errors）</label><button type="button" onClick={standardizeContent} className="shrink-0 rounded-lg border border-indigo-200 bg-indigo-50 px-2.5 py-1.5 text-xs text-indigo-700 hover:bg-indigo-100">补齐标准字段</button></div>
            <textarea className="w-full mt-1 px-3 py-2 rounded-lg border text-xs font-mono h-64" value={form.content} onChange={(e) => setForm({ ...form, content: e.target.value })} />
            <p className="mt-1 text-[11px] leading-5 text-gray-400">保存时会校验请求示例、响应示例、参数数组和 HTTP 响应定义；后端仍会自动补齐平台标准字段。</p>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.is_published} onChange={(e) => setForm({ ...form, is_published: e.target.checked })} />
            发布到前台
          </label>
          <div>
            <label className="text-xs text-gray-500">排序</label>
            <input type="number" className="w-full mt-1 px-3 py-2 rounded-lg border text-sm" value={form.sort_order} onChange={(e) => setForm({ ...form, sort_order: Number(e.target.value) || 0 })} />
          </div>
          {msg && <p className={`col-span-2 text-sm ${msg.includes("失败") || msg.includes("错误") ? "text-red-500" : "text-emerald-600"}`}>{msg}</p>}
          <button type="submit" disabled={saving} className="col-span-2 py-2 bg-primary rounded-xl text-dark font-semibold disabled:opacity-50">
            {saving ? "保存中..." : "保存 API 文档"}
          </button>
        </form>
      )}

      <div className="bg-white rounded-2xl border overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 text-gray-500">
            <tr>
              <th className="text-left px-4 py-3">文档</th>
              <th className="text-left px-4 py-3">绑定模型</th>
              <th className="text-left px-4 py-3">端点</th>
              <th className="text-left px-4 py-3">状态</th>
              <th className="text-left px-4 py-3">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {paginatedDocs.map((d) => (
              <tr key={d.id}>
                <td className="px-4 py-3">
                  <div className="font-medium text-gray-900">{d.title}</div>
                  <div className="text-xs text-gray-400 font-mono">{d.slug}</div>
                </td>
                <td className="px-4 py-3">
                  <div>{d.model_name}</div>
                  <div className="text-xs text-gray-400">{d.model_code}</div>
                </td>
                <td className="px-4 py-3 text-xs font-mono text-gray-500">{d.endpoint}</td>
                <td className="px-4 py-3">
                  {d.is_published ? (
                    <span className="text-green-600">已发布</span>
                  ) : (
                    <span className="text-gray-400">草稿</span>
                  )}
                </td>
                <td className="px-4 py-3 space-x-3">
                  <button onClick={() => openEdit(d)} className="text-xs text-secondary hover:underline">编辑</button>
                  <button onClick={() => remove(d)} className="text-xs text-red-500 hover:underline">删除</button>
                </td>
              </tr>
            ))}
            {docs.length === 0 && (
              <tr>
                <td colSpan={5} className="text-center text-gray-400 py-8">暂无 API 文档</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <AdminPagination page={page} total={docs.length} pageSize={PAGE_SIZE} onPageChange={setPage} />
    </div>
  );
}
