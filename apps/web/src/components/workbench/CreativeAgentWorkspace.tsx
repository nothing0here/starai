"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { ArrowUp, AudioLines, ChevronDown, ChevronLeft, ChevronRight, Download, HelpCircle, History, ImageIcon, Loader2, Maximize2, Menu, Plus, SlidersHorizontal, Sparkles, Upload, UserRound, Video, X } from "lucide-react";
import type { Model, User } from "@starai/shared-types";
import { api, uploadAsset } from "@/lib/api";
import { useSiteBranding } from "@/components/SiteBrand";
import { WorkbenchTopActions } from "@/components/WorkbenchTopActions";
import { AgentLanding } from "./AgentLanding";
import { ChatTopTools, type BottomBarState } from "./BottomBar";
import { AGENT_THEMES } from "./categoryMeta";

type Attachment = { public_id: string; name: string; url: string; kind?: string };
type Message = { role: "user" | "assistant"; content: string; images?: string[]; videos?: string[]; audios?: string[]; attachments?: Attachment[] };
type Plan = { intent?: string; reply?: string; prompt?: string; params?: Record<string, unknown>; needs_confirm?: boolean };
type TaskState = { task_no: string; type?: string; status: string; progress?: number; input?: Record<string, unknown>; output?: Record<string, unknown>; error_message?: string };
type HistoryItem = { public_id: string; title?: string | null; updated_at: string };
type AgentConfig = { runtime_config?: { analysis_model_code?: string; image_model_code?: string; video_model_code?: string; speech_model_code?: string; music_model_code?: string } };
type AgentEvent = { type?: string; asset_ids?: string[]; task_no?: string; media_type?: string; prompt?: string };
type AssetRecord = { public_id: string; name?: string; url: string; kind?: string; mime_type?: string };
type MediaSet = { images: string[]; videos: string[]; audios: string[] };
type MediaPreview = { url: string; type: "image" | "video" };
type GenerationType = "image" | "video" | "speech" | "music";

const HOT_PROMPTS = ["生成一张产品主图", "做一个 10 秒产品展示视频", "把这段文字合成自然旁白", "为品牌写一首宣传歌曲"];
const CREATIVE_FEATURES = [
  { icon: "✦", title: "理解并连续创作", subtitle: "从自然语言识别目标，自动整理提示词并衔接上一轮结果" },
  { icon: "🖼️", title: "图片生成与改图", subtitle: "支持参考素材、连续改图和多种图片生成模型" },
  { icon: "🎬", title: "视频生成", subtitle: "可直接使用刚生成的图片继续生成视频" },
  { icon: "🎵", title: "语音与音乐", subtitle: "分别调用文本转语音或歌曲音乐模型完成创作" },
];
const MOBILE_FEATURE_ICONS = [Sparkles, ImageIcon, Video, AudioLines];

function urls(value: unknown): string[] {
  if (typeof value === "string") return value.trim() ? [value.trim()] : [];
  if (Array.isArray(value)) return value.flatMap(urls);
  if (value && typeof value === "object") {
    const item = value as Record<string, unknown>;
    return urls(item.url || item.image_url || item.video_url || item.result_url);
  }
  return [];
}

function taskMedia(output?: Record<string, unknown>, mediaType?: string) {
  if (!output) return { images: [] as string[], videos: [] as string[], audios: [] as string[] };
  const images = [...urls(output.image_url), ...urls(output.images)];
  const videos = [...urls(output.video_url), ...urls(output.videos)];
  const audios = [...urls(output.audio_url), ...urls(output.audios)];
  const generic = [...urls(output.url), ...urls(output.urls), ...urls(output.results), ...urls(output.data)];
  if (mediaType === "video" || (videos.length > 0 && images.length === 0)) videos.push(...generic);
  else if (mediaType === "audio" || audios.length > 0) audios.push(...generic);
  else images.push(...generic);
  return { images: Array.from(new Set(images)), videos: Array.from(new Set(videos)), audios: Array.from(new Set(audios)) };
}

function taskReferences(input?: Record<string, unknown>) {
  if (!input) return { images: [], videos: [], audios: [] };
  return {
    images: Array.from(new Set([...urls(input.reference_images), ...urls(input.first_frame), ...urls(input.last_frame)])),
    videos: Array.from(new Set([...urls(input.reference_videos), ...urls(input.source_video)])),
    audios: Array.from(new Set([...urls(input.reference_audio), ...urls(input.reference_audios)])),
  };
}

function isMusicModel(model: Model) {
  const audio = (model.runtime_rule?.audio || {}) as Record<string, unknown>;
  return audio.input_layout === "dual" || /(music|suno|音乐|歌曲)/i.test(`${model.code} ${model.display_name || ""} ${(model.tags || []).join(" ")}`);
}

function generationLabel(type: string) {
  return type === "video" ? "视频" : type === "speech" ? "语音" : type === "music" ? "歌曲音乐" : "图片";
}

function storedPlan(content: string): Plan | null {
  try {
    const value = JSON.parse(content.trim().replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, "")) as Plan;
    return value && typeof value === "object" ? value : null;
  } catch {
    return null;
  }
}

function storedEvent(content: string): AgentEvent | null {
  try {
    const value = JSON.parse(content) as AgentEvent;
    return value?.type?.startsWith("creative_agent_") ? value : null;
  } catch {
    return null;
  }
}

export function CreativeAgentWorkspace({
  onOpenModelPicker,
  onOpenNav,
  onRecharge,
  walletBalance,
}: {
  onOpenModelPicker?: () => void;
  onOpenNav?: () => void;
  onRecharge?: () => void;
  walletBalance?: number;
}) {
  const { site_logo: siteLogo, site_name: siteName } = useSiteBranding();
  const [messages, setMessages] = useState<Message[]>([]);
  const [user, setUser] = useState<User | null>(null);
  const [prompt, setPrompt] = useState("");
  const [imageModels, setImageModels] = useState<Model[]>([]);
  const [videoModels, setVideoModels] = useState<Model[]>([]);
  const [speechModels, setSpeechModels] = useState<Model[]>([]);
  const [musicModels, setMusicModels] = useState<Model[]>([]);
  const [chatModelCode, setChatModelCode] = useState("");
  const [imageModelCode, setImageModelCode] = useState("");
  const [videoModelCode, setVideoModelCode] = useState("");
  const [defaultImageModelCode, setDefaultImageModelCode] = useState("");
  const [defaultVideoModelCode, setDefaultVideoModelCode] = useState("");
  const [speechModelCode, setSpeechModelCode] = useState("");
  const [musicModelCode, setMusicModelCode] = useState("");
  const [defaultSpeechModelCode, setDefaultSpeechModelCode] = useState("");
  const [defaultMusicModelCode, setDefaultMusicModelCode] = useState("");
  const [customEnabled, setCustomEnabled] = useState(false);
  const [customMediaType, setCustomMediaType] = useState<GenerationType>("image");
  const [bottom, setBottom] = useState<BottomBarState>({
    channel_key: "price_first",
    fallback_enabled: true,
    web_search: false,
    timeout_sec: 30,
    asset_ids: [],
    files: [],
  });
  const [conversationId, setConversationId] = useState("");
  const [busy, setBusy] = useState(false);
  const [task, setTask] = useState<TaskState | null>(null);
  const [error, setError] = useState("");
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyItems, setHistoryItems] = useState<HistoryItem[]>([]);
  const [historyLoadingId, setHistoryLoadingId] = useState("");
  const [activeFeature, setActiveFeature] = useState(0);
  const [deepThink, setDeepThink] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [guideOpen, setGuideOpen] = useState(false);
  const [latestGeneratedMedia, setLatestGeneratedMedia] = useState<MediaSet>({ images: [], videos: [], audios: [] });
  const [mediaPreview, setMediaPreview] = useState<MediaPreview | null>(null);
  const uploadRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    Promise.all([api<Model[]>("/api/models?category=chat"), api<Model[]>("/api/models?category=image"), api<Model[]>("/api/models?category=video"), api<Model[]>("/api/models?category=audio"), api<AgentConfig>("/api/agents/general_creative_agent"), api<User>("/api/me").catch(() => null)])
      .then(([chats, images, videos, audios, agent, currentUser]) => {
        const enabled = (items: Model[]) => (items || []).filter((item) => item.is_enabled !== false);
        const nextChats = enabled(chats).filter((item) => item.category === "chat" && item.code !== "multi_collab_chat" && !/多模型协作|multi.?collab/i.test(`${item.code} ${item.display_name || ""}`));
        const nextImages = enabled(images);
        const nextVideos = enabled(videos);
        const nextAudios = enabled(audios);
        const nextSpeech = nextAudios.filter((item) => !isMusicModel(item));
        const nextMusic = nextAudios.filter(isMusicModel);
        setImageModels(nextImages);
        setVideoModels(nextVideos);
        setSpeechModels(nextSpeech);
        setMusicModels(nextMusic);
        const config = agent.runtime_config || {};
        setChatModelCode(nextChats.find((item) => item.code === config.analysis_model_code)?.code || nextChats.find((item) => item.code === "chat_demo_v1")?.code || nextChats[0]?.code || "");
        const defaultImage = nextImages.find((item) => item.code === config.image_model_code)?.code || nextImages[0]?.code || "";
        const defaultVideo = nextVideos.find((item) => item.code === config.video_model_code)?.code || nextVideos[0]?.code || "";
        setImageModelCode(defaultImage);
        setVideoModelCode(defaultVideo);
        setDefaultImageModelCode(defaultImage);
        setDefaultVideoModelCode(defaultVideo);
        const defaultSpeech = nextSpeech.find((item) => item.code === config.speech_model_code)?.code || nextSpeech[0]?.code || "";
        const defaultMusic = nextMusic.find((item) => item.code === config.music_model_code)?.code || nextMusic[0]?.code || "";
        setSpeechModelCode(defaultSpeech);
        setMusicModelCode(defaultMusic);
        setDefaultSpeechModelCode(defaultSpeech);
        setDefaultMusicModelCode(defaultMusic);
        setUser(currentUser);
      })
      .catch(() => setError("模型列表加载失败，请稍后重试"));
  }, []);

  useEffect(() => {
    if (!task || task.status === "succeeded" || task.status === "failed" || task.status === "cancelled") return;
    const timer = window.setInterval(() => {
      api<TaskState>(`/api/tasks/${task.task_no}`)
        .then((next) => {
          setTask(next);
          if (next.status === "succeeded") {
            const media = taskMedia(next.output, next.type);
            setLatestGeneratedMedia(media);
            setMessages((current) => [...current, { role: "assistant", content: "生成完成", images: media.images, videos: media.videos, audios: media.audios }]);
          }
        })
        .catch(() => undefined);
    }, 2000);
    return () => window.clearInterval(timer);
  }, [task]);

  useEffect(() => {
    if (messages.length > 0 || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const timer = window.setInterval(() => {
      if (!document.hidden) setActiveFeature((current) => (current + 1) % CREATIVE_FEATURES.length);
    }, 4500);
    return () => window.clearInterval(timer);
  }, [messages.length]);

  const assetIDs = useMemo(
    () => Array.from(new Set([...bottom.asset_ids, ...bottom.files.map((item) => item.public_id)])),
    [bottom.asset_ids, bottom.files]
  );
  const customModels = customMediaType === "video" ? videoModels : customMediaType === "speech" ? speechModels : customMediaType === "music" ? musicModels : imageModels;
  const customModelCode = customMediaType === "video" ? videoModelCode : customMediaType === "speech" ? speechModelCode : customMediaType === "music" ? musicModelCode : imageModelCode;
  const activeMobileFeature = CREATIVE_FEATURES[activeFeature] || CREATIVE_FEATURES[0];
  const ActiveMobileFeatureIcon = MOBILE_FEATURE_ICONS[activeFeature] || Sparkles;

  const generationModelFor = (mediaType: GenerationType) => {
    const models = mediaType === "video" ? videoModels : mediaType === "speech" ? speechModels : mediaType === "music" ? musicModels : imageModels;
    const selectedCode = customEnabled
      ? mediaType === "video" ? videoModelCode : mediaType === "speech" ? speechModelCode : mediaType === "music" ? musicModelCode : imageModelCode
      : mediaType === "video" ? defaultVideoModelCode : mediaType === "speech" ? defaultSpeechModelCode : mediaType === "music" ? defaultMusicModelCode : defaultImageModelCode;
    return models.find((item) => item.code === selectedCode) || models[0] || null;
  };

  const uploadFiles = async (files: FileList | null) => {
    if (!files?.length) return;
    const selected = Array.from(files).slice(0, Math.max(0, 10 - bottom.files.length));
    if (!selected.length) return;
    setUploading(true);
    setError("");
    try {
      const uploaded: BottomBarState["files"] = [];
      for (const file of selected) {
        const kind = file.type.startsWith("image/") ? "image" : file.type.startsWith("video/") ? "video" : file.type.startsWith("audio/") ? "audio" : "doc";
        const asset = await uploadAsset(file, { name: file.name, kind, asset_type: "prop" });
        uploaded.push({ public_id: asset.public_id, url: asset.url, name: file.name });
      }
      setLatestGeneratedMedia({ images: [], videos: [], audios: [] });
      setBottom((current) => ({ ...current, files: [...current.files, ...uploaded] }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "素材上传失败");
    } finally {
      setUploading(false);
    }
  };

  const createGeneration = async (nextPlan: Plan, activeConversationId: string) => {
    const mediaType = customEnabled ? customMediaType : nextPlan.intent;
    if (mediaType !== "image" && mediaType !== "video" && mediaType !== "speech" && mediaType !== "music") throw new Error("Agent 未能识别生成类型");
    const model = generationModelFor(mediaType);
    if (!model) throw new Error(`后台尚未配置可用的${generationLabel(mediaType)}模型`);
    const generationPrompt = nextPlan.prompt?.trim();
    if (!generationPrompt) throw new Error("Agent 未能生成有效提示词，请补充需求后重试");
    const result = await api<TaskState>("/api/creative-agent/generate", {
      method: "POST",
      body: JSON.stringify({
        conversation_id: activeConversationId,
        media_type: mediaType,
        model_code: model.code,
        prompt: generationPrompt,
        params: { ...(model.default_params || {}), ...(nextPlan.params || {}) },
        asset_ids: assetIDs,
        reference_asset_ids: latestGeneratedMedia.images.length || latestGeneratedMedia.videos.length || latestGeneratedMedia.audios.length ? [] : assetIDs,
        reference_image_urls: latestGeneratedMedia.images,
        reference_video_urls: latestGeneratedMedia.videos,
        reference_audio_urls: latestGeneratedMedia.audios,
      }),
    });
    setTask(result);
    setMessages((current) => [...current, { role: "assistant", content: `已自动创建${generationLabel(mediaType)}任务：${result.task_no}` }]);
  };

  const sendMessage = async () => {
    const text = prompt.trim();
    if (!text || busy || !chatModelCode) return;
    const nextMessages = [...messages, { role: "user" as const, content: text }];
    setMessages(nextMessages);
    setPrompt("");
    setError("");
    setBusy(true);
    try {
      const result = await api<{ conversation_id?: string; plan?: Plan }>("/api/creative-agent/plan", {
        method: "POST",
        body: JSON.stringify({
          model_code: chatModelCode,
          conversation_id: conversationId,
          messages: nextMessages.map((item) => ({ role: item.role, content: item.content })),
          asset_ids: assetIDs,
          deep_think: deepThink,
          preferred_media_type: customEnabled ? customMediaType : "",
        }),
      });
      const activeConversationId = result.conversation_id || conversationId;
      if (activeConversationId) setConversationId(activeConversationId);
      const nextPlan = { ...(result.plan || { intent: "chat", reply: "暂时无法理解这次需求，请换一种说法。" }) };
      if (customEnabled) nextPlan.intent = customMediaType;
      const nextIntent = nextPlan.intent;
      if (nextIntent === "image" || nextIntent === "video" || nextIntent === "speech" || nextIntent === "music") {
        setMessages((current) => [...current, { role: "assistant", content: nextPlan.reply || `已理解需求，正在创建${generationLabel(nextIntent)}任务。` }]);
        await createGeneration(nextPlan, activeConversationId);
      } else {
        setMessages((current) => [...current, { role: "assistant", content: nextPlan.reply || "请继续描述你的需求。" }]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "智能体分析失败");
    } finally {
      setBusy(false);
    }
  };

  const resetSession = () => {
    setMessages([]);
    setPrompt("");
    setTask(null);
    setConversationId("");
    setError("");
    setHistoryOpen(false);
    setCustomEnabled(false);
    setLatestGeneratedMedia({ images: [], videos: [], audios: [] });
    setImageModelCode(defaultImageModelCode);
    setVideoModelCode(defaultVideoModelCode);
    setSpeechModelCode(defaultSpeechModelCode);
    setMusicModelCode(defaultMusicModelCode);
    setBottom((current) => ({ ...current, asset_ids: [], files: [] }));
  };

  const openHistory = () => {
    const next = !historyOpen;
    setHistoryOpen(next);
    if (!next) return;
    api<HistoryItem[]>("/api/chat/conversations")
      .then((items) => setHistoryItems((items || []).filter((item) => /^(?:Agent 通用智能体|Ageng 通用智能体|Agneg 通用智能体|通用智能体)/.test(item.title || ""))))
      .catch(() => setHistoryItems([]));
  };

  const loadHistory = async (publicID: string) => {
    setHistoryLoadingId(publicID);
    setHistoryOpen(false);
    setError("");
    try {
      const conversation = await api<{ messages?: Array<{ role: string; content: string }> }>(`/api/chat/conversations/${publicID}`);
      const restored: Message[] = [];
      const assetTargets: Array<{ messageIndex: number; assetIds: string[] }> = [];
      const taskTargets: Array<{ messageIndex: number; userMessageIndex: number; taskNo: string }> = [];
      for (const item of conversation.messages || []) {
        if (item.role === "user") {
          restored.push({ role: "user", content: item.content });
          continue;
        }
        if (item.role === "assistant") {
          const saved = storedPlan(item.content);
          restored.push({ role: "assistant", content: saved?.reply || saved?.prompt || item.content });
          continue;
        }
        const event = storedEvent(item.content);
        if ((event?.type === "creative_agent_assets" || event?.type === "creative_agent_generation") && event.asset_ids?.length) {
          for (let index = restored.length - 1; index >= 0; index -= 1) {
            if (restored[index].role === "user") {
              assetTargets.push({ messageIndex: index, assetIds: event.asset_ids });
              break;
            }
          }
        }
        if (event?.type === "creative_agent_generation" && event.task_no) {
          let userMessageIndex = -1;
          for (let index = restored.length - 1; index >= 0; index -= 1) {
            if (restored[index].role === "user") {
              userMessageIndex = index;
              break;
            }
          }
          const messageIndex = restored.push({ role: "assistant", content: `生成任务：${event.task_no}` }) - 1;
          taskTargets.push({ messageIndex, userMessageIndex, taskNo: event.task_no });
        }
      }
      if (restored.length === 0) throw new Error("该历史会话暂无可恢复内容");
      const allAssetIds = Array.from(new Set(assetTargets.flatMap((item) => item.assetIds)));
      const [assets, tasks] = await Promise.all([
        Promise.all(allAssetIds.map((id) => api<AssetRecord>(`/api/assets/${encodeURIComponent(id)}`).catch(() => null))),
        Promise.all(taskTargets.map((item) => api<TaskState>(`/api/tasks/${encodeURIComponent(item.taskNo)}`).catch(() => null))),
      ]);
      const assetMap = new Map(assets.filter((item): item is AssetRecord => !!item).map((item) => [item.public_id, item]));
      for (const target of assetTargets) {
        const items = target.assetIds.map((id) => assetMap.get(id)).filter((item): item is AssetRecord => !!item);
        restored[target.messageIndex].attachments = items.map((item) => ({ public_id: item.public_id, name: item.name || item.public_id, url: item.url, kind: item.kind }));
        restored[target.messageIndex].images = items.filter((item) => item.kind === "image" || item.mime_type?.startsWith("image/")).map((item) => item.url);
        restored[target.messageIndex].videos = items.filter((item) => item.kind === "video" || item.mime_type?.startsWith("video/")).map((item) => item.url);
        restored[target.messageIndex].audios = items.filter((item) => item.kind === "audio" || item.mime_type?.startsWith("audio/")).map((item) => item.url);
      }
      tasks.forEach((historyTask, index) => {
        if (!historyTask) return;
        const target = taskTargets[index];
        const media = taskMedia(historyTask.output, historyTask.type);
        const references = taskReferences(historyTask.input);
        if (target.userMessageIndex >= 0 && (references.images.length || references.videos.length || references.audios.length)) {
          restored[target.userMessageIndex].images = references.images;
          restored[target.userMessageIndex].videos = references.videos;
          restored[target.userMessageIndex].audios = references.audios;
        }
        restored[target.messageIndex] = {
          role: "assistant",
          content: historyTask.status === "succeeded" ? "生成完成" : historyTask.status === "failed" ? historyTask.error_message || "生成失败" : `生成中 ${historyTask.progress || 0}%`,
          images: media.images,
          videos: media.videos,
          audios: media.audios,
        };
      });
      const latestSucceededMedia = [...tasks].reverse().reduce<MediaSet | null>((found, historyTask) => {
        if (found || !historyTask || historyTask.status !== "succeeded") return found;
        const media = taskMedia(historyTask.output, historyTask.type);
        return media.images.length || media.videos.length || media.audios.length ? media : null;
      }, null);
      setLatestGeneratedMedia(latestSucceededMedia || { images: [], videos: [], audios: [] });
      setMessages(restored);
      const restoredFiles = Array.from(assetMap.values()).map((item) => ({ public_id: item.public_id, url: item.url, name: item.name || item.public_id }));
      setBottom((current) => ({ ...current, asset_ids: [], files: restoredFiles }));
      setConversationId(publicID);
      const latestTask = [...tasks].reverse().find((item): item is TaskState => !!item);
      setTask(latestTask && !["succeeded", "failed", "cancelled"].includes(latestTask.status) ? latestTask : null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "历史记录加载失败");
    } finally {
      setHistoryLoadingId("");
    }
  };

  return (
    <div className="relative isolate flex min-h-0 flex-1 flex-col overflow-hidden bg-[#eaf7fb] text-gray-900 dark:bg-[#05080f] dark:text-white">
      <div className="pointer-events-none absolute inset-0 hidden opacity-80 [background-image:linear-gradient(rgba(15,23,42,.08)_1px,transparent_1px),linear-gradient(90deg,rgba(15,23,42,.08)_1px,transparent_1px)] [background-size:40px_40px] dark:opacity-60 dark:[background-image:linear-gradient(rgba(34,211,238,.08)_1px,transparent_1px),linear-gradient(90deg,rgba(34,211,238,.08)_1px,transparent_1px)] md:block" />
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_70%_10%,rgba(34,211,238,.22),transparent_28%),radial-gradient(circle_at_12%_84%,rgba(20,184,166,.16),transparent_22%)] dark:bg-[radial-gradient(circle_at_76%_10%,rgba(20,184,166,.2),transparent_28%),radial-gradient(circle_at_14%_82%,rgba(6,182,212,.12),transparent_22%)]" />

      {onOpenModelPicker && (
        <div className="relative z-50 flex shrink-0 items-center gap-2 border-b border-white/60 bg-white/70 px-3 py-2 backdrop-blur-xl dark:border-white/10 dark:bg-white/[0.04] lg:hidden">
          <button type="button" onClick={onOpenModelPicker} className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gray-50 text-gray-600 dark:bg-white/10 dark:text-gray-200" aria-label="返回模型列表"><ChevronDown size={16} className="rotate-90" /></button>
          {onOpenNav && <button type="button" onClick={onOpenNav} className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gray-50 text-gray-600 dark:bg-white/10 dark:text-gray-200" aria-label="打开菜单"><Menu size={18} /></button>}
          <div className="min-w-0 flex-1 truncate text-sm font-semibold text-gray-900 dark:text-gray-100">Agent 通用智能体</div>
          <Link href="/app/wallet" className="shrink-0 text-xs font-medium tabular-nums text-primary">{walletBalance?.toFixed(0) ?? "0"}</Link>
        </div>
      )}

      <div className="relative z-50 flex shrink-0 items-center justify-between gap-2 border-b border-white/60 bg-white/35 px-3 py-1.5 backdrop-blur-xl dark:border-white/10 dark:bg-white/[0.02] sm:px-5 sm:py-3">
        <div className="flex min-w-0 items-center gap-2 sm:gap-3">
          <button type="button" onClick={resetSession} disabled={busy} aria-label="新任务" className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-2.5 text-[13px] font-semibold text-dark shadow-sm transition hover:bg-primary/90 disabled:opacity-50 sm:px-3">
            <Plus size={15} /><span className="hidden sm:inline">新任务</span>
          </button>
          <div className="relative">
            <button type="button" onClick={openHistory} disabled={!!historyLoadingId} aria-label="历史" className="flex h-9 items-center gap-1.5 rounded-xl border border-gray-100 bg-white px-2.5 text-[13px] text-gray-600 shadow-sm transition hover:border-gray-200 disabled:opacity-60 dark:border-white/10 dark:bg-white/5 dark:text-gray-300 dark:hover:border-white/20 sm:px-3">
              {historyLoadingId ? <Loader2 size={15} className="animate-spin" /> : <History size={15} />}<span className="hidden sm:inline">{historyLoadingId ? "加载中" : "历史"}</span><ChevronDown size={14} className="hidden sm:block" />
            </button>
            {historyOpen && (
              <div className="soft-card pointer-events-auto fixed left-4 right-4 top-[108px] z-[60] max-h-[60vh] overflow-y-auto p-2 sm:absolute sm:left-0 sm:right-auto sm:top-auto sm:mt-2 sm:w-[320px]">
                {historyItems.length === 0 ? (
                  <div className="py-6 text-center text-xs text-gray-400">暂无历史任务</div>
                ) : historyItems.map((item) => (
                  <button key={item.public_id} type="button" onClick={() => void loadHistory(item.public_id)} disabled={!!historyLoadingId} className="w-full rounded-xl px-3 py-2 text-left transition hover:bg-gray-50 disabled:opacity-50 dark:hover:bg-white/5">
                    <div className="truncate text-sm text-gray-800 dark:text-gray-100">{item.title?.replace(/^(?:Agent 通用智能体|Ageng 通用智能体|Agneg 通用智能体|通用智能体)：/, "") || item.public_id}</div>
                    <div className="mt-0.5 text-[10px] text-gray-400">{historyLoadingId === item.public_id ? "正在打开..." : new Date(item.updated_at).toLocaleString()}</div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
        <WorkbenchTopActions onRecharge={onRecharge} />
      </div>

      <div className={messages.length === 0 ? "relative z-10 flex min-h-0 flex-1 flex-col overflow-hidden px-2 py-1 sm:px-5 sm:py-3 lg:px-8 lg:py-4" : "relative z-10 min-h-0 flex-1 overflow-y-auto px-3 py-3 sm:px-6 sm:py-4"}>
        {messages.length === 0 ? (
          <div className="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col">
            <div className="flex flex-col items-center px-3 pt-5 text-center md:hidden">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary/15 text-primary"><Sparkles size={22} /></div>
              <h1 className="mt-3 text-xl font-bold tracking-tight text-gray-950 dark:text-gray-50">Agent 通用智能体</h1>
              <p className="mt-2 max-w-sm text-xs leading-5 text-gray-600 dark:text-gray-300">说出目标，Agent 会连续完成图片、视频、语音或音乐创作。</p>
              <div className="mt-4 grid w-full max-w-sm grid-cols-2 gap-2">
                {HOT_PROMPTS.map((item) => <button key={item} type="button" onClick={() => setPrompt(item)} className="min-h-10 rounded-xl border border-gray-200 bg-white/70 px-2 py-2 text-xs leading-4 text-gray-600 transition active:scale-[.98] dark:border-white/10 dark:bg-white/5 dark:text-gray-300">{item}</button>)}
              </div>
              <section className="mt-4 flex min-h-[220px] w-full max-w-sm flex-col rounded-2xl border border-cyan-200/70 bg-white/75 p-4 text-left shadow-sm backdrop-blur dark:border-cyan-400/20 dark:bg-white/[0.05]" aria-label="创作能力轮播">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs font-semibold text-cyan-700 dark:text-cyan-200">智能理解与生成</span>
                  <span className="rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-[10px] font-medium text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-200">支持图片、视频与音频</span>
                </div>
                <div className="mt-4 flex items-start gap-3">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary/15 text-primary"><ActiveMobileFeatureIcon size={23} /></div>
                  <div className="min-w-0 flex-1">
                    <h2 className="text-base font-bold text-gray-900 dark:text-gray-100">{activeMobileFeature.title}</h2>
                    <p className="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{activeMobileFeature.subtitle}</p>
                  </div>
                </div>
                <button type="button" onClick={() => setPrompt(HOT_PROMPTS[activeFeature] || HOT_PROMPTS[0])} className="mt-4 flex min-h-10 items-center justify-between gap-3 rounded-xl bg-cyan-500/10 px-3 py-2 text-left text-xs font-medium text-cyan-800 transition active:scale-[.98] dark:text-cyan-100">
                  <span className="shrink-0 text-[10px] text-cyan-600 dark:text-cyan-300">快捷创作</span>
                  <span className="truncate">{HOT_PROMPTS[activeFeature] || HOT_PROMPTS[0]}</span>
                </button>
                <div className="mt-auto flex items-center justify-between pt-4">
                  <div className="flex items-center gap-1.5">
                    {CREATIVE_FEATURES.map((item, index) => <button key={item.title} type="button" onClick={() => setActiveFeature(index)} className={`h-1.5 rounded-full transition-all ${index === activeFeature ? "w-5 bg-primary" : "w-1.5 bg-gray-300 dark:bg-white/20"}`} aria-label={`查看${item.title}`} aria-current={index === activeFeature ? "true" : undefined} />)}
                  </div>
                  <div className="flex items-center gap-1">
                    <button type="button" onClick={() => setActiveFeature((activeFeature + CREATIVE_FEATURES.length - 1) % CREATIVE_FEATURES.length)} className="flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 text-gray-500 transition active:scale-[.96] dark:border-white/10 dark:text-gray-300" aria-label="上一项"><ChevronLeft size={16} /></button>
                    <button type="button" onClick={() => setActiveFeature((activeFeature + 1) % CREATIVE_FEATURES.length)} className="flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 text-gray-500 transition active:scale-[.96] dark:border-white/10 dark:text-gray-300" aria-label="下一项"><ChevronRight size={16} /></button>
                  </div>
                </div>
              </section>
            </div>
            <div className="hidden min-h-0 flex-1 md:flex">
              <AgentLanding
              workflowIcon="✦"
              workflowName="Agent 通用智能体"
              workflowDescription="一句话理解创作目标，连续完成图片、视频、语音与音乐任务；上一轮结果可直接衔接下一轮。"
              heroTags={HOT_PROMPTS}
              features={CREATIVE_FEATURES}
              activeIndex={activeFeature}
              onSelect={setActiveFeature}
              theme={AGENT_THEMES.comic}
              generationType="mixed"
              compactOnMobile
            />
            </div>
          </div>
        ) : (
          <div className="mx-auto w-full max-w-5xl space-y-4 py-2">
            {messages.map((message, index) => (
              <div key={`${index}-${message.role}`} className={`flex ${message.role === "user" ? "justify-end" : "justify-start"}`}>
                <div className={`flex max-w-[96%] items-start gap-2.5 ${message.role === "user" ? "flex-row-reverse" : ""}`}>
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full border border-white/80 bg-white shadow-sm dark:border-white/10 dark:bg-gray-900">
                    {message.role === "assistant" ? (
                      <img src={siteLogo || "/assets/default-app-icon.svg"} alt={siteName || "Agent"} className="h-full w-full object-cover" />
                    ) : user?.avatar_url ? (
                      <img src={user.avatar_url} alt={user.nickname || "用户"} className="h-full w-full object-cover" />
                    ) : (
                      <UserRound size={18} className="text-gray-400" />
                    )}
                  </div>
                  <div className="min-w-0 max-w-[calc(100%_-_46px)] text-sm leading-6">
                    {message.content && !(message.content === "生成完成" && (message.images?.length || message.videos?.length || message.audios?.length)) ? (
                      <div className={`w-fit max-w-full whitespace-pre-wrap rounded-2xl px-4 py-3 shadow-sm ${message.role === "user" ? "ml-auto bg-primary text-dark" : "border border-white/80 bg-white text-gray-700 dark:border-white/10 dark:bg-gray-900 dark:text-gray-200"}`}>{message.content}</div>
                    ) : null}
                    {message.images?.length ? (
                      <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3">
                        {message.images.map((url) => (
                          <button key={url} type="button" onClick={() => setMediaPreview({ url, type: "image" })} className="group relative block w-full max-w-[260px] overflow-hidden rounded-xl text-left ring-1 ring-black/5 transition hover:ring-primary/50 dark:ring-white/10" aria-label="放大查看图片">
                            <img src={url} alt={message.role === "user" ? "参考素材" : "生成结果"} className="aspect-square w-full object-cover" />
                            <span className="absolute right-2 top-2 flex h-7 w-7 items-center justify-center rounded-full bg-black/55 text-white opacity-0 transition group-hover:opacity-100"><Maximize2 size={14} /></span>
                          </button>
                        ))}
                      </div>
                    ) : null}
                    {message.videos?.length ? (
                      <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                        {message.videos.map((url) => (
                          <button key={url} type="button" onClick={() => setMediaPreview({ url, type: "video" })} className="group relative block w-full max-w-[360px] overflow-hidden rounded-xl text-left ring-1 ring-black/5 transition hover:ring-primary/50 dark:ring-white/10" aria-label="放大播放视频">
                            <video src={url} muted preload="metadata" className="aspect-video w-full object-cover" />
                            <span className="absolute right-2 top-2 flex h-7 w-7 items-center justify-center rounded-full bg-black/55 text-white"><Maximize2 size={14} /></span>
                          </button>
                        ))}
                      </div>
                    ) : null}
                    {message.audios?.length ? (
                      <div className="mt-2 space-y-2">
                        {message.audios.map((url) => (
                          <div key={url} className="flex w-full max-w-[440px] items-center gap-2">
                            <audio src={url} controls preload="metadata" className="h-10 min-w-0 flex-1" />
                            <a href={url} download target="_blank" rel="noreferrer" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-black/10 text-gray-500 transition hover:border-primary/40 hover:text-primary dark:border-white/15 dark:text-gray-300" aria-label="下载音频"><Download size={15} /></a>
                          </div>
                        ))}
                      </div>
                    ) : null}
                    {message.attachments?.some((item) => item.kind !== "image" && item.kind !== "video" && item.kind !== "audio") ? (
                      <div className="mt-3 flex flex-wrap gap-1.5">
                        {message.attachments.filter((item) => item.kind !== "image" && item.kind !== "video" && item.kind !== "audio").map((item) => <a key={item.public_id} href={item.url} target="_blank" rel="noreferrer" className="max-w-[220px] truncate rounded-lg border border-black/10 px-2 py-1 text-xs underline-offset-2 hover:underline dark:border-white/10">{item.name}</a>)}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
        {busy && (
          <div className="mx-auto mt-4 flex max-w-5xl items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Loader2 size={14} className="animate-spin text-primary" />
            正在分析或创建任务...
          </div>
        )}
        {task && (
          <div className="soft-card mx-auto mt-4 flex max-w-5xl items-center justify-between gap-3 px-4 py-3 text-xs text-gray-500 dark:text-gray-300">
            <span className="min-w-0 truncate">任务 {task.task_no}</span>
            <span className="shrink-0 font-medium text-gray-700 dark:text-gray-200">{task.status === "succeeded" ? "已完成" : task.status === "failed" ? task.error_message || "生成失败" : `生成中 ${task.progress || 0}%`}</span>
          </div>
        )}
      </div>

      <div className="relative z-10 shrink-0 px-2 pb-2 pt-1 sm:px-6 sm:pb-5 sm:pt-2">
        <div className="mx-auto w-full max-w-[1040px]">
          {error && <p className="mb-2 px-1 text-sm text-red-500">{error}</p>}
          <div className="soft-input overflow-hidden">
            <div className="flex items-center gap-2 border-b border-gray-50 px-3 py-2 dark:border-white/10 sm:px-4">
              <div className="min-w-0 flex-1"><ChatTopTools value={bottom} onChange={(next) => {
                const before = [...bottom.asset_ids, ...bottom.files.map((item) => item.public_id)].sort().join("|");
                const after = [...next.asset_ids, ...next.files.map((item) => item.public_id)].sort().join("|");
                if (before !== after) setLatestGeneratedMedia({ images: [], videos: [], audios: [] });
                setBottom(next);
              }} showUpload={false} showRole={false} /></div>
              <button type="button" onClick={() => setGuideOpen(true)} className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-xl px-2.5 text-xs text-gray-500 transition hover:bg-gray-50 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-white/5 dark:hover:text-white"><HelpCircle size={15} />玩法说明</button>
            </div>
            <div className="flex min-h-[62px] items-center gap-2 px-2 py-2 sm:min-h-[104px] sm:items-stretch sm:gap-3 sm:px-4 sm:py-3">
              <button type="button" onClick={() => uploadRef.current?.click()} disabled={uploading || bottom.files.length >= 10} className="relative flex h-11 w-11 shrink-0 flex-col items-center justify-center rounded-xl border border-dashed border-gray-200 bg-gray-50 text-gray-500 transition hover:border-primary/40 hover:bg-primary/5 hover:text-primary disabled:opacity-50 dark:border-white/10 dark:bg-white/5 dark:text-gray-300 sm:h-auto sm:w-20 sm:gap-1.5 sm:rounded-2xl" aria-label="上传素材">
                {uploading ? <Loader2 size={19} className="animate-spin text-primary" /> : <Upload size={19} />}
                <span className="hidden text-[10px] sm:block">{uploading ? "上传中" : "上传文件"}</span>
                <span className="absolute -bottom-1 -right-1 rounded-full bg-gray-700 px-1 text-[8px] leading-4 text-white dark:bg-gray-200 dark:text-gray-900 sm:static sm:bg-transparent sm:px-0 sm:text-[9px] sm:leading-normal sm:text-gray-400 dark:sm:bg-transparent dark:sm:text-gray-400">{bottom.files.length}/10</span>
              </button>
              <input ref={uploadRef} type="file" multiple accept="image/*,video/*,audio/*,.pdf,.doc,.docx,.txt,.md" className="hidden" onChange={(event) => { void uploadFiles(event.target.files); event.target.value = ""; }} />
              <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); void sendMessage(); } }} placeholder="描述想法，或上传参考素材直接创作。" rows={3} aria-label="创作需求" className="min-h-[48px] min-w-0 flex-1 resize-none bg-transparent px-1 py-1.5 text-sm leading-relaxed text-gray-700 outline-none placeholder:text-gray-400 dark:text-gray-100 dark:placeholder:text-gray-500 sm:min-h-[88px] sm:py-2" />
            </div>
            {bottom.files.length > 0 && (
              <div className="scroll-x-only flex flex-nowrap items-center gap-1.5 px-3 pb-3 sm:px-4">
                {bottom.files.map((file) => (
                  <div key={file.public_id} className="flex max-w-[180px] shrink-0 items-center gap-1.5 rounded-xl border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-gray-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300">
                    <span className="truncate">{file.name}</span>
                    <button type="button" onClick={() => setBottom((current) => ({ ...current, files: current.files.filter((item) => item.public_id !== file.public_id) }))} className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-500 dark:border-white/10 dark:bg-white/10 dark:text-gray-300" aria-label={`移除 ${file.name}`}><X size={10} /></button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center gap-2 border-t border-gray-50 px-2 py-2 dark:border-white/10 sm:px-4 sm:py-3">
              <div className="scroll-x-only flex min-w-0 flex-1 flex-nowrap items-center gap-2 overflow-x-auto pb-1">
                <button type="button" onClick={() => setCustomEnabled(false)} aria-pressed={!customEnabled} className={`h-8 whitespace-nowrap rounded-xl border px-2.5 text-xs transition sm:h-9 sm:px-3 sm:text-sm ${!customEnabled ? "border-primary/30 bg-primary/10 text-primary" : "border-gray-200 bg-gray-50 text-gray-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300"}`}>Agent 模式</button>
                <button type="button" onClick={() => setDeepThink((value) => !value)} aria-pressed={deepThink} className={`h-8 whitespace-nowrap rounded-xl border px-2.5 text-xs transition sm:h-9 sm:px-3 sm:text-sm ${deepThink ? "border-primary/30 bg-primary/10 text-primary" : "border-gray-200 bg-gray-50 text-gray-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300"}`}>深度思考</button>
                <button type="button" onClick={() => setCustomEnabled((value) => !value)} aria-pressed={customEnabled} className={`inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl border px-2.5 text-xs transition sm:h-9 sm:px-3 sm:text-sm ${customEnabled ? "border-primary/30 bg-primary/10 text-primary" : "border-gray-200 bg-gray-50 text-gray-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300"}`}><SlidersHorizontal size={14} />自定义</button>
                {customEnabled && (
                  <>
                    <select value={customMediaType} onChange={(event) => setCustomMediaType(event.target.value as GenerationType)} aria-label="生成类型" className="h-9 rounded-xl border border-gray-200 bg-gray-50 px-3 text-sm text-gray-600 outline-none focus:border-primary [color-scheme:light] dark:border-white/15 dark:bg-gray-900 dark:text-gray-100 dark:[color-scheme:dark]">
                      <option value="image">生成图片</option>
                      <option value="video">生成视频</option>
                      <option value="speech">语音合成</option>
                      <option value="music">歌曲音乐</option>
                    </select>
                    <select value={customModelCode} onChange={(event) => customMediaType === "video" ? setVideoModelCode(event.target.value) : customMediaType === "speech" ? setSpeechModelCode(event.target.value) : customMediaType === "music" ? setMusicModelCode(event.target.value) : setImageModelCode(event.target.value)} aria-label={`${generationLabel(customMediaType)}模型`} className="h-9 max-w-[240px] rounded-xl border border-gray-200 bg-gray-50 px-3 text-sm text-gray-600 outline-none focus:border-primary [color-scheme:light] dark:border-white/15 dark:bg-gray-900 dark:text-gray-100 dark:[color-scheme:dark]">
                      {customModels.map((model) => <option key={model.code} value={model.code}>{model.display_name}</option>)}
                    </select>
                  </>
                )}
                {assetIDs.length > 0 && <span className="whitespace-nowrap text-xs text-gray-400">已选素材 {assetIDs.length}</span>}
              </div>
              <button type="button" onClick={() => void sendMessage()} disabled={busy || !prompt.trim() || !chatModelCode} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-secondary text-white shadow-md transition hover:bg-secondary/90 disabled:opacity-40 sm:h-12 sm:w-12" aria-label="发送">
                {busy ? <Loader2 size={20} className="animate-spin" /> : <ArrowUp size={20} />}
              </button>
            </div>
          </div>
        </div>
      </div>
      {guideOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4" onClick={() => setGuideOpen(false)}>
          <div className="w-full max-w-lg rounded-2xl border border-gray-100 bg-white p-5 shadow-2xl dark:border-white/10 dark:bg-gray-900" onClick={(event) => event.stopPropagation()}>
            <div className="flex items-center justify-between gap-4"><h2 className="font-semibold text-gray-900 dark:text-white">Agent 通用智能体玩法说明</h2><button type="button" onClick={() => setGuideOpen(false)} aria-label="关闭玩法说明" className="flex h-8 w-8 items-center justify-center rounded-xl text-gray-400 hover:bg-gray-100 dark:hover:bg-white/10"><X size={17} /></button></div>
            <div className="mt-4 space-y-3 text-sm leading-6 text-gray-600 dark:text-gray-300">
              <p>直接描述想生成的图片、视频、配音或歌曲音乐，也可以先上传参考文件或从资产库选择素材。</p>
              <p>Agent 模式会自动判断任务类型，并直接调用后台为图片、视频、语音或音乐配置的默认模型。</p>
              <p>连续创作时，上一轮成功生成的媒体会优先作为下一轮参考；重新上传或改选资产后则以新素材为准。</p>
              <p>需要指定模型时开启“自定义”，选择一种生成类型后只会显示该类型的模型。</p>
              <p>开启深度思考后，Agent 会更仔细检查目标、素材和参数，响应时间及模型费用可能增加。</p>
              <p>计费由两部分组成：主聊天模型按实际对话用量计费，生成任务按所选图片、视频或音频模型计费；不额外收取智能体工作流费。</p>
            </div>
          </div>
        </div>
      )}
      {mediaPreview && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm" onClick={() => setMediaPreview(null)}>
          <div className="relative flex max-h-[92vh] w-full max-w-6xl items-center justify-center" onClick={(event) => event.stopPropagation()}>
            {mediaPreview.type === "image" ? (
              <img src={mediaPreview.url} alt="媒体预览" className="max-h-[88vh] max-w-full rounded-xl object-contain" />
            ) : (
              <video src={mediaPreview.url} controls autoPlay className="max-h-[88vh] max-w-full rounded-xl" />
            )}
            <div className="absolute right-2 top-2 flex gap-2">
              <a href={mediaPreview.url} download target="_blank" rel="noreferrer" className="flex h-10 items-center gap-2 rounded-xl bg-black/65 px-3 text-sm text-white transition hover:bg-black/80"><Download size={16} />下载</a>
              <button type="button" onClick={() => setMediaPreview(null)} className="flex h-10 w-10 items-center justify-center rounded-xl bg-black/65 text-white transition hover:bg-black/80" aria-label="关闭预览"><X size={18} /></button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
