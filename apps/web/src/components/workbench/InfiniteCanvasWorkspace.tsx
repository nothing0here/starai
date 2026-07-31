"use client";

import {
  Background,
  BackgroundVariant,
  Handle,
  MarkerType,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  ReactFlowProvider,
  addEdge,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeProps,
  type Viewport,
} from "@xyflow/react";
import {
  AlignCenter,
  Boxes,
  Check,
  ChevronDown,
  CircleHelp,
  Copy,
  Download,
  Eye,
  FileImage,
  FileJson,
  Film,
  FolderOpen,
  Image as ImageIcon,
  Images,
  LoaderCircle,
  Map as MapIcon,
  MessageSquareText,
  Mic,
  MoreHorizontal,
  Play,
  Plus,
  RotateCcw,
  Save,
  Search,
  Sparkles,
  Trash2,
  Type,
  Upload,
  X,
} from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { Model } from "@starai/shared-types";
import {
  buildAudioTaskParams,
  buildVideoTaskParams,
  parseAudioRuntime,
  parseVideoRuntime,
} from "@starai/shared-types";
import { api, apiForLocale, listAssets, uploadAsset } from "@/lib/api";
import { useI18n } from "@/i18n/I18nProvider";
import { SchemaForm, schemaDefaults, schemaProperties } from "./SchemaForm";

type CanvasNodeKind = "textInput" | "imageInput" | "generator" | "compositor";
type GeneratorKind = "text" | "image" | "video" | "audio";
type StoryNarrationMode = "narration" | "first_person" | "third_person" | "character_dialogue" | "smart";
type StorySpeechItem = {
  segment_index: number;
  speaker_code: string;
  speaker_name: string;
  speech_type: "narration" | "dialogue" | "inner_monologue";
  text: string;
  voice_hint?: string;
};
type NewNodeKind =
  | "text"
  | "textGenerator"
  | "imageGenerator"
  | "videoGenerator"
  | "audioGenerator"
  | "compositor";
type CanvasNodeData = Record<string, unknown> & {
  label: string;
  prompt?: string;
  modelCode?: string;
  mediaKind?: GeneratorKind;
  mode?: string;
  assetUrl?: string;
  assetId?: string;
  assetUrls?: string[];
  assetIds?: string[];
  referenceImageUrls?: string[];
  referenceImageIds?: string[];
  referenceVideoUrls?: string[];
  referenceVideoIds?: string[];
  referenceAudioUrls?: string[];
  referenceAudioIds?: string[];
  outputUrl?: string;
  outputUrls?: string[];
  outputText?: string;
  outputKind?: GeneratorKind;
  taskNo?: string;
  taskNos?: string[];
  status?: "idle" | "pending" | "running" | "succeeded" | "failed" | "stale" | "blocked";
  progress?: number;
  progressStage?: string;
  error?: string;
  warning?: string;
  dirty?: boolean;
  lastRunSignature?: string;
  count?: number;
  ratio?: string;
  quality?: string;
  duration?: string;
  seed?: number;
  negativePrompt?: string;
  params?: Record<string, unknown>;
  estimatedCost?: number;
  actualCost?: number;
  composeMode?: string;
  outputSize?: string;
  referenceImageLabel?: string;
  referenceVideoLabel?: string;
  referenceAudioLabel?: string;
  storyGroupID?: string;
  storyRole?: "input" | "script" | "keyframe" | "video" | "narrationText" | "narration" | "final";
  storySegmentIndex?: number;
  storySegmentCount?: number;
  storySegmentDuration?: number;
  storyDurationOptions?: number[];
  storyNarrationMode?: StoryNarrationMode;
  storySpeechPlan?: StorySpeechItem[];
  storyVoiceAssignments?: Record<string, string>;
  storyVoiceOverrides?: Record<string, string>;
  viralGroupID?: string;
  viralRole?: "brief" | "reference" | "brand" | "analysis" | "keyframe" | "video" | "final";
  viralSegmentIndex?: number;
  viralSegmentCount?: number;
  viralSegmentDuration?: number;
  viralDurationOptions?: number[];
};
type CanvasNode = Node<CanvasNodeData, CanvasNodeKind>;
type CanvasEdge = Edge;

type CanvasDocument = {
  version: 1;
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  viewport: Viewport;
};

type CanvasSummary = {
  public_id: string;
  workflow_code?: string;
  title: string;
  created_at: string;
  updated_at: string;
};

type CanvasDetail = CanvasSummary & {
  document: CanvasDocument;
};

type CanvasTemplate = {
  id: string;
  name: string;
  description?: string;
  template_id?: string;
  document?: CanvasDocument;
};

type CanvasWorkflow = {
  display_config?: {
    canvas_templates?: CanvasTemplate[];
  };
  runtime_config?: {
    default_template_id?: string;
    default_segment_count?: number;
    default_segment_duration?: number;
  };
};

type CanvasAsset = {
  public_id: string;
  url: string;
  name?: string;
  kind?: string;
  mime_type?: string;
};

type CanvasResultPreview = {
  url: string;
  kind: Exclude<GeneratorKind, "text">;
  title: string;
};

const LOCAL_CANVAS_STORAGE_KEY = "starai_infinite_canvases_v1";

function readLocalCanvases(): CanvasDetail[] {
  if (typeof window === "undefined") return [];
  try {
    const parsed = JSON.parse(localStorage.getItem(LOCAL_CANVAS_STORAGE_KEY) || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function writeLocalCanvases(items: CanvasDetail[]) {
  localStorage.setItem(LOCAL_CANVAS_STORAGE_KEY, JSON.stringify(items.slice(0, 50)));
}

type TaskResult = {
  task_no: string;
  type?: string;
  status: string;
  progress?: number;
  output?: Record<string, unknown>;
  error_message?: string;
  estimated_cost?: number;
  actual_cost?: number;
};

type NodeActions = {
  chatModels: Model[];
  imageModels: Model[];
  videoModels: Model[];
  audioModels: Model[];
  update: (id: string, patch: Partial<CanvasNodeData>) => void;
  remove: (id: string) => void;
  run: (id: string) => Promise<void>;
  runFrom: (id: string) => Promise<void>;
  upload: (id: string, file: File, append?: boolean) => Promise<void>;
  uploadReference: (id: string, kind: GeneratorKind, file: File) => Promise<void>;
  openAssetLibrary: (id: string, kind: GeneratorKind) => void;
  openOutputMenu: (id: string, point: { x: number; y: number }) => void;
  openResultPreview: (preview: CanvasResultPreview) => void;
  configureStory: (id: string, segmentCount: number, segmentDuration: number, narrationMode?: StoryNarrationMode) => void;
  configureViral: (id: string, segmentCount: number, segmentDuration: number) => void;
};

const CanvasNodeActions = createContext<NodeActions | null>(null);

const NODE_TEMPLATES = [
  { id: "text-image", icon: Sparkles, titleKey: "canvas.template.textImage", descKey: "canvas.template.textImageDesc", tone: "orange" },
  { id: "image-image", icon: ImageIcon, titleKey: "canvas.template.imageImage", descKey: "canvas.template.imageImageDesc", tone: "emerald" },
  { id: "text-image-mix", icon: FileImage, titleKey: "canvas.template.textImageMix", descKey: "canvas.template.textImageMixDesc", tone: "blue" },
  { id: "multi-image", icon: Images, titleKey: "canvas.template.multiImage", descKey: "canvas.template.multiImageDesc", tone: "amber" },
  { id: "text-video", icon: Film, titleKey: "canvas.template.textVideo", descKey: "canvas.template.textVideoDesc", tone: "pink" },
  { id: "image-video", icon: Boxes, titleKey: "canvas.template.imageVideo", descKey: "canvas.template.imageVideoDesc", tone: "violet" },
  { id: "story-short-video", icon: FileImage, titleKey: "canvas.template.storyVideo", descKey: "canvas.template.storyVideoDesc", tone: "blue" },
  { id: "viral-remake", icon: RotateCcw, titleKey: "canvas.template.viralRemake", descKey: "canvas.template.viralRemakeDesc", tone: "orange" },
] as const;

const LIBRARY_TEMPLATES = [
  { id: "ecommerce-visual-pack", icon: Images, titleKey: "canvas.template.ecommercePack", descKey: "canvas.template.ecommercePackDesc", tone: "amber" },
  { id: "social-campaign", icon: Sparkles, titleKey: "canvas.template.socialCampaign", descKey: "canvas.template.socialCampaignDesc", tone: "pink" },
  { id: "product-showcase-video", icon: Film, titleKey: "canvas.template.productShowcase", descKey: "canvas.template.productShowcaseDesc", tone: "orange" },
  { id: "brand-visual-kit", icon: Boxes, titleKey: "canvas.template.brandKit", descKey: "canvas.template.brandKitDesc", tone: "violet" },
  { id: "photo-restoration", icon: ImageIcon, titleKey: "canvas.template.photoRestore", descKey: "canvas.template.photoRestoreDesc", tone: "emerald" },
] as const;

const ALL_TEMPLATE_DEFINITIONS = [...NODE_TEMPLATES, ...LIBRARY_TEMPLATES] as const;

const NEW_NODE_OPTIONS = [
  { kind: "text" as const, icon: Type, key: "canvas.node.addText" },
  { kind: "textGenerator" as const, icon: MessageSquareText, key: "canvas.node.addTextGenerator" },
  { kind: "imageGenerator" as const, icon: Sparkles, key: "canvas.node.addImageGenerator" },
  { kind: "videoGenerator" as const, icon: Play, key: "canvas.node.addVideoGenerator" },
  { kind: "audioGenerator" as const, icon: Mic, key: "canvas.node.addAudioGenerator" },
  { kind: "compositor" as const, icon: Boxes, key: "canvas.node.addCompositor" },
] satisfies Array<{ kind: NewNodeKind; icon: typeof Type; key: string }>;

const OUTPUT_NODE_OPTIONS = NEW_NODE_OPTIONS;

const DEFAULT_TEMPLATE_ZH: Record<string, { name: string; description: string }> = {
  "text-image": { name: "文字生图片", description: "文本提示词连接图片生成节点" },
  "image-image": { name: "图片生图片", description: "文本需求连接带参考图入口的图片生成节点" },
  "text-image-mix": { name: "文案与配图", description: "文本需求先生成文案，再生成配图" },
  "multi-image": { name: "多图对比", description: "同一文本需求并行生成两套图片方案" },
  "text-video": { name: "文字生视频", description: "文本提示词连接视频生成节点" },
  "image-video": { name: "首帧生视频", description: "文本需求连接支持人像形象和首帧素材的视频生成节点" },
  "ecommerce-visual-pack": { name: "电商视觉套图", description: "商品信息与参考图同时生成主图和详情海报" },
  "social-campaign": { name: "社媒图文视频", description: "一份营销文案同时生成社媒配图和短视频" },
  "product-showcase-video": { name: "商品展示视频", description: "商品图先生成关键视觉，再延展为展示视频" },
  "brand-visual-kit": { name: "品牌视觉套件", description: "品牌需求并行生成标志创意和视觉海报" },
  "photo-restoration": { name: "老照片修复", description: "参考照片经过修复、上色与高清增强生成新图" },
  "story-short-video": { name: "故事短视频", description: "故事拆分为多关键帧、多视频片段并合成为完整成片" },
  "viral-remake": { name: "爆款复刻", description: "多模态拆解爆款参考，生成多关键帧、多片段并合成为原创短视频" },
};

const TEMPLATE_TONES: Record<string, string> = {
  orange: "bg-orange-100 text-orange-600 dark:bg-orange-500/15 dark:text-orange-300",
  emerald: "bg-emerald-100 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300",
  blue: "bg-blue-100 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300",
  amber: "bg-amber-100 text-amber-600 dark:bg-amber-500/15 dark:text-amber-300",
  pink: "bg-pink-100 text-pink-600 dark:bg-pink-500/15 dark:text-pink-300",
  violet: "bg-violet-100 text-violet-600 dark:bg-violet-500/15 dark:text-violet-300",
};

const wait = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));
const newNodeID = () => `node_${crypto.randomUUID()}`;

function createsCycle(source: string, target: string, edges: CanvasEdge[]) {
  const pending = [target];
  const visited = new Set<string>();
  while (pending.length) {
    const current = pending.pop();
    if (!current || visited.has(current)) continue;
    if (current === source) return true;
    visited.add(current);
    edges.forEach((edge) => {
      if (edge.source === current) pending.push(edge.target);
    });
  }
  return false;
}

function hasGraphCycle(nodes: CanvasNode[], edges: CanvasEdge[]) {
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const indegree = new Map(nodes.map((node) => [node.id, 0]));
  const outgoing = new Map<string, string[]>();
  edges.forEach((edge) => {
    if (!nodeIDs.has(edge.source) || !nodeIDs.has(edge.target)) return;
    indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1);
    outgoing.set(edge.source, [...(outgoing.get(edge.source) || []), edge.target]);
  });
  const queue = nodes.filter((node) => (indegree.get(node.id) || 0) === 0).map((node) => node.id);
  let visited = 0;
  while (queue.length) {
    const id = queue.shift();
    if (!id) continue;
    visited += 1;
    (outgoing.get(id) || []).forEach((target) => {
      const next = (indegree.get(target) || 0) - 1;
      indegree.set(target, next);
      if (next === 0) queue.push(target);
    });
  }
  return visited !== nodes.length;
}

function orderedGeneratorNodes(nodes: CanvasNode[], edges: CanvasEdge[]) {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const indegree = new Map(nodes.map((node) => [node.id, 0]));
  edges.forEach((edge) => {
    if (byID.has(edge.source) && byID.has(edge.target)) {
      indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1);
    }
  });
  const queue = nodes.filter((node) => (indegree.get(node.id) || 0) === 0).map((node) => node.id);
  const ordered: CanvasNode[] = [];
  while (queue.length) {
    const id = queue.shift();
    if (!id) continue;
    const node = byID.get(id);
    if (node) ordered.push(node);
    edges.forEach((edge) => {
      if (edge.source !== id) return;
      const next = (indegree.get(edge.target) || 0) - 1;
      indegree.set(edge.target, next);
      if (next === 0) queue.push(edge.target);
    });
  }
  return (ordered.length === nodes.length ? ordered : nodes).filter((node) => node.type === "generator" || node.type === "compositor");
}

function collectUpstreamNodes(targetID: string, nodes: CanvasNode[], edges: CanvasEdge[]) {
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const visited = new Set<string>([targetID]);
  const pending = edges.filter((edge) => edge.target === targetID).map((edge) => edge.source);
  const upstream: CanvasNode[] = [];
  while (pending.length) {
    const id = pending.shift();
    if (!id || visited.has(id)) continue;
    visited.add(id);
    const node = byID.get(id);
    if (node) upstream.push(node);
    edges.forEach((edge) => {
      if (edge.target === id && !visited.has(edge.source)) pending.push(edge.source);
    });
  }
  return upstream;
}

function collectDownstreamIDs(sourceID: string, edges: CanvasEdge[]) {
  const visited = new Set<string>();
  const pending = edges.filter((edge) => edge.source === sourceID).map((edge) => edge.target);
  while (pending.length) {
    const id = pending.shift();
    if (!id || visited.has(id)) continue;
    visited.add(id);
    edges.forEach((edge) => {
      if (edge.source === id && !visited.has(edge.target)) pending.push(edge.target);
    });
  }
  return visited;
}

function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => [key, stableValue(item)])
  );
}

function compactSignature(value: string) {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `v1:${value.length}:${(hash >>> 0).toString(16)}`;
}

function nodeRunSignature(nodeID: string, nodes: CanvasNode[], edges: CanvasEdge[]) {
  const node = nodes.find((item) => item.id === nodeID);
  if (!node) return "";
  const runtimeKeys = new Set([
    "label",
    "status",
    "error",
    "dirty",
    "lastRunSignature",
    "outputUrl",
    "outputUrls",
    "outputText",
    "outputKind",
    "taskNo",
    "taskNos",
    "warning",
    "storySpeechPlan",
    "storyVoiceAssignments",
    "estimatedCost",
    "actualCost",
  ]);
  const configuration = Object.fromEntries(
    Object.entries(node.data).filter(([key]) => !runtimeKeys.has(key))
  );
  const directInputs = edges
    .filter((edge) => edge.target === nodeID)
    .map((edge) => nodes.find((item) => item.id === edge.source))
    .filter((item): item is CanvasNode => Boolean(item))
    .map((item) => ({
      id: item.id,
      outputUrl: item.data.outputUrl || "",
      outputUrls: item.data.outputUrls || [],
      outputText: item.data.outputText || "",
      taskNo: item.data.taskNo || "",
      taskNos: item.data.taskNos || [],
      prompt: item.type === "textInput" ? item.data.prompt || "" : "",
      assetUrls: item.type === "imageInput" ? item.data.assetUrls || [] : [],
      referenceImageUrls: item.data.referenceImageUrls || [],
      referenceVideoUrls: item.data.referenceVideoUrls || [],
      referenceAudioUrls: item.data.referenceAudioUrls || [],
    }));
  return compactSignature(JSON.stringify(stableValue({ configuration, directInputs })));
}

function normalizeCanvasNodes(nodes: CanvasNode[]) {
  return nodes.map((node) => {
    if (node.type !== "generator" && node.type !== "compositor") return node;
    const interrupted = node.data.status === "pending" || node.data.status === "running";
    const resultMissing = node.data.status === "succeeded"
      && !(node.data.mediaKind === "text" ? node.data.outputText : node.data.outputUrl);
    return {
      ...node,
      data: {
        ...node.data,
        status: interrupted || resultMissing ? "idle" : node.data.status || "idle",
        dirty: interrupted || resultMissing || !node.data.lastRunSignature
          ? true
          : Boolean(node.data.dirty),
        error: interrupted ? "" : node.data.error,
      },
    };
  });
}

function validateCompositorNode(node: CanvasNode, nodes: CanvasNode[], edges: CanvasEdge[]) {
  const directSources = edges
    .filter((edge) => edge.target === node.id)
    .map((edge) => nodes.find((item) => item.id === edge.source))
    .filter((item): item is CanvasNode => Boolean(item));
  if (directSources.length === 0) return "canvas.compositor.noSources";
  const kinds = directSources
    .map((source) => source.data.outputKind || source.data.mediaKind)
    .filter((kind): kind is GeneratorKind => kind === "image" || kind === "video" || kind === "audio");
  if (kinds.length !== directSources.length) return "";
  const counts = {
    image: kinds.filter((kind) => kind === "image").length,
    video: kinds.filter((kind) => kind === "video").length,
    audio: kinds.filter((kind) => kind === "audio").length,
  };
  const mode = String(node.data.composeMode || "auto");
  if (mode === "concat") {
    const usedKinds = Object.values(counts).filter((count) => count > 0).length;
    if (usedKinds !== 1 || kinds.length < 2) return "canvas.compositor.concatInvalid";
  } else if (mode === "mux") {
    if (counts.video === 0 || counts.audio !== 1 || counts.image > 0) return "canvas.compositor.muxInvalid";
  } else if (counts.image > 0 && (counts.video > 0 || counts.audio > 0)) {
    return "canvas.compositor.autoMixedInvalid";
  }
  return "";
}

function collectURLs(value: unknown): string[] {
  if (!value) return [];
  if (typeof value === "string") return value.trim() ? [value.trim()] : [];
  if (Array.isArray(value)) return value.flatMap(collectURLs);
  if (typeof value === "object") {
    const item = value as Record<string, unknown>;
    return [
      ...collectURLs(item.url),
      ...collectURLs(item.image_url),
      ...collectURLs(item.video_url),
      ...collectURLs(item.audio_url),
      ...collectURLs(item.output_url),
      ...collectURLs(item.result_url),
      ...collectURLs(item.b64_json),
    ];
  }
  return [];
}

function extractMedia(output: Record<string, unknown> | undefined, kind: GeneratorKind) {
  if (!output) return "";
  const keys =
    kind === "video"
      ? ["video_url", "videos", "results", "data"]
      : kind === "audio"
        ? ["audio_url", "audios", "url", "result_url", "results", "data"]
      : ["image_url", "images", "urls", "results", "data", "b64_json"];
  const found = keys.flatMap((key) => collectURLs(output[key]))[0] || "";
  if (kind === "image" && found && !/^(https?:|data:image\/|blob:)/i.test(found) && found.length > 100) {
    return `data:image/png;base64,${found.replace(/\s+/g, "")}`;
  }
  return found;
}

function modelsForKind(kind: GeneratorKind, actions: Pick<NodeActions, "chatModels" | "imageModels" | "videoModels" | "audioModels"> | null) {
  if (kind === "text") return actions?.chatModels || [];
  if (kind === "video") return actions?.videoModels || [];
  if (kind === "audio") return actions?.audioModels || [];
  return actions?.imageModels || [];
}

function isMultiCollabModel(model: Model) {
  return model.category === "multi_collab" || model.code === "multi_collab_chat";
}

function preferredVideoModel(models: Model[]) {
  const seedanceModels = models.filter((model) => parseVideoRuntime(model.runtime_rule).upload_profile === "seedance_2");
  return seedanceModels.find((model) => /(?:doubao[\s_-]*)?(?:seedance|sd)[\s_-]*2(?:\.0)?/i.test(`${model.code} ${model.display_name}`))
    || seedanceModels[0]
    || models.find((model) => /(?:doubao[\s_-]*)?(?:seedance|sd)[\s_-]*2(?:\.0)?/i.test(`${model.code} ${model.display_name}`))
    || models[0];
}

type SeedanceMaterialMode =
  | "text"
  | "image"
  | "video"
  | "image_audio"
  | "image_video"
  | "video_audio"
  | "image_video_audio";

function inferSeedanceMaterialMode(imageCount: number, videoCount: number, audioCount: number): SeedanceMaterialMode {
  const hasImage = imageCount > 0;
  const hasVideo = videoCount > 0;
  const hasAudio = audioCount > 0;
  if (hasImage && hasVideo && hasAudio) return "image_video_audio";
  if (hasImage && hasVideo) return "image_video";
  if (hasImage && hasAudio) return "image_audio";
  if (hasVideo && hasAudio) return "video_audio";
  if (hasImage) return "image";
  if (hasVideo) return "video";
  return "text";
}

function omitSchemaField(schema: Model["input_schema"], field: string) {
  const source = (schema || {}) as Record<string, unknown>;
  const properties = { ...((source.properties as Record<string, unknown> | undefined) || {}) };
  delete properties[field];
  const required = Array.isArray(source.required)
    ? source.required.filter((item) => item !== field)
    : source.required;
  return { ...source, properties, ...(required ? { required } : {}) };
}

function canvasInputSchema(kind: GeneratorKind, schema: Model["input_schema"]) {
  if (kind !== "audio") return schema;
  const source = (schema || {}) as Record<string, unknown>;
  const properties = { ...((source.properties as Record<string, unknown> | undefined) || {}) };
  delete properties.count;
  delete properties.n;
  return { ...source, properties };
}

function canvasModelDefaults(kind: GeneratorKind, model?: Model) {
  if (!model) return {};
  const schemaValues = schemaDefaults(canvasInputSchema(kind, model.input_schema));
  const defaults = kind === "video"
    ? { ...(model.default_params || {}), ...schemaValues }
    : { ...schemaValues, ...(model.default_params || {}) };
  if (kind === "audio") {
    delete defaults.count;
    delete defaults.n;
  }
  return defaults;
}

function kindIcon(kind: GeneratorKind) {
  if (kind === "text") return <MessageSquareText size={16} />;
  if (kind === "video") return <Film size={16} />;
  if (kind === "audio") return <Mic size={16} />;
  return <Sparkles size={16} />;
}

function runningProgress(taskProgress: unknown, attempt: number) {
  const reported = Number(taskProgress || 0);
  const staged = Math.round(18 + 76 * (1 - Math.exp(-Math.max(0, attempt) / 28)));
  return Math.min(94, Math.max(12, Number.isFinite(reported) ? reported : 0, staged));
}

async function copyCanvasText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand("copy");
  textarea.remove();
}

async function downloadCanvasResult(url: string, filename: string) {
  try {
    const response = await fetch(url);
    if (!response.ok) throw new Error("download failed");
    const blob = await response.blob();
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
  } catch {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

function numericDuration(value: unknown) {
  const match = String(value ?? "").trim().match(/^(-?\d+(?:\.\d+)?)\s*(?:s|秒)?$/i);
  if (!match) return null;
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) ? parsed : null;
}

const STORY_SEGMENT_COUNT_OPTIONS = [3, 4, 6, 8] as const;
const VIRAL_SEGMENT_COUNT_OPTIONS = [3, 4, 6] as const;
const STORY_NARRATION_MODES: StoryNarrationMode[] = ["smart", "narration", "first_person", "third_person", "character_dialogue"];

function normalizeStoryNarrationMode(value: unknown): StoryNarrationMode {
  const mode = String(value || "").trim() as StoryNarrationMode;
  return STORY_NARRATION_MODES.includes(mode) ? mode : "smart";
}

function extractJSONValue(text: string): unknown {
  const trimmed = text.trim().replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/i, "");
  const arrayStart = trimmed.indexOf("[");
  const arrayEnd = trimmed.lastIndexOf("]");
  const objectStart = trimmed.indexOf("{");
  const objectEnd = trimmed.lastIndexOf("}");
  const candidate = arrayStart >= 0 && arrayEnd > arrayStart
    ? trimmed.slice(arrayStart, arrayEnd + 1)
    : objectStart >= 0 && objectEnd > objectStart
      ? trimmed.slice(objectStart, objectEnd + 1)
      : trimmed;
  try {
    return JSON.parse(candidate);
  } catch {
    return null;
  }
}

function parseStorySpeechPlan(text: string): { items: StorySpeechItem[]; fallback: boolean } {
  const parsed = extractJSONValue(text);
  const rawItems = Array.isArray(parsed)
    ? parsed
    : parsed && typeof parsed === "object" && Array.isArray((parsed as Record<string, unknown>).speeches)
      ? (parsed as Record<string, unknown>).speeches as unknown[]
      : [];
  const items = rawItems.flatMap((raw, index) => {
    if (!raw || typeof raw !== "object") return [];
    const item = raw as Record<string, unknown>;
    const speechText = String(item.text || item.content || "").trim();
    if (!speechText) return [];
    const speechTypeValue = String(item.speech_type || item.type || "narration");
    const speechType: StorySpeechItem["speech_type"] = speechTypeValue === "dialogue"
      ? "dialogue"
      : speechTypeValue === "inner_monologue"
        ? "inner_monologue"
        : "narration";
    return [{
      segment_index: Math.max(1, Number(item.segment_index || item.segment || index + 1) || index + 1),
      speaker_code: String(item.speaker_code || (speechType === "narration" ? "NARRATOR" : `CHAR_${index + 1}`)).trim(),
      speaker_name: String(item.speaker_name || item.speaker || (speechType === "narration" ? "旁白" : `角色${index + 1}`)).trim(),
      speech_type: speechType,
      text: speechText,
      voice_hint: String(item.voice_hint || "").trim() || undefined,
    }];
  });
  if (items.length > 0) return { items, fallback: false };
  const fallbackText = text.trim();
  return {
    items: fallbackText ? [{ segment_index: 1, speaker_code: "NARRATOR", speaker_name: "旁白", speech_type: "narration", text: fallbackText }] : [],
    fallback: Boolean(fallbackText),
  };
}

function storyVoiceConfiguration(model: Model | undefined, params: Record<string, unknown>) {
  const properties = schemaProperties(canvasInputSchema("audio", model?.input_schema || {}));
  const preferredKeys = ["voice_id", "voice", "speaker_id", "speaker", "timber", "voice_name"];
  const key = preferredKeys.find((candidate) => properties[candidate]) || preferredKeys.find((candidate) => params[candidate] !== undefined) || "";
  const property = key ? properties[key] as Record<string, unknown> | undefined : undefined;
  const options = Array.isArray(property?.enum) ? property.enum.map(String).filter(Boolean) : [];
  const configured = key && params[key] !== undefined ? String(params[key]) : "";
  return { key, options, configured };
}

function assignStoryVoices(items: StorySpeechItem[], model: Model | undefined, params: Record<string, unknown>, overrides: Record<string, string> = {}) {
  const config = storyVoiceConfiguration(model, params);
  const assignments: Record<string, string> = {};
  const speakers = Array.from(new Set(items.map((item) => item.speaker_code || "NARRATOR")));
  speakers.forEach((speaker, index) => {
    if (overrides[speaker]) assignments[speaker] = overrides[speaker];
    else if (speaker === "NARRATOR" && config.configured) assignments[speaker] = config.configured;
    else if (config.options.length > 0) assignments[speaker] = config.options[index % config.options.length];
    else if (config.configured) assignments[speaker] = config.configured;
  });
  const distinct = new Set(Object.values(assignments).filter(Boolean));
  return {
    ...config,
    assignments,
    degraded: speakers.length > 1 && distinct.size < Math.min(2, speakers.length),
  };
}

function storyDurationOptions(model?: Model) {
  const durationProperty = schemaProperties(canvasInputSchema("video", model?.input_schema || {})).duration as
    | Record<string, unknown>
    | undefined;
  const candidates = [
    ...(Array.isArray(durationProperty?.enum) ? durationProperty.enum : []),
    model?.default_params?.duration,
    durationProperty?.default,
  ];
  const values = candidates
    .map(numericDuration)
    .filter((value): value is number => value !== null && value >= 2 && value <= 30);
  return Array.from(new Set(values)).sort((left, right) => left - right);
}

function preferredStoryDuration(model?: Model) {
  const options = storyDurationOptions(model);
  const configured = numericDuration(model?.default_params?.duration);
  if (configured && options.includes(configured)) return configured;
  if (options.includes(8)) return 8;
  return options[0] || configured || 8;
}

function preferredNarrationAudioModel(models: Model[]) {
  const speechHint = /(speech|tts|voice|语音|配音|朗读|音声|음성)/i;
  return models.find((model) =>
    parseAudioRuntime(model.runtime_rule).input_layout !== "dual"
    && speechHint.test(`${model.code} ${model.display_name || ""}`)
  ) || models.find((model) => parseAudioRuntime(model.runtime_rule).input_layout !== "dual") || models[0];
}

function preferredMultimodalChatModel(models: Model[]) {
  const explicit = models.find((model) => {
    const capabilities = (model.runtime_rule?.capabilities || {}) as Record<string, unknown>;
    return capabilities.vision === true || capabilities.multimodal === true || capabilities.image_input === true || capabilities.video_input === true;
  });
  const modelHint = /(vision|multimodal|(?:^|[-_\s])vl(?:[-_\s]|$)|gpt[-_\s]?(?:4o|5)|gemini|claude)/i;
  return explicit || models.find((model) => modelHint.test(`${model.code} ${model.display_name}`)) || models[0];
}

function aspectRatioParams(model: Model | undefined, params: Record<string, unknown>, ratio = "9:16") {
  if (!model) return params;
  const properties = schemaProperties(canvasInputSchema(model.category as GeneratorKind, model.input_schema || {}));
  const next = { ...params };
  for (const key of ["aspect_ratio", "ratio"]) {
    const property = properties[key] as Record<string, unknown> | undefined;
    if (!property) continue;
    const options = Array.isArray(property.enum) ? property.enum : [];
    const match = options.find((item) => String(item).replace(/\s/g, "") === ratio);
    if (match !== undefined || options.length === 0) next[key] = match ?? ratio;
  }
  const orientation = properties.orientation as Record<string, unknown> | undefined;
  if (orientation) {
    const options = Array.isArray(orientation.enum) ? orientation.enum : [];
    const portrait = options.find((item) => /^(portrait|vertical|9:16)$/i.test(String(item)));
    if (portrait !== undefined) next.orientation = portrait;
  }
  return normalizeCanvasParamsForModel(next, model.input_schema, model.default_params);
}

function storyNodeNeedsReset(node: CanvasNode, patch: Partial<CanvasNodeData>): CanvasNode {
  const executable = node.type === "generator" || node.type === "compositor";
  return {
    ...node,
    data: {
      ...node.data,
      ...patch,
      ...(executable
        ? {
            status: node.data.status === "succeeded" ? "stale" : "idle",
            dirty: true,
            progress: 0,
            progressStage: "",
            error: "",
            outputUrl: "",
            outputUrls: [],
            outputText: "",
            outputKind: undefined,
            taskNo: "",
            taskNos: [],
            warning: "",
            storySpeechPlan: [],
            storyVoiceAssignments: {},
            estimatedCost: 0,
            actualCost: 0,
            lastRunSignature: "",
          }
        : {}),
    },
  };
}

function normalizeCanvasParamsForModel(
  params: Record<string, unknown>,
  inputSchema?: Model["input_schema"],
  defaultParams?: Record<string, unknown>
) {
  const normalized = { ...params };
  const properties = schemaProperties(inputSchema || {});
  for (const [key, rawProperty] of Object.entries(properties)) {
    const property = (rawProperty || {}) as Record<string, unknown>;
    const enumValues = Array.isArray(property.enum) ? property.enum : [];
    if (!enumValues.length || normalized[key] === undefined) continue;
    const current = normalized[key];
    const exact = enumValues.find((item) => Object.is(item, current));
    if (exact !== undefined) {
      normalized[key] = exact;
      continue;
    }
    // Video providers differ on duration types: Veo uses "4s", while
    // Seedance uses numeric seconds. Match their semantic duration but retain
    // the exact enum value and type declared by the selected model.
    const currentDuration = key === "duration" ? numericDuration(current) : null;
    const equivalent = enumValues.find((item) => {
      if (String(item) === String(current)) return true;
      return key === "duration" && currentDuration !== null && numericDuration(item) === currentDuration;
    });
    if (equivalent !== undefined) {
      normalized[key] = equivalent;
      continue;
    }
    const fallback = property.default ?? defaultParams?.[key] ?? enumValues[0];
    const validFallback = enumValues.find((item) => Object.is(item, fallback) || String(item) === String(fallback));
    normalized[key] = validFallback ?? enumValues[0];
  }
  return normalized;
}

function truncateCanvasTitle(value: string, maxLength = 48) {
  const normalized = value.replace(/\s+/g, " ").trim();
  const characters = Array.from(normalized);
  return characters.length > maxLength ? `${characters.slice(0, Math.max(1, maxLength - 1)).join("")}…` : normalized;
}

function automaticCanvasTitle(nodes: CanvasNode[], workflowName: string) {
  const prompt = nodes
    .filter((node) => node.type === "textInput")
    .map((node) => String(node.data.prompt || "").replace(/\s+/g, " ").trim())
    .find(Boolean);
  const flow = workflowName.replace(/\s+/g, " ").trim();
  if (!prompt) return truncateCanvasTitle(flow);
  const suffix = flow ? ` · ${flow}` : "";
  const maxPromptLength = Math.max(12, 48 - Array.from(suffix).length);
  return truncateCanvasTitle(`${truncateCanvasTitle(prompt, maxPromptLength)}${suffix}`);
}

function NodeFrame({
  id,
  title,
  icon,
  children,
  source = true,
  target = true,
  status,
  headerActions,
  className = "w-[272px]",
  selected = false,
  runnable = false,
  progress = 0,
  progressLabel,
}: {
  id: string;
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
  source?: boolean;
  target?: boolean;
  status?: CanvasNodeData["status"];
  headerActions?: React.ReactNode;
  className?: string;
  selected?: boolean;
  runnable?: boolean;
  progress?: number;
  progressLabel?: string;
}) {
  const actions = useContext(CanvasNodeActions);
  const { t } = useI18n();
  const [actionMenuOpen, setActionMenuOpen] = useState(false);
  const running = status === "pending" || status === "running";
  const safeProgress = Math.max(0, Math.min(100, Math.round(Number(progress || 0))));
  return (
    <div className={`${className} relative overflow-visible`}>
      {target && (
        <Handle
          type="target"
          position={Position.Left}
          aria-label={t("canvas.node.connectInput")}
          title={t("canvas.node.connectInput")}
          className={`!z-10 !flex !h-6 !w-6 !items-center !justify-center !border-2 !border-white !bg-cyan-500 !text-white !shadow-[0_0_9px_rgba(6,182,212,0.34)] !transition-opacity dark:!border-gray-900 ${selected ? "!opacity-100" : "!opacity-0"}`}
        >
          <Plus size={12} />
        </Handle>
      )}
      <div className={`overflow-hidden rounded-xl border bg-white/95 backdrop-blur transition-[border-color,box-shadow] dark:bg-gray-900/95 ${
        selected
          ? "border-cyan-400 shadow-[0_0_0_1px_rgba(34,211,238,0.22),0_12px_34px_rgba(15,23,42,0.15)] dark:border-cyan-400/80 dark:shadow-[0_0_0_1px_rgba(34,211,238,0.18),0_16px_40px_rgba(0,0,0,0.36)]"
          : "border-gray-200 shadow-[0_10px_30px_rgba(15,23,42,0.11)] dark:border-white/10 dark:shadow-[0_16px_40px_rgba(0,0,0,0.32)]"
      }`}>
        <div className="flex items-center gap-2 border-b border-gray-100 px-2.5 py-2 dark:border-white/10">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300">
            {icon}
          </span>
          <span className="min-w-0 flex-1 truncate text-xs font-semibold text-gray-900 dark:text-gray-100">{title}</span>
          {headerActions}
          {status && status !== "idle" && (
            <span className={`text-[10px] ${status === "failed" || status === "blocked" ? "text-red-500" : status === "succeeded" ? "text-emerald-500" : status === "stale" ? "text-amber-500" : "text-cyan-500"}`}>
              {status === "failed"
                ? t("canvas.status.failed")
                : status === "blocked"
                  ? t("canvas.status.blocked")
                  : status === "succeeded"
                    ? t("canvas.status.completed")
                    : status === "stale"
                      ? t("canvas.status.stale")
                      : t("canvas.status.running")}
            </span>
          )}
          {runnable && (
            <div className="nodrag relative">
              <button
                type="button"
                title={t("canvas.node.moreActions")}
                aria-label={t("canvas.node.moreActions")}
                aria-expanded={actionMenuOpen}
                onClick={(event) => {
                  event.stopPropagation();
                  setActionMenuOpen((value) => !value);
                }}
                className="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-cyan-300 hover:text-cyan-500 dark:border-white/10"
              >
                <MoreHorizontal size={14} />
              </button>
              {actionMenuOpen && (
                <div className="absolute right-0 top-8 z-50 w-40 rounded-xl border border-gray-200 bg-white p-1.5 shadow-xl dark:border-white/10 dark:bg-gray-900">
                  <button
                    type="button"
                    onClick={() => {
                      setActionMenuOpen(false);
                      void actions?.run(id);
                    }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[11px] text-gray-600 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5"
                  >
                    <Play size={13} /> {status === "failed" ? t("canvas.node.retry") : t("canvas.node.runOnly")}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setActionMenuOpen(false);
                      void actions?.runFrom(id);
                    }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[11px] text-gray-600 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5"
                  >
                    <Boxes size={13} /> {t("canvas.node.runFromHere")}
                  </button>
                </div>
              )}
            </div>
          )}
          <button type="button" onClick={() => actions?.remove(id)} className="nodrag rounded-lg p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10">
            <X size={14} />
          </button>
        </div>
        {running && (
          <div className="border-b border-gray-100 bg-cyan-50/60 px-2.5 py-2 dark:border-white/10 dark:bg-cyan-500/[0.045]">
            <div className="mb-1.5 flex items-center justify-between gap-2 text-[9px]">
              <span className="flex min-w-0 items-center gap-1.5 font-medium text-cyan-700 dark:text-cyan-300">
                <LoaderCircle size={11} className="shrink-0 animate-spin" />
                <span className="truncate">{progressLabel || t("canvas.progress.generating")}</span>
              </span>
              <span className="shrink-0 tabular-nums text-cyan-600 dark:text-cyan-300">{safeProgress}%</span>
            </div>
            <div
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={safeProgress}
              className="h-1.5 overflow-hidden rounded-full bg-cyan-100 dark:bg-white/10"
            >
              <div
                className="h-full rounded-full bg-gradient-to-r from-cyan-500 via-sky-500 to-violet-500 shadow-[0_0_8px_rgba(6,182,212,0.35)] transition-[width] duration-500 ease-out"
                style={{ width: `${safeProgress}%` }}
              />
            </div>
          </div>
        )}
        {children}
      </div>
      {source && (
        <Handle
          type="source"
          position={Position.Right}
          aria-label={t("canvas.node.addNext")}
          title={t("canvas.node.addNext")}
          onClick={(event) => {
            event.stopPropagation();
            actions?.openOutputMenu(id, { x: event.clientX, y: event.clientY });
          }}
          className={`nodrag !z-10 !flex !h-6 !w-6 !items-center !justify-center !border-2 !border-white !bg-cyan-500 !text-white !shadow-[0_0_10px_rgba(6,182,212,0.42)] !transition-opacity hover:!bg-cyan-600 dark:!border-gray-900 ${selected ? "!opacity-100" : "!opacity-0"}`}
        >
          <Plus size={12} />
        </Handle>
      )}
    </div>
  );
}

function TextInputNode({ id, data, selected }: NodeProps<CanvasNode>) {
  const actions = useContext(CanvasNodeActions);
  const { t } = useI18n();
  const imageURLs = Array.isArray(data.referenceImageUrls) ? data.referenceImageUrls : [];
  const videoURLs = Array.isArray(data.referenceVideoUrls) ? data.referenceVideoUrls : [];
  const audioURLs = Array.isArray(data.referenceAudioUrls) ? data.referenceAudioUrls : [];
  return (
    <NodeFrame
      id={id}
      selected={selected}
      title={data.label || t("canvas.node.textInput")}
      icon={<Plus size={15} />}
      className="w-[320px]"
      headerActions={(
        <div className="nodrag flex items-center gap-1">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg border border-cyan-300 bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300"><Type size={14} /></span>
          <button type="button" title={t("canvas.node.referenceImages")} onClick={() => actions?.openAssetLibrary(id, "image")} className="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-violet-300 hover:text-violet-500 dark:border-white/10"><ImageIcon size={14} /></button>
          <button type="button" title={t("canvas.node.referenceVideos")} onClick={() => actions?.openAssetLibrary(id, "video")} className="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-pink-300 hover:text-pink-500 dark:border-white/10"><Film size={14} /></button>
          <button type="button" title={t("canvas.node.referenceAudio")} onClick={() => actions?.openAssetLibrary(id, "audio")} className="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-amber-300 hover:text-amber-500 dark:border-white/10"><Mic size={14} /></button>
          <span className="flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 dark:border-white/10"><MoreHorizontal size={14} /></span>
        </div>
      )}
    >
      <div className="flex min-h-[210px] flex-col gap-2 p-2.5">
        <textarea
          className="nodrag nowheel h-24 w-full resize-none rounded-lg border border-gray-100 bg-gray-50 p-2.5 text-[11px] leading-relaxed outline-none transition focus:border-cyan-300 dark:border-white/10 dark:bg-white/5 dark:text-gray-100"
          placeholder={t("canvas.node.textPlaceholder")}
          value={data.prompt || ""}
          onChange={(event) => actions?.update(id, { prompt: event.target.value })}
        />
        {data.storyRole === "input" && (
          <div className="nodrag grid grid-cols-2 gap-2 rounded-xl border border-blue-200/70 bg-blue-50/70 p-2 dark:border-blue-400/15 dark:bg-blue-500/[0.06]">
            <label className="min-w-0 text-[9px] text-gray-500 dark:text-gray-300">
              <span className="mb-1 block">{t("canvas.story.segmentCount")}</span>
              <select
                value={Number(data.storySegmentCount || 4)}
                onChange={(event) => actions?.configureStory(
                  id,
                  Number(event.target.value),
                  Number(data.storySegmentDuration || 8),
                  normalizeStoryNarrationMode(data.storyNarrationMode)
                )}
                className="h-8 w-full rounded-lg border border-blue-200 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none dark:border-blue-400/20 dark:bg-gray-900 dark:text-gray-100"
              >
                {STORY_SEGMENT_COUNT_OPTIONS.map((count) => (
                  <option key={count} value={count}>{t("canvas.story.segmentCountValue", { count })}</option>
                ))}
              </select>
            </label>
            <label className="min-w-0 text-[9px] text-gray-500 dark:text-gray-300">
              <span className="mb-1 block">{t("canvas.story.segmentDuration")}</span>
              <select
                value={Number(data.storySegmentDuration || 8)}
                onChange={(event) => actions?.configureStory(
                  id,
                  Number(data.storySegmentCount || 4),
                  Number(event.target.value),
                  normalizeStoryNarrationMode(data.storyNarrationMode)
                )}
                className="h-8 w-full rounded-lg border border-blue-200 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none dark:border-blue-400/20 dark:bg-gray-900 dark:text-gray-100"
              >
                {(Array.isArray(data.storyDurationOptions) && data.storyDurationOptions.length
                  ? data.storyDurationOptions
                  : [Number(data.storySegmentDuration || 8)]
                ).map((duration) => (
                  <option key={duration} value={duration}>{t("canvas.story.secondsValue", { seconds: duration })}</option>
                ))}
              </select>
            </label>
            <label className="col-span-2 min-w-0 text-[9px] text-gray-500 dark:text-gray-300">
              <span className="mb-1 block">{t("canvas.story.narrationMode")}</span>
              <select
                value={normalizeStoryNarrationMode(data.storyNarrationMode)}
                onChange={(event) => actions?.configureStory(
                  id,
                  Number(data.storySegmentCount || 4),
                  Number(data.storySegmentDuration || 8),
                  normalizeStoryNarrationMode(event.target.value)
                )}
                className="h-8 w-full rounded-lg border border-blue-200 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none dark:border-blue-400/20 dark:bg-gray-900 dark:text-gray-100"
              >
                {STORY_NARRATION_MODES.map((mode) => <option key={mode} value={mode}>{t(`canvas.story.narrationMode.${mode}`)}</option>)}
              </select>
            </label>
            <div className="col-span-2 flex items-center justify-between gap-2 text-[9px] text-blue-600 dark:text-blue-300">
              <span>{t("canvas.story.estimatedDuration")}</span>
              <span className="font-semibold">
                {t("canvas.story.estimatedDurationValue", {
                  count: Number(data.storySegmentCount || 4),
                  duration: Number(data.storySegmentDuration || 8),
                  total: Number(data.storySegmentCount || 4) * Number(data.storySegmentDuration || 8),
                })}
              </span>
            </div>
          </div>
        )}
        {data.viralRole === "brief" && (
          <div className="nodrag grid grid-cols-2 gap-2 rounded-xl border border-orange-200/70 bg-orange-50/70 p-2 dark:border-orange-400/15 dark:bg-orange-500/[0.06]">
            <label className="min-w-0 text-[9px] text-gray-500 dark:text-gray-300">
              <span className="mb-1 block">{t("canvas.viral.segmentCount")}</span>
              <select
                value={Number(data.viralSegmentCount || 3)}
                onChange={(event) => actions?.configureViral(
                  id,
                  Number(event.target.value),
                  Number(data.viralSegmentDuration || 5)
                )}
                className="h-8 w-full rounded-lg border border-orange-200 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none dark:border-orange-400/20 dark:bg-gray-900 dark:text-gray-100"
              >
                {VIRAL_SEGMENT_COUNT_OPTIONS.map((count) => (
                  <option key={count} value={count}>{t("canvas.story.segmentCountValue", { count })}</option>
                ))}
              </select>
            </label>
            <label className="min-w-0 text-[9px] text-gray-500 dark:text-gray-300">
              <span className="mb-1 block">{t("canvas.story.segmentDuration")}</span>
              <select
                value={Number(data.viralSegmentDuration || 5)}
                onChange={(event) => actions?.configureViral(
                  id,
                  Number(data.viralSegmentCount || 3),
                  Number(event.target.value)
                )}
                className="h-8 w-full rounded-lg border border-orange-200 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none dark:border-orange-400/20 dark:bg-gray-900 dark:text-gray-100"
              >
                {(Array.isArray(data.viralDurationOptions) && data.viralDurationOptions.length
                  ? data.viralDurationOptions
                  : [Number(data.viralSegmentDuration || 5)]
                ).map((duration) => (
                  <option key={duration} value={duration}>{t("canvas.story.secondsValue", { seconds: duration })}</option>
                ))}
              </select>
            </label>
            <div className="col-span-2 flex items-center justify-between gap-2 text-[9px] text-orange-600 dark:text-orange-300">
              <span>{t("canvas.story.estimatedDuration")}</span>
              <span className="font-semibold">
                {t("canvas.story.estimatedDurationValue", {
                  count: Number(data.viralSegmentCount || 3),
                  duration: Number(data.viralSegmentDuration || 5),
                  total: Number(data.viralSegmentCount || 3) * Number(data.viralSegmentDuration || 5),
                })}
              </span>
            </div>
          </div>
        )}
        {(imageURLs.length > 0 || videoURLs.length > 0) && <div className="text-[9px] text-gray-400">{t("canvas.node.linkedAssets", { count: imageURLs.length + videoURLs.length })}</div>}
        {(imageURLs.length > 0 || videoURLs.length > 0 || audioURLs.length > 0) && (
          <div className="flex gap-1.5 overflow-x-auto pb-0.5">
            {imageURLs.map((url, index) => (
              <div key={`image-${url}-${index}`} className="group relative h-11 w-11 shrink-0 overflow-hidden rounded-lg border border-gray-100 dark:border-white/10">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={url} alt="" className="h-full w-full object-cover" />
                <button type="button" onClick={() => actions?.update(id, {
                  referenceImageUrls: imageURLs.filter((_, itemIndex) => itemIndex !== index),
                  referenceImageIds: (data.referenceImageIds || []).filter((_, itemIndex) => itemIndex !== index),
                })} className="nodrag absolute right-0.5 top-0.5 rounded bg-black/65 p-0.5 text-white opacity-0 group-hover:opacity-100"><X size={9} /></button>
              </div>
            ))}
            {videoURLs.map((url, index) => (
              <div key={`video-${url}-${index}`} className="group relative h-11 w-11 shrink-0 overflow-hidden rounded-lg border border-gray-100 dark:border-white/10">
                <video src={url} muted preload="metadata" className="h-full w-full object-cover" />
                <button type="button" onClick={() => actions?.update(id, {
                  referenceVideoUrls: videoURLs.filter((_, itemIndex) => itemIndex !== index),
                  referenceVideoIds: (data.referenceVideoIds || []).filter((_, itemIndex) => itemIndex !== index),
                })} className="nodrag absolute right-0.5 top-0.5 rounded bg-black/65 p-0.5 text-white opacity-0 group-hover:opacity-100"><X size={9} /></button>
              </div>
            ))}
            {audioURLs.map((url, index) => (
              <div key={`audio-${url}-${index}`} className="group relative flex h-11 w-28 shrink-0 items-center overflow-hidden rounded-lg border border-gray-100 px-1 dark:border-white/10">
                <audio src={url} controls preload="metadata" className="h-7 w-full" />
                <button type="button" onClick={() => actions?.update(id, {
                  referenceAudioUrls: audioURLs.filter((_, itemIndex) => itemIndex !== index),
                  referenceAudioIds: (data.referenceAudioIds || []).filter((_, itemIndex) => itemIndex !== index),
                })} className="nodrag absolute right-0.5 top-0.5 rounded bg-black/65 p-0.5 text-white opacity-0 group-hover:opacity-100"><X size={9} /></button>
              </div>
            ))}
          </div>
        )}
        <div className="mt-auto flex items-center justify-between text-[9px] text-gray-400">
          <span>{t("canvas.node.textOutputHint")}</span>
          <span>{String(data.prompt || "").length}/4000</span>
        </div>
      </div>
    </NodeFrame>
  );
}

function ImageInputNode({ id, data, selected }: NodeProps<CanvasNode>) {
  const actions = useContext(CanvasNodeActions);
  const { t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const mediaKind: GeneratorKind =
    data.mediaKind === "video" || data.mediaKind === "audio" ? data.mediaKind : "image";
  const mediaTitle =
    mediaKind === "video"
      ? t("canvas.node.referenceVideo")
      : mediaKind === "audio"
        ? t("canvas.node.referenceAudio")
        : t("canvas.node.referenceImage");
  const mediaIcon =
    mediaKind === "video" ? <Film size={16} /> : mediaKind === "audio" ? <Mic size={16} /> : <ImageIcon size={16} />;
  const urls = Array.isArray(data.assetUrls) && data.assetUrls.length ? data.assetUrls : data.assetUrl ? [data.assetUrl] : [];
  return (
    <NodeFrame id={id} selected={selected} title={data.label || mediaTitle} icon={mediaIcon}>
      <div className="space-y-2 p-2.5">
        <input
          ref={inputRef}
          type="file"
          accept={mediaKind === "video" ? "video/*" : mediaKind === "audio" ? "audio/*" : "image/*"}
          multiple
          className="hidden"
          onChange={(event) => {
            const files = Array.from(event.target.files || []);
            void (async () => {
              for (let index = 0; index < files.length; index += 1) {
                await actions?.upload(id, files[index], index > 0 || urls.length > 0);
              }
            })();
            event.target.value = "";
          }}
        />
        <div className="grid grid-cols-2 gap-1.5">
          <select
            value={mediaKind}
            onChange={(event) => actions?.update(id, { mediaKind: event.target.value as GeneratorKind, assetUrl: "", assetId: "", assetUrls: [], assetIds: [] })}
            className="nodrag col-span-2 h-8 rounded-lg border border-gray-100 bg-gray-50 px-2 text-[10px] outline-none dark:border-white/10 dark:bg-white/5 dark:text-gray-100"
          >
            <option value="image">{t("canvas.kind.image")}</option>
            <option value="video">{t("canvas.kind.video")}</option>
            <option value="audio">{t("canvas.kind.audio")}</option>
          </select>
          <button type="button" onClick={() => inputRef.current?.click()} className="nodrag h-8 rounded-lg border border-cyan-200 bg-cyan-50 px-2 text-[10px] font-medium text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300">
            <Upload size={12} className="mr-1 inline" />{t("canvas.node.addMedia")}
          </button>
          <button type="button" onClick={() => actions?.openAssetLibrary(id, mediaKind)} className="nodrag h-8 rounded-lg border border-violet-200 bg-violet-50 px-2 text-[10px] font-medium text-violet-600 dark:bg-violet-500/10 dark:text-violet-300">
            <FolderOpen size={12} className="mr-1 inline" />{t("canvas.assetLibrary")}
          </button>
        </div>
        {urls.length ? (
          <div className="grid max-h-32 grid-cols-2 gap-1.5 overflow-y-auto">
            {urls.map((url, index) => (
              <div key={`${url}-${index}`} className="group relative overflow-hidden rounded-lg border border-gray-100 bg-gray-950/5 dark:border-white/10">
                {mediaKind === "video"
                  ? <video src={url} className="h-16 w-full object-cover" />
                  : mediaKind === "audio"
                    ? <div className="flex h-16 items-center px-2"><audio src={url} controls preload="metadata" className="h-8 w-full" /></div>
                  // eslint-disable-next-line @next/next/no-img-element
                  : <img src={url} alt="" className="h-16 w-full object-cover" />}
                <button type="button" onClick={() => {
                  const nextUrls = urls.filter((_, itemIndex) => itemIndex !== index);
                  const ids = Array.isArray(data.assetIds) ? data.assetIds : data.assetId ? [data.assetId] : [];
                  const nextIDs = ids.filter((_, itemIndex) => itemIndex !== index);
                  actions?.update(id, { assetUrls: nextUrls, assetIds: nextIDs, assetUrl: nextUrls[0] || "", assetId: nextIDs[0] || "" });
                }} className="nodrag absolute right-1 top-1 rounded-md bg-black/60 p-1 text-white opacity-0 group-hover:opacity-100"><X size={10} /></button>
              </div>
            ))}
          </div>
        ) : (
          <button
            type="button"
            onClick={() => inputRef.current?.click()}
            className="nodrag flex h-20 w-full flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-gray-200 bg-gray-50 text-[10px] text-gray-400 hover:border-cyan-300 hover:text-cyan-600 dark:border-white/10 dark:bg-white/5"
          >
            {data.status === "running" ? <LoaderCircle size={22} className="animate-spin" /> : <Upload size={22} />}
            {data.status === "running"
              ? t("canvas.node.uploading")
              : mediaKind === "video"
                ? t("canvas.node.uploadVideo")
                : mediaKind === "audio"
                  ? t("canvas.node.uploadAudio")
                  : t("canvas.node.uploadImage")}
          </button>
        )}
        <input
          value={data.prompt || ""}
          onChange={(event) => actions?.update(id, { prompt: event.target.value })}
          placeholder={t("canvas.node.mediaNote")}
          className="nodrag h-8 w-full rounded-lg border border-gray-100 bg-gray-50 px-2.5 text-[10px] outline-none dark:border-white/10 dark:bg-white/5 dark:text-gray-100"
        />
        {data.error && <p className="mt-2 text-[11px] text-red-500">{data.error}</p>}
      </div>
    </NodeFrame>
  );
}

function GeneratorNode({ id, data, selected }: NodeProps<CanvasNode>) {
  const actions = useContext(CanvasNodeActions);
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const videoInputRef = useRef<HTMLInputElement>(null);
  const audioInputRef = useRef<HTMLInputElement>(null);
  const kind = data.mediaKind || "image";
  const models = modelsForKind(kind, actions);
  const selectedModel = models.find((model) => model.code === data.modelCode);
  const selectedVideoRuntime = kind === "video" ? parseVideoRuntime(selectedModel?.runtime_rule) : null;
  const isSeedanceFullReference = kind === "video" && selectedVideoRuntime?.upload_profile === "seedance_2";
  const rawModelSchema = selectedModel ? canvasInputSchema(kind, selectedModel.input_schema) : {};
  const modelSchema = isSeedanceFullReference
    ? omitSchemaField(rawModelSchema, selectedVideoRuntime?.mode_param || "generation_mode")
    : rawModelSchema;
  const configurableFields = Object.keys(schemaProperties(modelSchema)).length;
  const referenceImages = Array.isArray(data.referenceImageUrls) ? data.referenceImageUrls.map(String) : [];
  const referenceVideos = Array.isArray(data.referenceVideoUrls) ? data.referenceVideoUrls.map(String) : [];
  const referenceAudios = Array.isArray(data.referenceAudioUrls) ? data.referenceAudioUrls.map(String) : [];
  const storyAudioURLs = Array.isArray(data.outputUrls) ? data.outputUrls.map(String).filter(Boolean) : [];
  const storySpeechPlan = Array.isArray(data.storySpeechPlan) ? data.storySpeechPlan : [];
  const storyVoiceConfig = kind === "audio" && data.storyRole === "narration"
    ? storyVoiceConfiguration(selectedModel, data.params || {})
    : null;
  const storySpeakers = Array.from(new Map(storySpeechPlan.map((item) => [item.speaker_code, item.speaker_name])).entries());
  const seedanceImageLimit = selectedVideoRuntime?.reference_images?.max ?? selectedVideoRuntime?.max_reference_images ?? 9;
  const seedanceMaterialMode = inferSeedanceMaterialMode(referenceImages.length, referenceVideos.length, referenceAudios.length);
  const seedanceAudioNeedsVisual = isSeedanceFullReference
    && referenceAudios.length > 0
    && referenceImages.length === 0
    && referenceVideos.length === 0;
  const kindLabel = t(`canvas.kind.${kind}`);
  const generationTitle =
    kind === "text"
      ? t("canvas.node.textGeneration")
      : kind === "video"
      ? t("canvas.node.videoGeneration")
      : kind === "audio"
        ? t("canvas.node.audioGeneration")
        : t("canvas.node.imageGeneration");
  const tone =
    kind === "text"
      ? {
          border: "border-cyan-400/60",
          select: "border-cyan-400/40 bg-cyan-500/10 focus:border-cyan-400",
          result: "border-cyan-400/25 bg-cyan-500/5 text-cyan-600 dark:text-cyan-300",
          button: "bg-cyan-500 hover:bg-cyan-600",
        }
      : kind === "video"
      ? {
          border: "border-pink-400/60",
          select: "border-pink-400/40 bg-pink-500/10 focus:border-pink-400",
          result: "border-pink-400/25 bg-pink-500/5 text-pink-500 dark:text-pink-300",
          button: "bg-pink-500 hover:bg-pink-600",
        }
      : kind === "audio"
        ? {
            border: "border-violet-400/60",
            select: "border-violet-400/40 bg-violet-500/10 focus:border-violet-400",
            result: "border-violet-400/25 bg-violet-500/5 text-violet-500 dark:text-violet-300",
            button: "bg-violet-500 hover:bg-violet-600",
          }
        : {
            border: "border-amber-400/60",
            select: "border-amber-400/40 bg-amber-500/10 focus:border-amber-400",
            result: "border-amber-400/25 bg-amber-500/5 text-amber-600 dark:text-amber-300",
            button: "bg-amber-500 hover:bg-amber-600",
          };
  const referenceRows: Array<{ kind: GeneratorKind; urls: string[]; urlKey: keyof CanvasNodeData; idKey: keyof CanvasNodeData; inputRef: React.RefObject<HTMLInputElement | null>; accept: string; label: string }> =
    kind === "text"
      ? []
      : kind === "image"
      ? [{ kind: "image", urls: referenceImages, urlKey: "referenceImageUrls", idKey: "referenceImageIds", inputRef: imageInputRef, accept: "image/*", label: data.referenceImageLabel || t("canvas.node.referenceImages") }]
      : kind === "audio"
        ? [{ kind: "audio", urls: referenceAudios, urlKey: "referenceAudioUrls", idKey: "referenceAudioIds", inputRef: audioInputRef, accept: "audio/*", label: data.referenceAudioLabel || t("canvas.node.referenceAudio") }]
        : [
            ...(!isSeedanceFullReference ? [{ kind: "image" as const, urls: referenceImages, urlKey: "referenceImageUrls" as const, idKey: "referenceImageIds" as const, inputRef: imageInputRef, accept: "image/*", label: data.referenceImageLabel || t("canvas.node.referenceImages") }] : []),
            { kind: "video", urls: referenceVideos, urlKey: "referenceVideoUrls", idKey: "referenceVideoIds", inputRef: videoInputRef, accept: "video/*", label: data.referenceVideoLabel || t("canvas.node.referenceVideos") },
            { kind: "audio", urls: referenceAudios, urlKey: "referenceAudioUrls", idKey: "referenceAudioIds", inputRef: audioInputRef, accept: "audio/*", label: data.referenceAudioLabel || t("canvas.node.referenceAudio") },
          ];
  const appendReferenceMention = (token: string) => {
    const prompt = String(data.prompt || "").trimEnd();
    actions?.update(id, { prompt: `${prompt}${prompt ? " " : ""}${token} ` });
  };
  return (
    <NodeFrame
      id={id}
      selected={selected}
      title={data.label || generationTitle}
      icon={kindIcon(kind)}
      status={data.status}
      progress={Number(data.progress || 0)}
      progressLabel={t(data.progressStage || (kind === "text" ? "canvas.progress.text" : kind === "image" ? "canvas.progress.image" : kind === "video" ? "canvas.progress.video" : "canvas.progress.audio"))}
      runnable
      className={`w-[360px] ${tone.border}`}
    >
      <div className="space-y-2.5 p-2.5">
        <select
          value={data.modelCode || ""}
          onChange={(event) => {
            const model = models.find((item) => item.code === event.target.value);
            let nextParams = canvasModelDefaults(kind, model);
            if (kind === "video" && (data.storyRole === "video" || data.viralRole === "video")) {
              const wantedDuration = Number(data.storySegmentDuration || data.viralSegmentDuration || 0);
              if (wantedDuration > 0) {
                nextParams = normalizeCanvasParamsForModel(
                  { ...nextParams, duration: wantedDuration },
                  model?.input_schema,
                  model?.default_params
                );
              }
            }
            if (data.viralRole === "keyframe" || data.viralRole === "video") {
              nextParams = aspectRatioParams(model, nextParams, "9:16");
            }
            actions?.update(id, {
              modelCode: event.target.value,
              params: nextParams,
              error: "",
            });
          }}
          className={`nodrag h-9 w-full rounded-lg border px-2.5 text-[11px] font-medium outline-none dark:text-gray-100 ${tone.select}`}
        >
          <option value="">{t("canvas.node.selectModel", { kind: kindLabel })}</option>
          {models.map((model) => (
            <option key={model.code} value={model.code}>{model.display_name}</option>
          ))}
        </select>
        {selectedModel && (
          <div className="nodrag rounded-lg border border-gray-100 bg-gray-50/70 px-2 py-2 dark:border-white/10 dark:bg-white/[0.035]">
            {configurableFields > 0 ? (
              <div className="max-h-32 overflow-y-auto pr-0.5">
                <SchemaForm schema={modelSchema} values={data.params || {}} onChange={(params) => actions?.update(id, { params })} />
              </div>
            ) : (
              <div className="text-[10px] text-gray-400">{t("canvas.node.noModelParameters")}</div>
            )}
          </div>
        )}
        {kind === "audio" && data.storyRole === "narration" && (
          <div className="nodrag space-y-2 rounded-lg border border-violet-400/20 bg-violet-500/5 p-2.5">
            <label className="block rounded-lg border border-violet-400/20 bg-white/70 p-2 dark:bg-gray-950/40">
              <span className="mb-1.5 block text-[10px] font-semibold text-violet-600 dark:text-violet-300">{t("canvas.story.narrationMode")}</span>
              <select
                value={normalizeStoryNarrationMode(data.storyNarrationMode)}
                onChange={(event) => actions?.configureStory(
                  id,
                  Number(data.storySegmentCount || 4),
                  Number(data.storySegmentDuration || 8),
                  normalizeStoryNarrationMode(event.target.value),
                )}
                className="h-8 w-full rounded-lg border border-violet-300/30 bg-white px-2 text-[10px] font-medium text-gray-700 outline-none focus:border-violet-400 dark:bg-gray-900 dark:text-gray-100"
              >
                {STORY_NARRATION_MODES.map((mode) => (
                  <option key={mode} value={mode}>{t(`canvas.story.narrationMode.${mode}`)}</option>
                ))}
              </select>
            </label>
            <div className="text-[10px] font-semibold text-violet-600 dark:text-violet-300">{t("canvas.story.voiceDirection")}</div>
            {storySpeakers.length > 0 ? (
              <div className="space-y-1.5">
                {storySpeakers.map(([speakerCode, speakerName]) => (
                  <label key={speakerCode} className="flex items-center gap-2 text-[9px] text-gray-500 dark:text-gray-300">
                    <span className="min-w-0 flex-1 truncate" title={`${speakerName} (${speakerCode})`}>{speakerName}</span>
                    {storyVoiceConfig?.options.length ? (
                      <select
                        value={data.storyVoiceOverrides?.[speakerCode] || data.storyVoiceAssignments?.[speakerCode] || storyVoiceConfig.configured || storyVoiceConfig.options[0]}
                        onChange={(event) => actions?.update(id, { storyVoiceOverrides: { ...(data.storyVoiceOverrides || {}), [speakerCode]: event.target.value } })}
                        className="h-7 max-w-[190px] rounded-md border border-violet-300/30 bg-white px-2 text-[9px] text-gray-700 dark:bg-gray-900 dark:text-gray-100"
                      >
                        {storyVoiceConfig.options.map((voice) => <option key={voice} value={voice}>{voice}</option>)}
                      </select>
                    ) : <span className="max-w-[190px] truncate text-amber-600 dark:text-amber-300">{data.storyVoiceAssignments?.[speakerCode] || storyVoiceConfig?.configured || t("canvas.story.defaultVoice")}</span>}
                  </label>
                ))}
              </div>
            ) : <div className="text-[9px] leading-relaxed text-gray-400">{t("canvas.story.voicePlanAfterRun")}</div>}
          </div>
        )}
        {isSeedanceFullReference && (
          <div className="nodrag space-y-2 border-t border-pink-400/20 pt-2.5">
            <div className="flex items-center gap-2 rounded-lg border border-pink-400/20 bg-pink-500/5 px-2.5 py-2">
              <span className="text-[9px] text-gray-400">{t("video.generationMode")}</span>
              <span className="min-w-0 flex-1 truncate text-[10px] font-semibold text-pink-500 dark:text-pink-300">
                {t(`video.option.generation_mode.${seedanceMaterialMode}`)}
              </span>
              <span className="shrink-0 rounded-full bg-cyan-500/10 px-2 py-0.5 text-[9px] font-semibold text-cyan-600 dark:text-cyan-300">
                {t("canvas.node.autoMaterialMode")}
              </span>
            </div>
            {seedanceAudioNeedsVisual && (
              <div className="rounded-lg bg-amber-500/10 px-2.5 py-2 text-[9px] leading-relaxed text-amber-600 dark:text-amber-300">
                {t("canvas.node.seedanceAudioNeedsVisual")}
              </div>
            )}
            <div className="flex items-center justify-between gap-2">
              <div>
                <div className="text-[10px] font-semibold text-pink-500 dark:text-pink-300">{data.referenceImageLabel || t("canvas.node.avatarAndFirstFrame")}</div>
                <div className="mt-0.5 text-[9px] text-gray-400">{t("canvas.node.seedancePortraitHint")}</div>
              </div>
              <span className="rounded-full bg-pink-500/10 px-2 py-0.5 text-[9px] font-semibold text-pink-500">{t("canvas.node.fullReference")}</span>
            </div>
            <input
              ref={imageInputRef}
              type="file"
              accept="image/*"
              multiple
              className="hidden"
              onChange={(event) => {
                const room = Math.max(0, seedanceImageLimit - referenceImages.length);
                Array.from(event.target.files || []).slice(0, room).forEach((file) => void actions?.uploadReference(id, "image", file));
                event.target.value = "";
              }}
            />
            <div className="flex flex-wrap items-center gap-1.5">
              {referenceImages.map((url, index) => (
                <div key={`${url}-${index}`} className="group relative h-14 w-14 overflow-hidden rounded-xl border border-pink-300/30 bg-pink-500/5">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={url} alt="" className="h-full w-full object-cover" />
                  <button type="button" onClick={() => actions?.update(id, {
                    referenceImageUrls: referenceImages.filter((_, itemIndex) => itemIndex !== index),
                    referenceImageIds: (data.referenceImageIds || []).filter((_, itemIndex) => itemIndex !== index),
                  })} className="absolute right-0.5 top-0.5 rounded bg-black/65 p-0.5 text-white opacity-0 group-hover:opacity-100"><X size={9} /></button>
                </div>
              ))}
              <button type="button" disabled={referenceImages.length >= seedanceImageLimit} onClick={() => imageInputRef.current?.click()} className="flex h-14 w-14 flex-col items-center justify-center gap-0.5 rounded-xl border border-dashed border-pink-300/50 bg-pink-500/5 text-pink-500 hover:bg-pink-500/10 disabled:cursor-not-allowed disabled:opacity-40">
                <Plus size={15} /><span className="text-[9px]">{t("common.upload")}</span>
              </button>
              <button type="button" disabled={referenceImages.length >= seedanceImageLimit} onClick={() => actions?.openAssetLibrary(id, "image")} className="flex h-14 min-w-14 flex-col items-center justify-center gap-0.5 rounded-xl border border-dashed border-violet-300/50 bg-violet-500/5 px-2 text-violet-500 hover:bg-violet-500/10 disabled:cursor-not-allowed disabled:opacity-40">
                <FolderOpen size={14} /><span className="text-[9px]">{t("canvas.assetLibrary")}</span>
              </button>
            </div>
          </div>
        )}
        {kind === "text" && data.outputText ? (
          <div className={`relative max-h-44 overflow-y-auto whitespace-pre-wrap rounded-xl border p-3 pr-10 text-[11px] leading-relaxed ${tone.result}`}>
            <button
              type="button"
              title={copied ? t("common.copied") : t("common.copy")}
              aria-label={copied ? t("common.copied") : t("common.copy")}
              onClick={async () => {
                await copyCanvasText(String(data.outputText || ""));
                setCopied(true);
                window.setTimeout(() => setCopied(false), 1600);
              }}
              className="nodrag sticky right-0 top-0 float-right -mr-7 -mt-1 flex h-7 w-7 items-center justify-center rounded-lg border border-cyan-300/30 bg-white/85 text-cyan-600 shadow-sm backdrop-blur hover:bg-white dark:bg-gray-900/85 dark:text-cyan-300"
            >
              {copied ? <Check size={13} /> : <Copy size={13} />}
            </button>
            {data.outputText}
          </div>
        ) : kind === "audio" && storyAudioURLs.length > 1 ? (
          <div className={`max-h-48 space-y-2 overflow-y-auto rounded-xl border p-2 ${tone.result}`}>
            <div className="flex items-center justify-between px-1 text-[10px] font-semibold">
              <span>{t("canvas.story.generatedTracks", { count: storyAudioURLs.length })}</span>
              <span className="text-gray-400">{t(`canvas.story.narrationMode.${normalizeStoryNarrationMode(data.storyNarrationMode)}`)}</span>
            </div>
            {storyAudioURLs.map((url, index) => (
              <div key={`${url}-${index}`} className="rounded-lg border border-violet-400/15 bg-white/60 p-1.5 dark:bg-black/15">
                <div className="mb-1 truncate px-1 text-[9px] text-gray-400">
                  {data.storySpeechPlan?.[index]?.speaker_name || t("canvas.story.track", { index: index + 1 })} · {data.storySpeechPlan?.[index]?.text || ""}
                </div>
                <audio src={url} controls className="h-8 w-full" />
              </div>
            ))}
          </div>
        ) : data.outputUrl ? (
          <div className={`group/result relative overflow-hidden rounded-xl border p-2 ${tone.result}`}>
            <div className="absolute right-3 top-3 z-10 flex gap-1 opacity-100 transition sm:opacity-0 sm:group-hover/result:opacity-100 focus-within:opacity-100">
              <button
                type="button"
                onClick={() => actions?.openResultPreview({
                  url: String(data.outputUrl),
                  kind: kind as Exclude<GeneratorKind, "text">,
                  title: data.label || generationTitle,
                })}
                title={t("common.preview")}
                className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-white/20 bg-gray-950/75 text-white shadow backdrop-blur hover:bg-gray-900"
              >
                <Eye size={13} />
              </button>
              <button
                type="button"
                onClick={() => void downloadCanvasResult(
                  String(data.outputUrl),
                  `starai-${kind}-${Date.now()}.${kind === "image" ? "png" : kind === "video" ? "mp4" : "mp3"}`
                )}
                title={t("common.download")}
                className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-white/20 bg-gray-950/75 text-white shadow backdrop-blur hover:bg-gray-900"
              >
                <Download size={13} />
              </button>
            </div>
            {kind === "video" ? (
              <video src={data.outputUrl} controls className="max-h-52 w-full rounded-lg object-contain" />
            ) : kind === "audio" ? (
              <audio src={data.outputUrl} controls className="w-full" />
            ) : (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={data.outputUrl} alt="" className="max-h-52 w-full rounded-lg object-contain" />
            )}
          </div>
        ) : (
          <div className={`flex h-20 items-center gap-3 rounded-xl border border-dashed px-3 ${tone.result}`}>
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-current/10">{kindIcon(kind)}</span>
            <span>
              <span className="block text-[11px] font-semibold">{t(`canvas.node.${kind}ResultWaiting`)}</span>
              <span className="mt-0.5 block text-[9px] text-gray-400">{t("canvas.node.resultWaitingDesc")}</span>
            </span>
          </div>
        )}
        <textarea
          rows={1}
          className="nodrag nowheel h-9 min-h-9 max-h-32 w-full resize-y rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-2 text-[11px] leading-[18px] outline-none focus:border-cyan-300 dark:border-white/10 dark:bg-black/15 dark:text-gray-100"
          placeholder={isSeedanceFullReference ? t("canvas.node.seedancePromptPlaceholder") : t("canvas.node.promptPlaceholder")}
          value={data.prompt || ""}
          onChange={(event) => actions?.update(id, { prompt: event.target.value })}
        />
        {isSeedanceFullReference && (referenceImages.length > 0 || referenceVideos.length > 0 || referenceAudios.length > 0) && (
          <div className="nodrag flex flex-wrap items-center gap-1">
            <span className="mr-0.5 text-[9px] text-gray-400">{t("canvas.node.quickReference")}</span>
            {referenceImages.map((_, index) => <button key={`mention-image-${index}`} type="button" onClick={() => appendReferenceMention(`@${t("canvas.kind.image")}${index + 1}`)} className="rounded-md bg-pink-500/10 px-1.5 py-1 text-[9px] text-pink-500">@{t("canvas.kind.image")}{index + 1}</button>)}
            {referenceVideos.map((_, index) => <button key={`mention-video-${index}`} type="button" onClick={() => appendReferenceMention(`@${t("canvas.kind.video")}${index + 1}`)} className="rounded-md bg-pink-500/10 px-1.5 py-1 text-[9px] text-pink-500">@{t("canvas.kind.video")}{index + 1}</button>)}
            {referenceAudios.map((_, index) => <button key={`mention-audio-${index}`} type="button" onClick={() => appendReferenceMention(`@${t("canvas.kind.audio")}${index + 1}`)} className="rounded-md bg-violet-500/10 px-1.5 py-1 text-[9px] text-violet-500">@{t("canvas.kind.audio")}{index + 1}</button>)}
          </div>
        )}
        <div className="space-y-1.5 border-t border-gray-100 pt-2 dark:border-white/10">
          {referenceRows.map((row) => (
            <div key={row.kind} className="flex items-center gap-2">
              <input
                ref={row.inputRef}
                type="file"
                accept={row.accept}
                multiple
                className="hidden"
                onChange={(event) => {
                  Array.from(event.target.files || []).forEach((file) => void actions?.uploadReference(id, row.kind, file));
                  event.target.value = "";
                }}
              />
              <span className="min-w-0 flex-1 truncate text-[10px] text-gray-500 dark:text-gray-300">
                {row.label}
                {row.urls.length > 0 && <span className="ml-1 text-cyan-500">{row.urls.length}</span>}
              </span>
              <button type="button" onClick={() => row.inputRef.current?.click()} title={t("canvas.node.addMedia")} className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-cyan-300 hover:text-cyan-500 dark:border-white/10"><Plus size={13} /></button>
              <button type="button" onClick={() => actions?.openAssetLibrary(id, row.kind)} title={t("canvas.assetLibrary")} className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-violet-300 hover:text-violet-500 dark:border-white/10"><FolderOpen size={13} /></button>
              {row.urls.length > 0 && (
                <button
                  type="button"
                  onClick={() => actions?.update(id, { [row.urlKey]: [], [row.idKey]: [] })}
                  title={t("canvas.node.clearReferences")}
                  className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-gray-200 text-gray-400 hover:border-red-300 hover:text-red-500 dark:border-white/10"
                >
                  <X size={12} />
                </button>
              )}
            </div>
          ))}
        </div>
        {data.warning && (
          <div className="rounded-lg bg-amber-50 px-2.5 py-2 text-[10px] leading-relaxed text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">{data.warning}</div>
        )}
        {data.error && (
          <div className="flex items-center gap-2 rounded-lg bg-red-50 px-2.5 py-2 text-[11px] text-red-600 dark:bg-red-500/10 dark:text-red-300">
            <span className="min-w-0 flex-1">{data.error}</span>
            <button type="button" onClick={() => void actions?.run(id)} className="nodrag shrink-0 rounded-md border border-red-200 px-2 py-1 text-[10px] font-semibold hover:bg-red-100 dark:border-red-400/20 dark:hover:bg-red-500/10">
              {t("canvas.node.retry")}
            </button>
          </div>
        )}
        <div className="flex items-center justify-between">
          <div className="min-w-0">
            <div className="truncate text-[10px] text-gray-400">
              {data.mode || (kind === "text" ? t("canvas.node.textMode") : kind === "video" ? t("canvas.node.videoMode") : kind === "audio" ? t("canvas.node.audioMode") : t("canvas.node.imageMode"))}
            </div>
            {(Number(data.actualCost || 0) > 0 || Number(data.estimatedCost || 0) > 0) && (
              <div className="mt-0.5 text-[10px] font-medium text-cyan-600 dark:text-cyan-300">
                {Number(data.actualCost || 0) > 0 ? "实际" : "预估"} {Number(data.actualCost || data.estimatedCost || 0).toFixed(2)} 算力
              </div>
            )}
          </div>
        </div>
      </div>
    </NodeFrame>
  );
}

function CompositorNode({ id, data, selected }: NodeProps<CanvasNode>) {
  const actions = useContext(CanvasNodeActions);
  const { t } = useI18n();
  return (
    <NodeFrame
      id={id}
      selected={selected}
      title={data.label || t("canvas.node.compositor")}
      icon={<Boxes size={16} />}
      status={data.status}
      progress={Number(data.progress || 0)}
      progressLabel={t(data.progressStage || "canvas.progress.composing")}
      runnable
      className="w-[300px]"
    >
      <div className="space-y-2.5 p-2.5">
        <div className="rounded-lg border border-violet-300/40 bg-violet-500/10 px-2.5 py-2 text-[10px] leading-relaxed text-violet-600 dark:text-violet-300">
          {t("canvas.compositor.hint")}
        </div>
        <label className="block text-[10px] text-gray-500 dark:text-gray-300">
          <span className="mb-1 block">{t("canvas.compositor.mode")}</span>
          <select
            value={data.composeMode || "auto"}
            onChange={(event) => actions?.update(id, { composeMode: event.target.value })}
            className="nodrag h-8 w-full rounded-lg border border-gray-200 bg-gray-50 px-2 text-[11px] outline-none dark:border-white/10 dark:bg-white/5 dark:text-gray-100"
          >
            <option value="auto">{t("canvas.compositor.modeAuto")}</option>
            <option value="concat">{t("canvas.compositor.modeConcat")}</option>
            <option value="mux">{t("canvas.compositor.modeMux")}</option>
          </select>
        </label>
        <label className="block text-[10px] text-gray-500 dark:text-gray-300">
          <span className="mb-1 block">{t("canvas.compositor.outputSize")}</span>
          <select
            value={data.outputSize || "keep"}
            onChange={(event) => actions?.update(id, { outputSize: event.target.value })}
            className="nodrag h-8 w-full rounded-lg border border-gray-200 bg-gray-50 px-2 text-[11px] outline-none dark:border-white/10 dark:bg-white/5 dark:text-gray-100"
          >
            <option value="keep">{t("canvas.compositor.keepSize")}</option>
            <option value="1920x1080">1920×1080 (16:9)</option>
            <option value="1080x1920">1080×1920 (9:16)</option>
            <option value="1080x1080">1080×1080 (1:1)</option>
            <option value="720x1280">720×1280 (9:16)</option>
            <option value="720x480">720×480 (3:2)</option>
          </select>
        </label>
        {data.outputUrl ? (
          <div className="group/result relative overflow-hidden rounded-xl border border-violet-300/30 bg-violet-500/5 p-2">
            <div className="absolute right-3 top-3 z-10 flex gap-1 opacity-100 transition sm:opacity-0 sm:group-hover/result:opacity-100 focus-within:opacity-100">
              <button
                type="button"
                onClick={() => actions?.openResultPreview({
                  url: String(data.outputUrl),
                  kind: (data.outputKind || "video") as Exclude<GeneratorKind, "text">,
                  title: data.label || t("canvas.node.compositor"),
                })}
                title={t("common.preview")}
                className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-white/20 bg-gray-950/75 text-white shadow backdrop-blur hover:bg-gray-900"
              >
                <Eye size={13} />
              </button>
              <button
                type="button"
                onClick={() => {
                  const kind = data.outputKind || "video";
                  void downloadCanvasResult(String(data.outputUrl), `starai-compose-${Date.now()}.${kind === "image" ? "png" : kind === "audio" ? "mp3" : "mp4"}`);
                }}
                title={t("common.download")}
                className="nodrag flex h-7 w-7 items-center justify-center rounded-lg border border-white/20 bg-gray-950/75 text-white shadow backdrop-blur hover:bg-gray-900"
              >
                <Download size={13} />
              </button>
            </div>
            {data.outputKind === "video" ? (
              <video src={data.outputUrl} controls className="max-h-44 w-full rounded-lg object-contain" />
            ) : data.outputKind === "audio" ? (
              <audio src={data.outputUrl} controls className="w-full" />
            ) : (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={data.outputUrl} alt="" className="max-h-44 w-full rounded-lg object-contain" />
            )}
          </div>
        ) : (
          <div className="flex h-20 items-center gap-3 rounded-xl border border-dashed border-violet-300/30 bg-violet-500/5 px-3 text-violet-500 dark:text-violet-300">
            <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-violet-500/10"><Boxes size={17} /></span>
            <span>
              <span className="block text-[11px] font-semibold">{t("canvas.compositor.waiting")}</span>
              <span className="mt-0.5 block text-[9px] text-gray-400">{t("canvas.compositor.waitingDesc")}</span>
            </span>
          </div>
        )}
        {data.error && (
          <div className="flex items-center gap-2 rounded-lg bg-red-50 px-2.5 py-2 text-[11px] text-red-600 dark:bg-red-500/10 dark:text-red-300">
            <span className="min-w-0 flex-1">{data.error}</span>
            <button type="button" onClick={() => void actions?.run(id)} className="nodrag shrink-0 rounded-md border border-red-200 px-2 py-1 text-[10px] font-semibold hover:bg-red-100 dark:border-red-400/20 dark:hover:bg-red-500/10">
              {t("canvas.node.retry")}
            </button>
          </div>
        )}
      </div>
    </NodeFrame>
  );
}

const nodeTypes = {
  textInput: TextInputNode,
  imageInput: ImageInputNode,
  generator: GeneratorNode,
  compositor: CompositorNode,
};

function CanvasEditor({
  authenticated,
  workflowCode = "infinite_canvas",
  initialTemplateID = "",
}: {
  authenticated: boolean;
  workflowCode?: string;
  initialTemplateID?: string;
}) {
  const { locale, formatDate, t } = useI18n();
  const [nodes, setNodes, onNodesChange] = useNodesState<CanvasNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<CanvasEdge>([]);
  const [chatModels, setChatModels] = useState<Model[]>([]);
  const [imageModels, setImageModels] = useState<Model[]>([]);
  const [videoModels, setVideoModels] = useState<Model[]>([]);
  const [audioModels, setAudioModels] = useState<Model[]>([]);
  const [modelCatalogReady, setModelCatalogReady] = useState(false);
  const [workspaceConfigReady, setWorkspaceConfigReady] = useState(false);
  const [canvasID, setCanvasID] = useState("");
  const [title, setTitle] = useState(() => t("canvas.untitled"));
  const [history, setHistory] = useState<CanvasSummary[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [nodeSearch, setNodeSearch] = useState("");
  const [saving, setSaving] = useState(false);
  const [runningAll, setRunningAll] = useState(false);
  const [executionProgress, setExecutionProgress] = useState({ current: 0, total: 0 });
  const [notice, setNotice] = useState("");
  const [helpOpen, setHelpOpen] = useState(false);
  const [showMiniMap, setShowMiniMap] = useState(true);
  const [importOpen, setImportOpen] = useState(false);
  const [importTab, setImportTab] = useState<"templates" | "history" | "code">("templates");
  const [importCode, setImportCode] = useState("");
  const [managedTemplates, setManagedTemplates] = useState<CanvasTemplate[]>([]);
  const [workspaceRuntime, setWorkspaceRuntime] = useState<NonNullable<CanvasWorkflow["runtime_config"]>>({});
  const [showEmptyWelcome, setShowEmptyWelcome] = useState(true);
  const [nodePaletteOpen, setNodePaletteOpen] = useState(false);
  const [assetLibraryOpen, setAssetLibraryOpen] = useState(false);
  const [assetTargetID, setAssetTargetID] = useState("");
  const [assetTargetKind, setAssetTargetKind] = useState<GeneratorKind>("image");
  const [assetItems, setAssetItems] = useState<CanvasAsset[]>([]);
  const [assetQuery, setAssetQuery] = useState("");
  const [assetLoading, setAssetLoading] = useState(false);
  const [resultPreview, setResultPreview] = useState<CanvasResultPreview | null>(null);
  const [touchNavigation, setTouchNavigation] = useState(false);
  const [flowColorMode, setFlowColorMode] = useState<"light" | "dark">("light");
  const [outputMenu, setOutputMenu] = useState<{
    sourceID: string;
    left: number;
    top: number;
    nodePosition: { x: number; y: number };
  } | null>(null);
  const importRef = useRef<HTMLInputElement>(null);
  const editorRef = useRef<HTMLDivElement>(null);
  const nodesRef = useRef(nodes);
  const edgesRef = useRef(edges);
  const connectionSourceRef = useRef("");
  const connectionCompletedRef = useRef(false);
  const stopExecutionRef = useRef(false);
  const workflowNameRef = useRef(t("canvas.untitled"));
  const titleManuallyEditedRef = useRef(false);
  const executionActiveRef = useRef(false);
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastAutoSaveFingerprintRef = useRef("");
  const initialTemplateAppliedRef = useRef(false);
  const { fitView, getViewport, screenToFlowPosition, setViewport } = useReactFlow<CanvasNode, CanvasEdge>();

  useEffect(() => {
    nodesRef.current = nodes;
  }, [nodes]);
  useEffect(() => {
    edgesRef.current = edges;
  }, [edges]);

  useEffect(() => {
    const query = window.matchMedia("(pointer: coarse), (max-width: 1023px)");
    const updatePointerMode = () => setTouchNavigation(query.matches);
    updatePointerMode();
    query.addEventListener("change", updatePointerMode);
    return () => query.removeEventListener("change", updatePointerMode);
  }, []);

  useEffect(() => {
    const query = window.matchMedia("(prefers-color-scheme: dark)");
    const updateColorMode = () => setFlowColorMode(query.matches ? "dark" : "light");
    updateColorMode();
    query.addEventListener("change", updateColorMode);
    return () => query.removeEventListener("change", updateColorMode);
  }, []);

  const refreshHistory = useCallback(() => {
    if (!authenticated) {
      setHistory(readLocalCanvases()
        .filter((item) => (item.workflow_code || "infinite_canvas") === workflowCode)
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at)));
      return;
    }
    api<{ items: CanvasSummary[] }>(`/api/canvases?page_size=50&workflow_code=${encodeURIComponent(workflowCode)}`)
      .then((result) => setHistory(result.items || []))
      .catch(() => setHistory([]));
  }, [authenticated, workflowCode]);

  useEffect(() => {
    let active = true;
    const chatController = new AbortController();
    const imageController = new AbortController();
    const videoController = new AbortController();
    const audioController = new AbortController();
    setModelCatalogReady(false);
    Promise.allSettled([
      apiForLocale<Model[]>("/api/models?category=chat", locale, { signal: chatController.signal }),
      apiForLocale<Model[]>("/api/models?category=image", locale, { signal: imageController.signal }),
      apiForLocale<Model[]>("/api/models?category=video", locale, { signal: videoController.signal }),
      apiForLocale<Model[]>("/api/models?category=audio", locale, { signal: audioController.signal }),
    ]).then(([chat, image, video, audio]) => {
      if (!active) return;
      if (chat.status === "fulfilled") setChatModels((chat.value || []).filter((model) => model.is_enabled !== false && !isMultiCollabModel(model)));
      else setChatModels([]);
      setImageModels(image.status === "fulfilled" ? image.value || [] : []);
      setVideoModels(video.status === "fulfilled" ? video.value || [] : []);
      setAudioModels(audio.status === "fulfilled" ? audio.value || [] : []);
      setModelCatalogReady(true);
    });
    return () => {
      active = false;
      chatController.abort();
      imageController.abort();
      videoController.abort();
      audioController.abort();
    };
  }, [locale]);

  useEffect(() => {
    const controller = new AbortController();
    setWorkspaceConfigReady(false);
    apiForLocale<CanvasWorkflow>(`/api/agents/${encodeURIComponent(workflowCode)}`, locale, { signal: controller.signal })
      .then((workflow) => {
        const items = workflow.display_config?.canvas_templates;
        setManagedTemplates(Array.isArray(items) ? items.filter((item) => item && item.id && item.name) : []);
        setWorkspaceRuntime(workflow.runtime_config || {});
        setWorkspaceConfigReady(true);
      })
      .catch((error) => {
        if (error?.name !== "AbortError") {
          setManagedTemplates([]);
          setWorkspaceRuntime({});
          setWorkspaceConfigReady(true);
        }
      });
    return () => controller.abort();
  }, [locale, workflowCode]);

  useEffect(() => {
    refreshHistory();
  }, [refreshHistory]);

  const update = useCallback((id: string, patch: Partial<CanvasNodeData>) => {
    const runtimeKeys = new Set([
      "label",
      "status",
      "progress",
      "progressStage",
      "error",
      "dirty",
      "lastRunSignature",
      "outputUrl",
      "outputUrls",
      "outputText",
      "outputKind",
      "taskNo",
      "taskNos",
      "warning",
      "storySpeechPlan",
      "storyVoiceAssignments",
      "estimatedCost",
      "actualCost",
    ]);
    const patchKeys = Object.keys(patch);
    const configurationChanged = patchKeys.some((key) => !runtimeKeys.has(key));
    const currentNode = nodesRef.current.find((node) => node.id === id);
    const outputChanged =
      (Object.prototype.hasOwnProperty.call(patch, "outputUrl") && currentNode?.data.outputUrl !== patch.outputUrl)
      || (Object.prototype.hasOwnProperty.call(patch, "outputUrls") && JSON.stringify(currentNode?.data.outputUrls || []) !== JSON.stringify(patch.outputUrls || []))
      || (Object.prototype.hasOwnProperty.call(patch, "outputText") && currentNode?.data.outputText !== patch.outputText);
    const downstream = configurationChanged || outputChanged
      ? collectDownstreamIDs(id, edgesRef.current)
      : new Set<string>();
    const next: CanvasNode[] = nodesRef.current.map((node) => {
      if (node.id === id) {
        const executable = node.type === "generator" || node.type === "compositor";
        return {
          ...node,
          data: {
            ...node.data,
            ...patch,
            ...(configurationChanged && executable
              ? {
                  dirty: true,
                  status: node.data.status === "succeeded" ? "stale" : "idle",
                  error: "",
                }
              : {}),
          },
        };
      }
      if (downstream.has(node.id) && (node.type === "generator" || node.type === "compositor")) {
        return {
          ...node,
          data: {
            ...node.data,
            dirty: true,
            status: node.data.status === "running" || node.data.status === "pending"
              ? node.data.status
              : node.data.status === "succeeded"
                ? "stale"
                : "idle",
            error: "",
          },
        };
      }
      return node;
    });
    nodesRef.current = next;
    setNodes(next);
  }, [setNodes]);

  const markDirtyFrom = useCallback((id: string, includeSelf = true) => {
    const affected = collectDownstreamIDs(id, edgesRef.current);
    if (includeSelf) affected.add(id);
    const next: CanvasNode[] = nodesRef.current.map((node) => {
      if (!affected.has(node.id) || (node.type !== "generator" && node.type !== "compositor")) return node;
      return {
        ...node,
        data: {
          ...node.data,
          dirty: true,
          status: node.data.status === "succeeded" ? "stale" : "idle",
          error: "",
        },
      };
    });
    nodesRef.current = next;
    setNodes(next);
  }, [setNodes]);

  const remove = useCallback((id: string) => {
    const directTargets = edgesRef.current.filter((edge) => edge.source === id).map((edge) => edge.target);
    nodesRef.current = nodesRef.current.filter((node) => node.id !== id);
    edgesRef.current = edgesRef.current.filter((edge) => edge.source !== id && edge.target !== id);
    setNodes(nodesRef.current);
    setEdges(edgesRef.current);
    directTargets.forEach((targetID) => markDirtyFrom(targetID));
  }, [markDirtyFrom, setEdges, setNodes]);

  const upload = useCallback(async (id: string, file: File, append = false) => {
    if (!authenticated) {
      update(id, { status: "failed", error: t("canvas.loginRequiredToUpload") });
      return;
    }
    update(id, { status: "running", error: "" });
    try {
      const kind: GeneratorKind = file.type.startsWith("video/")
        ? "video"
        : file.type.startsWith("audio/")
          ? "audio"
          : "image";
      const asset = await uploadAsset(file, { name: file.name, kind, asset_type: "prop" });
      const current = nodesRef.current.find((node) => node.id === id)?.data;
      const urls = append ? [...(Array.isArray(current?.assetUrls) ? current.assetUrls : current?.assetUrl ? [String(current.assetUrl)] : []), asset.url] : [asset.url];
      const ids = append ? [...(Array.isArray(current?.assetIds) ? current.assetIds : current?.assetId ? [String(current.assetId)] : []), asset.public_id] : [asset.public_id];
      update(id, { assetUrl: urls[0], assetId: ids[0], assetUrls: urls, assetIds: ids, mediaKind: kind, status: "succeeded" });
    } catch (error) {
      update(id, { status: "failed", error: error instanceof Error ? error.message : "上传失败" });
    }
  }, [authenticated, t, update]);

  const uploadReference = useCallback(async (id: string, kind: GeneratorKind, file: File) => {
    if (!authenticated) {
      update(id, { status: "failed", error: t("canvas.loginRequiredToUpload") });
      return;
    }
    update(id, { status: "running", error: "" });
    try {
      const asset = await uploadAsset(file, { name: file.name, kind, asset_type: "prop" });
      const current = nodesRef.current.find((node) => node.id === id)?.data;
      const urlKey =
        kind === "video" ? "referenceVideoUrls" : kind === "audio" ? "referenceAudioUrls" : "referenceImageUrls";
      const idKey =
        kind === "video" ? "referenceVideoIds" : kind === "audio" ? "referenceAudioIds" : "referenceImageIds";
      const currentURLs = Array.isArray(current?.[urlKey]) ? (current?.[urlKey] as string[]) : [];
      const currentIDs = Array.isArray(current?.[idKey]) ? (current?.[idKey] as string[]) : [];
      update(id, {
        [urlKey]: [...currentURLs, asset.url],
        [idKey]: [...currentIDs, asset.public_id],
        status: "idle",
        error: "",
      });
    } catch (error) {
      update(id, { status: "failed", error: error instanceof Error ? error.message : t("canvas.assetUploadFailed") });
    }
  }, [authenticated, t, update]);

  const loadAssetLibrary = useCallback(async (query = "", kind = assetTargetKind) => {
    if (!authenticated) return;
    setAssetLoading(true);
    try {
      const result = await listAssets({ q: query.trim() || undefined, kind, page_size: 60 });
      setAssetItems(Array.isArray(result.items) ? result.items : []);
    } catch (error) {
      setAssetItems([]);
      setNotice(error instanceof Error ? error.message : t("canvas.assetLibraryLoadFailed"));
    } finally {
      setAssetLoading(false);
    }
  }, [assetTargetKind, authenticated, t]);

  const openAssetLibrary = useCallback((id: string, kind: GeneratorKind) => {
    if (!authenticated) {
      setNotice(t("canvas.loginRequiredToUseAssets"));
      return;
    }
    setAssetTargetID(id);
    setAssetTargetKind(kind);
    setAssetQuery("");
    setAssetLibraryOpen(true);
    void loadAssetLibrary("", kind);
  }, [authenticated, loadAssetLibrary, t]);

  const selectAsset = useCallback((asset: CanvasAsset) => {
    const targetNode = nodesRef.current.find((node) => node.id === assetTargetID);
    const current = targetNode?.data;
    if (!targetNode || !current) return;
    if (targetNode.type === "textInput") {
      if (assetTargetKind === "video") {
        const urls = Array.isArray(current.referenceVideoUrls) ? current.referenceVideoUrls.map(String) : [];
        const ids = Array.isArray(current.referenceVideoIds) ? current.referenceVideoIds.map(String) : [];
        update(assetTargetID, {
          referenceVideoUrls: urls.includes(asset.url) ? urls : [...urls, asset.url],
          referenceVideoIds: ids.includes(asset.public_id) ? ids : [...ids, asset.public_id],
        });
      } else if (assetTargetKind === "image") {
        const urls = Array.isArray(current.referenceImageUrls) ? current.referenceImageUrls.map(String) : [];
        const ids = Array.isArray(current.referenceImageIds) ? current.referenceImageIds.map(String) : [];
        update(assetTargetID, {
          referenceImageUrls: urls.includes(asset.url) ? urls : [...urls, asset.url],
          referenceImageIds: ids.includes(asset.public_id) ? ids : [...ids, asset.public_id],
        });
      } else {
        const urls = Array.isArray(current.referenceAudioUrls) ? current.referenceAudioUrls.map(String) : [];
        const ids = Array.isArray(current.referenceAudioIds) ? current.referenceAudioIds.map(String) : [];
        update(assetTargetID, {
          referenceAudioUrls: urls.includes(asset.url) ? urls : [...urls, asset.url],
          referenceAudioIds: ids.includes(asset.public_id) ? ids : [...ids, asset.public_id],
        });
      }
      setAssetLibraryOpen(false);
      return;
    }
    if (targetNode.type === "generator") {
      const urlKey =
        assetTargetKind === "video"
          ? "referenceVideoUrls"
          : assetTargetKind === "audio"
            ? "referenceAudioUrls"
            : "referenceImageUrls";
      const idKey =
        assetTargetKind === "video"
          ? "referenceVideoIds"
          : assetTargetKind === "audio"
            ? "referenceAudioIds"
            : "referenceImageIds";
      const currentURLs = Array.isArray(current[urlKey]) ? (current[urlKey] as string[]).map(String) : [];
      const currentIDs = Array.isArray(current[idKey]) ? (current[idKey] as string[]).map(String) : [];
      update(assetTargetID, {
        [urlKey]: currentURLs.includes(asset.url) ? currentURLs : [...currentURLs, asset.url],
        [idKey]: currentIDs.includes(asset.public_id) ? currentIDs : [...currentIDs, asset.public_id],
        error: "",
      });
      setAssetLibraryOpen(false);
      return;
    }
    const currentURLs = Array.isArray(current.assetUrls) ? current.assetUrls.map(String) : current.assetUrl ? [String(current.assetUrl)] : [];
    const currentIDs = Array.isArray(current.assetIds) ? current.assetIds.map(String) : current.assetId ? [String(current.assetId)] : [];
    const urls = currentURLs.includes(asset.url) ? currentURLs : [...currentURLs, asset.url];
    const ids = currentIDs.includes(asset.public_id) ? currentIDs : [...currentIDs, asset.public_id];
    update(assetTargetID, {
      assetUrl: urls[0] || "",
      assetId: ids[0] || "",
      assetUrls: urls,
      assetIds: ids,
      mediaKind: assetTargetKind,
      status: "succeeded",
      error: "",
    });
    setAssetLibraryOpen(false);
  }, [assetTargetID, assetTargetKind, update]);

  const runCompositor = useCallback(async (id: string) => {
    const node = nodesRef.current.find((item) => item.id === id);
    if (!node || node.type !== "compositor") return;
    const runSignature = nodeRunSignature(id, nodesRef.current, edgesRef.current);
    if (!authenticated) {
      update(id, { status: "failed", error: t("canvas.loginRequiredToRun") });
      return;
    }
    const directSources = edgesRef.current
      .filter((edge) => edge.target === id)
      .map((edge) => nodesRef.current.find((item) => item.id === edge.source))
      .filter((item): item is CanvasNode => !!item);
    const sources: Array<{ kind: GeneratorKind; url: string; task_no?: string; asset_id?: string }> = [];
    const addSource = (kind: GeneratorKind, url: string, taskNo?: string, assetID?: string) => {
      if (!url || (!taskNo && !assetID)) return;
      if (sources.some((item) => item.kind === kind && item.url === url)) return;
      sources.push({ kind, url, ...(taskNo ? { task_no: taskNo } : {}), ...(assetID ? { asset_id: assetID } : {}) });
    };
    directSources.forEach((source) => {
      const outputURLs = Array.isArray(source.data.outputUrls) ? source.data.outputUrls.map(String).filter(Boolean) : [];
      const outputTaskNos = Array.isArray(source.data.taskNos) ? source.data.taskNos.map(String) : [];
      if (outputURLs.length > 0 && source.data.outputKind) {
        outputURLs.forEach((url, index) => addSource(source.data.outputKind as GeneratorKind, url, outputTaskNos[index]));
      } else if (source.data.outputUrl && source.data.taskNo && source.data.outputKind) {
        addSource(source.data.outputKind, source.data.outputUrl, source.data.taskNo);
      }
      const mediaKind = source.data.mediaKind;
      const urls = Array.isArray(source.data.assetUrls) ? source.data.assetUrls.map(String) : source.data.assetUrl ? [String(source.data.assetUrl)] : [];
      const ids = Array.isArray(source.data.assetIds) ? source.data.assetIds.map(String) : source.data.assetId ? [String(source.data.assetId)] : [];
      if (mediaKind) urls.forEach((url, index) => addSource(mediaKind, url, undefined, ids[index]));
    });
    if (sources.length === 0) {
      update(id, { status: "failed", error: t("canvas.compositor.noSources") });
      return;
    }
    update(id, {
      status: "pending",
      progress: 6,
      progressStage: "canvas.progress.preparing",
      error: "",
      outputUrl: "",
      outputUrls: [],
      taskNos: [],
    });
    try {
      let task = await api<TaskResult>("/api/canvases/compose", {
        method: "POST",
        body: JSON.stringify({
          sources,
          mode: node.data.composeMode || "auto",
          output_size: node.data.outputSize || "keep",
        }),
      });
      update(id, {
        taskNo: task.task_no,
        status: task.status === "failed" ? "failed" : "running",
        progress: task.status === "failed" ? 0 : Math.max(12, Number(task.progress || 0)),
        progressStage: "canvas.progress.composing",
        error: task.error_message || "",
      });
      if (task.status === "failed") return;
      for (let attempt = 0; attempt < 240; attempt += 1) {
        await wait(2500);
        task = await api<TaskResult>(`/api/tasks/${task.task_no}`);
        const progress = runningProgress(task.progress, attempt);
        if (!["succeeded", "failed", "cancelled"].includes(task.status)) {
          update(id, {
            status: "running",
            progress,
            progressStage: progress >= 90 ? "canvas.progress.finalizing" : "canvas.progress.composing",
          });
        }
        if (task.status === "succeeded") {
          const outputKind =
            String(task.output?.media_kind || "") === "audio"
              ? "audio"
              : String(task.output?.media_kind || "") === "image"
                ? "image"
                : "video";
          const outputUrl = extractMedia(task.output, outputKind);
          update(id, {
            status: outputUrl ? "succeeded" : "failed",
            progress: outputUrl ? 100 : 0,
            progressStage: "canvas.progress.completed",
            outputUrl,
            outputKind,
            error: outputUrl ? "" : t("canvas.noMediaResult"),
            dirty: !outputUrl,
            lastRunSignature: outputUrl ? runSignature : node.data.lastRunSignature,
          });
          return;
        }
        if (["failed", "cancelled"].includes(task.status)) {
          update(id, { status: "failed", progress: 0, error: task.error_message || t("canvas.generationFailed") });
          return;
        }
      }
      update(id, { status: "failed", progress: 0, error: t("canvas.generationTimeout") });
    } catch (error) {
      update(id, { status: "failed", progress: 0, error: error instanceof Error ? error.message : t("canvas.generationFailed") });
    }
  }, [authenticated, t, update]);

  const run = useCallback(async (id: string) => {
    const node = nodesRef.current.find((item) => item.id === id);
    if (!node) return;
    if (node.type === "compositor") {
      await runCompositor(id);
      return;
    }
    if (node.type !== "generator") return;
    const runSignature = nodeRunSignature(id, nodesRef.current, edgesRef.current);
    if (!authenticated) {
      update(id, { status: "failed", error: t("canvas.loginRequiredToRun") });
      return;
    }
    const modelCode = String(node.data.modelCode || "");
    if (!modelCode) {
      update(id, { status: "failed", error: t("canvas.selectModelFirst") });
      return;
    }
    const selectedModel = [...chatModels, ...imageModels, ...videoModels, ...audioModels].find((item) => item.code === modelCode);
    if (!selectedModel) {
      update(id, { status: "failed", error: t("canvas.modelUnavailable") });
      return;
    }
    const incoming = collectUpstreamNodes(id, nodesRef.current, edgesRef.current);
    const directIncoming = edgesRef.current
      .filter((edge) => edge.target === id)
      .map((edge) => nodesRef.current.find((item) => item.id === edge.source))
      .filter((item): item is CanvasNode => Boolean(item));
    // 故事视频片段只使用自己直接连接的关键帧作为视觉输入，避免后续片段
    // 因关键帧一致性链路而把前面所有关键帧重复提交给视频模型。
    const mediaIncoming =
      node.data.storyRole === "video" || node.data.viralRole === "keyframe" || node.data.viralRole === "video"
        ? directIncoming
        : incoming;
    const storyRole = node.data.storyRole;
    const viralRole = node.data.viralRole;
    const textInputs =
      storyRole === "keyframe" || storyRole === "video" || storyRole === "narrationText"
        ? incoming
            .filter((item) => item.data.storyRole === "script")
            .map((item) => String(item.data.outputText || "").trim())
            .filter(Boolean)
        : storyRole === "narration"
          ? directIncoming
              .filter((item) => item.data.storyRole === "narrationText")
              .map((item) => String(item.data.outputText || "").trim())
              .filter(Boolean)
          : viralRole === "keyframe" || viralRole === "video"
            ? incoming
                .filter((item) => item.data.viralRole === "analysis")
                .map((item) => String(item.data.outputText || "").trim())
                .filter(Boolean)
          : incoming
              .flatMap((item) => [String(item.data.outputText || ""), String(item.data.prompt || "")])
              .filter(Boolean);
    const imageInputs = mediaIncoming
      .flatMap((item) => [
        ...(Array.isArray(item.data.referenceImageUrls) ? item.data.referenceImageUrls.map(String) : []),
        ...(item.data.mediaKind === "image" && Array.isArray(item.data.assetUrls) ? item.data.assetUrls.map(String) : []),
        String(item.data.mediaKind === "image" ? item.data.assetUrl || "" : ""),
        String(item.data.outputKind === "image" ? item.data.outputUrl || "" : ""),
      ])
      .filter(Boolean)
      .concat(Array.isArray(node.data.referenceImageUrls) ? node.data.referenceImageUrls.map(String) : []);
    const videoInputs = mediaIncoming
      .flatMap((item) => [
        ...(Array.isArray(item.data.referenceVideoUrls) ? item.data.referenceVideoUrls.map(String) : []),
        ...(item.data.mediaKind === "video" && Array.isArray(item.data.assetUrls) ? item.data.assetUrls.map(String) : []),
        String(item.data.mediaKind === "video" ? item.data.assetUrl || "" : ""),
        String(item.data.outputKind === "video" ? item.data.outputUrl || "" : ""),
      ])
      .filter(Boolean)
      .concat(Array.isArray(node.data.referenceVideoUrls) ? node.data.referenceVideoUrls.map(String) : []);
    const audioInputs = mediaIncoming
      .flatMap((item) => [
        ...(Array.isArray(item.data.referenceAudioUrls) ? item.data.referenceAudioUrls.map(String) : []),
        ...(item.data.mediaKind === "audio" && Array.isArray(item.data.assetUrls) ? item.data.assetUrls.map(String) : []),
        String(item.data.mediaKind === "audio" ? item.data.assetUrl || "" : ""),
        String(item.data.outputKind === "audio" ? item.data.outputUrl || "" : ""),
      ])
      .filter(Boolean)
      .concat(Array.isArray(node.data.referenceAudioUrls) ? node.data.referenceAudioUrls.map(String) : []);
    const legacyStoryNarrationPrompt = storyRole === "narrationText" && !node.data.storyNarrationMode
      ? t("canvas.story.narrationTextPrompt", {
          count: Number(node.data.storySegmentCount || 4),
          duration: Number(node.data.storySegmentDuration || 8),
          total: Number(node.data.storySegmentCount || 4) * Number(node.data.storySegmentDuration || 8),
          mode: t("canvas.story.narrationMode.smart"),
          instruction: t("canvas.story.narrationInstruction.smart"),
        })
      : "";
    const prompt = [legacyStoryNarrationPrompt || String(node.data.prompt || "").trim(), ...textInputs].filter(Boolean).join("\n\n");
    const videoRuntime = parseVideoRuntime(selectedModel?.runtime_rule);
    const audioRuntime = parseAudioRuntime(selectedModel?.runtime_rule);
    const isSeedance2 = node.data.mediaKind === "video" && videoRuntime.upload_profile === "seedance_2";
    const isMiniMaxH3 = node.data.mediaKind === "video" && videoRuntime.upload_profile === "minimax_h3";
    const promptRequired =
      node.data.mediaKind === "video"
        ? videoRuntime.prompt_required !== false
        : node.data.mediaKind === "audio"
          ? audioRuntime.prompt_required !== false
          : true;
    if (promptRequired && !prompt && !(isSeedance2 && (imageInputs.length > 0 || videoInputs.length > 0))) {
      update(id, { status: "failed", error: t("canvas.enterOrConnectPrompt") });
      return;
    }
    update(id, {
      status: "pending",
      progress: 6,
      progressStage: "canvas.progress.preparing",
      error: "",
      warning: "",
      outputUrl: "",
      outputUrls: [],
      outputText: "",
      taskNo: "",
      taskNos: [],
    });
    try {
      if (node.data.mediaKind === "text") {
        update(id, { status: "running", progress: 28, progressStage: "canvas.progress.text" });
        const result = await api<{ content: string; cost: number }>("/api/chat/completions", {
          method: "POST",
          body: JSON.stringify({
            model_code: modelCode,
            messages: [{ role: "user", content: prompt }],
            params: {
              ...(node.data.params || {}),
              ...(imageInputs.length ? { reference_images: imageInputs } : {}),
              ...(videoInputs.length ? { reference_videos: videoInputs } : {}),
            },
            stream: false,
            ephemeral: true,
          }),
        });
        const outputText = String(result.content || "").trim();
        update(id, {
          status: outputText ? "succeeded" : "failed",
          progress: outputText ? 100 : 0,
          progressStage: "canvas.progress.completed",
          outputText,
          outputKind: "text",
          error: outputText ? "" : t("canvas.noTextResult"),
          actualCost: Number(result.cost || 0),
          dirty: !outputText,
          lastRunSignature: outputText ? runSignature : node.data.lastRunSignature,
        });
        return;
      }
      if ((isSeedance2 || isMiniMaxH3) && audioInputs.length > 0 && imageInputs.length === 0 && videoInputs.length === 0) {
        update(id, {
          status: "failed",
          error: t(isMiniMaxH3 ? "canvas.node.referenceVisualRequired" : "canvas.node.seedanceAudioNeedsVisual"),
        });
        return;
      }
      const inferredSeedanceMode = inferSeedanceMaterialMode(imageInputs.length, videoInputs.length, audioInputs.length);
      const baseParams = normalizeCanvasParamsForModel({
        ...(selectedModel?.default_params || {}),
        ...(node.data.params || {}),
        user_prompt: prompt,
      }, selectedModel.input_schema, selectedModel.default_params);
      if (node.data.mediaKind === "audio") {
        delete baseParams.count;
        delete baseParams.n;
      }
      if (isSeedance2) baseParams[videoRuntime.mode_param || "generation_mode"] = inferredSeedanceMode;
      const h3Mode = String(baseParams[videoRuntime.mode_param || "generation_mode"] || "text");
      if (isMiniMaxH3) {
        if (h3Mode === "first_frame" && imageInputs.length < 1) {
          update(id, { status: "failed", error: t("canvas.node.firstFrameRequired") });
          return;
        }
        if (h3Mode === "last_frame" && imageInputs.length < 1) {
          update(id, { status: "failed", error: t("canvas.node.lastFrameRequired") });
          return;
        }
        if (h3Mode === "first_last" && imageInputs.length < 2) {
          update(id, { status: "failed", error: t("canvas.node.firstLastFramesRequired") });
          return;
        }
        if (h3Mode === "reference" && imageInputs.length === 0 && videoInputs.length === 0) {
          update(id, { status: "failed", error: t("canvas.node.referenceVisualRequired") });
          return;
        }
        baseParams[videoRuntime.mode_param || "generation_mode"] = h3Mode;
      }
      if (node.data.mediaKind === "audio" && storyRole === "narration") {
        const parsedPlan = parseStorySpeechPlan(prompt);
        if (parsedPlan.items.length === 0) {
          update(id, { status: "failed", progress: 0, error: t("canvas.story.narrationPlanEmpty") });
          return;
        }
        const voiceConfig = assignStoryVoices(parsedPlan.items, selectedModel, baseParams, node.data.storyVoiceOverrides || {});
        const warnings = [
          parsedPlan.fallback ? t("canvas.story.narrationPlanFallback") : "",
          voiceConfig.degraded ? t("canvas.story.voiceFallback") : "",
        ].filter(Boolean);
        const outputURLs: string[] = [];
        const taskNos: string[] = [];
        let estimatedCost = 0;
        let actualCost = 0;
        update(id, {
          status: "running",
          progress: 8,
          progressStage: "canvas.progress.audio",
          warning: warnings.join(" "),
          storySpeechPlan: parsedPlan.items,
          storyVoiceAssignments: voiceConfig.assignments,
          outputUrls: [],
          taskNos: [],
        });
        for (let itemIndex = 0; itemIndex < parsedPlan.items.length; itemIndex += 1) {
          const speech = parsedPlan.items[itemIndex];
          const itemParams: Record<string, unknown> = { ...baseParams, user_prompt: speech.text };
          const assignedVoice = voiceConfig.assignments[speech.speaker_code];
          if (voiceConfig.key && assignedVoice) itemParams[voiceConfig.key] = assignedVoice;
          const itemTaskParams = {
            ...buildAudioTaskParams(
              itemParams,
              speech.text,
              String(itemParams[audioRuntime.secondary_prompt_key || "style_prompt"] || speech.voice_hint || ""),
              selectedModel.runtime_rule
            ),
            user_prompt: speech.text,
          };
          let speechTask = await api<TaskResult>("/api/tasks", {
            method: "POST",
            body: JSON.stringify({ model_code: modelCode, prompt: speech.text, params: itemTaskParams }),
          });
          if (speechTask.task_no) taskNos.push(speechTask.task_no);
          estimatedCost += Number(speechTask.estimated_cost || 0);
          if (speechTask.status === "failed") {
            update(id, {
              status: "failed",
              progress: 0,
              taskNo: taskNos[0] || "",
              taskNos,
              outputUrls: outputURLs,
              error: t("canvas.story.speechFailed", { index: itemIndex + 1, reason: speechTask.error_message || t("canvas.generationFailed") }),
            });
            return;
          }
          let finished = false;
          for (let attempt = 0; attempt < 240; attempt += 1) {
            if (!["succeeded", "failed", "cancelled"].includes(speechTask.status)) {
              await wait(2500);
              speechTask = await api<TaskResult>(`/api/tasks/${speechTask.task_no}`);
            }
            const itemProgress = speechTask.status === "succeeded" ? 100 : runningProgress(speechTask.progress, attempt);
            update(id, {
              status: "running",
              progress: Math.min(96, Math.round(8 + 88 * ((itemIndex + itemProgress / 100) / parsedPlan.items.length))),
              progressStage: "canvas.progress.audio",
              taskNo: taskNos[0] || speechTask.task_no,
              taskNos,
            });
            if (speechTask.status === "succeeded") {
              const audioURL = extractMedia(speechTask.output, "audio");
              if (!audioURL) {
                update(id, { status: "failed", progress: 0, error: t("canvas.noMediaResult") });
                return;
              }
              outputURLs.push(audioURL);
              actualCost += Number(speechTask.actual_cost || speechTask.estimated_cost || 0);
              finished = true;
              break;
            }
            if (["failed", "cancelled"].includes(speechTask.status)) {
              update(id, {
                status: "failed",
                progress: 0,
                taskNo: taskNos[0] || speechTask.task_no,
                taskNos,
                outputUrls: outputURLs,
                error: t("canvas.story.speechFailed", { index: itemIndex + 1, reason: speechTask.error_message || t("canvas.generationFailed") }),
              });
              return;
            }
          }
          if (!finished) {
            update(id, { status: "failed", progress: 0, error: t("canvas.generationTimeout") });
            return;
          }
        }
        update(id, {
          status: "succeeded",
          progress: 100,
          progressStage: "canvas.progress.completed",
          outputUrl: outputURLs[0] || "",
          outputUrls: outputURLs,
          outputKind: "audio",
          taskNo: taskNos[0] || "",
          taskNos,
          estimatedCost,
          actualCost,
          error: "",
          dirty: false,
          lastRunSignature: runSignature,
        });
        return;
      }
      const h3FirstFrame = isMiniMaxH3 && (h3Mode === "first_frame" || h3Mode === "first_last")
        ? { url: imageInputs[0], name: imageInputs[0] }
        : null;
      const h3LastFrame = isMiniMaxH3 && h3Mode === "last_frame"
        ? { url: imageInputs[0], name: imageInputs[0] }
        : isMiniMaxH3 && h3Mode === "first_last"
          ? { url: imageInputs[1], name: imageInputs[1] }
          : null;
      const taskParams =
        node.data.mediaKind === "video"
          ? {
              ...buildVideoTaskParams(
                baseParams,
                {
                  reference_images: isMiniMaxH3 && h3Mode !== "reference"
                    ? []
                    : imageInputs.map((url) => ({ url, name: url })),
                  reference_videos: videoInputs.map((url) => ({ url, name: url })),
                  reference_audios: audioInputs.map((url) => ({ url, name: url })),
                  first_frame: h3FirstFrame,
                  last_frame: h3LastFrame,
                },
                selectedModel.runtime_rule
              ),
              user_prompt: prompt,
            }
          : node.data.mediaKind === "audio"
            ? {
                ...buildAudioTaskParams(
                  baseParams,
                  prompt,
                  String(baseParams[audioRuntime.secondary_prompt_key || "style_prompt"] || ""),
                  selectedModel.runtime_rule
                ),
                ...(audioInputs.length ? { reference_audio: audioInputs[0], reference_audios: audioInputs } : {}),
                user_prompt: prompt,
              }
            : {
                ...baseParams,
                ...(imageInputs.length ? { reference_images: imageInputs, image_url: imageInputs[0] } : {}),
              };
      let task = await api<TaskResult>("/api/tasks", {
        method: "POST",
        body: JSON.stringify({
          model_code: modelCode,
          prompt,
          params: taskParams,
        }),
      });
      update(id, {
        taskNo: task.task_no,
        status: task.status === "failed" ? "failed" : "running",
        progress: task.status === "failed" ? 0 : Math.max(12, Number(task.progress || 0)),
        progressStage: task.status === "failed" ? "canvas.progress.preparing" : "canvas.progress.queued",
        error: task.error_message || "",
        estimatedCost: Number(task.estimated_cost || 0),
        actualCost: Number(task.actual_cost || 0),
      });
      if (task.status === "failed") return;
      for (let attempt = 0; attempt < 240; attempt += 1) {
        await wait(2500);
        task = await api<TaskResult>(`/api/tasks/${task.task_no}`);
        const progress = runningProgress(task.progress, attempt);
        if (!["succeeded", "failed", "cancelled"].includes(task.status)) {
          update(id, {
            status: "running",
            progress,
            progressStage: progress >= 90 ? "canvas.progress.finalizing" : node.data.mediaKind === "image"
              ? "canvas.progress.image"
              : node.data.mediaKind === "video"
                ? "canvas.progress.video"
                : "canvas.progress.audio",
          });
        }
        if (task.status === "succeeded") {
          const mediaKind = (node.data.mediaKind || "image") as GeneratorKind;
          const outputUrl = extractMedia(task.output, mediaKind);
          update(id, {
            status: outputUrl ? "succeeded" : "failed",
            progress: outputUrl ? 100 : 0,
            progressStage: "canvas.progress.completed",
            outputUrl,
            outputKind: mediaKind,
            error: outputUrl ? "" : t("canvas.noMediaResult"),
            estimatedCost: Number(task.estimated_cost || 0),
            actualCost: Number(task.actual_cost || task.estimated_cost || 0),
            dirty: !outputUrl,
            lastRunSignature: outputUrl ? runSignature : node.data.lastRunSignature,
          });
          return;
        }
        if (["failed", "cancelled"].includes(task.status)) {
          update(id, { status: "failed", progress: 0, error: task.error_message || t("canvas.generationFailed") });
          return;
        }
      }
      update(id, { status: "failed", progress: 0, error: t("canvas.generationTimeout") });
    } catch (error) {
      update(id, { status: "failed", progress: 0, error: error instanceof Error ? error.message : t("canvas.generationFailed") });
    }
  }, [authenticated, audioModels, chatModels, imageModels, runCompositor, t, update, videoModels]);

  const executeNodes = useCallback(async (scope?: Set<string>) => {
    if (executionActiveRef.current) {
      stopExecutionRef.current = true;
      setNotice(t("canvas.executionStopping"));
      return;
    }
    const ordered = orderedGeneratorNodes(nodesRef.current, edgesRef.current)
      .filter((node) => !scope || scope.has(node.id));
    if (ordered.length === 0) {
      setNotice(t("canvas.noExecutableNodes"));
      return;
    }
    if (!authenticated) {
      setNotice(t("canvas.loginRequiredToRun"));
      return;
    }
    const nodeIDs = new Set(nodesRef.current.map((node) => node.id));
    if (edgesRef.current.some((edge) => !nodeIDs.has(edge.source) || !nodeIDs.has(edge.target))) {
      setNotice(t("canvas.invalidConnection"));
      return;
    }
    if (hasGraphCycle(nodesRef.current, edgesRef.current)) {
      setNotice(t("canvas.cycleNotAllowed"));
      return;
    }
    const availableModelCodes = new Set([...chatModels, ...imageModels, ...videoModels, ...audioModels].map((model) => model.code));
    const missingModel = ordered.find((node) =>
      node.type === "generator"
      && (!node.data.modelCode || !availableModelCodes.has(String(node.data.modelCode)))
    );
    if (missingModel) {
      update(missingModel.id, { status: "failed", dirty: true, error: t("canvas.selectModelFirst") });
      setNotice(t("canvas.nodeNeedsModel", { name: missingModel.data.label || missingModel.id }));
      return;
    }
    const modelsByCode = new Map([...chatModels, ...imageModels, ...videoModels, ...audioModels].map((model) => [model.code, model]));
    const missingPrompt = ordered.find((node) => {
      if (node.type !== "generator") return false;
      const model = modelsByCode.get(String(node.data.modelCode || ""));
      if (!model) return false;
      const upstream = collectUpstreamNodes(node.id, nodesRef.current, edgesRef.current);
      const prompt = [
        String(node.data.prompt || "").trim(),
        ...upstream.flatMap((item) => [String(item.data.outputText || "").trim(), String(item.data.prompt || "").trim()]),
      ].some(Boolean);
      const imageAvailable = Boolean(
        (node.data.referenceImageUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "image"
          || item.data.outputKind === "image"
          || Boolean((item.data.referenceImageUrls as unknown[] | undefined)?.length)
        )
      );
      const videoAvailable = Boolean(
        (node.data.referenceVideoUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "video"
          || item.data.outputKind === "video"
          || Boolean((item.data.referenceVideoUrls as unknown[] | undefined)?.length)
        )
      );
      const audioAvailable = Boolean(
        (node.data.referenceAudioUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "audio"
          || item.data.outputKind === "audio"
          || Boolean((item.data.referenceAudioUrls as unknown[] | undefined)?.length)
        )
      );
      if (node.data.mediaKind === "video") {
        const runtime = parseVideoRuntime(model.runtime_rule);
        const seedanceMaterialOnly = runtime.upload_profile === "seedance_2" && (imageAvailable || videoAvailable || audioAvailable);
        return runtime.prompt_required !== false && !prompt && !seedanceMaterialOnly;
      }
      if (node.data.mediaKind === "audio") {
        return parseAudioRuntime(model.runtime_rule).prompt_required !== false && !prompt;
      }
      return !prompt;
    });
    if (missingPrompt) {
      update(missingPrompt.id, { status: "failed", dirty: true, error: t("canvas.enterOrConnectPrompt") });
      setNotice(t("canvas.nodeInvalid", { name: missingPrompt.data.label || missingPrompt.id, reason: t("canvas.enterOrConnectPrompt") }));
      return;
    }
    const invalidViralAnalysis = ordered
      .filter((node) => node.data.viralRole === "analysis")
      .map((node) => {
        const groupID = String(node.data.viralGroupID || "");
        const group = nodesRef.current.filter((item) => item.data.viralGroupID === groupID);
        const reference = group.find((item) => item.data.viralRole === "reference");
        const brand = group.find((item) => item.data.viralRole === "brand");
        const hasAssets = (item?: CanvasNode) => Boolean(
          item?.data.assetUrl
          || (Array.isArray(item?.data.assetUrls) && item.data.assetUrls.length > 0)
        );
        return !hasAssets(reference)
          ? { node, reason: t("canvas.viral.referenceRequired") }
          : !hasAssets(brand)
            ? { node, reason: t("canvas.viral.brandRequired") }
            : null;
      })
      .find((item): item is { node: CanvasNode; reason: string } => Boolean(item));
    if (invalidViralAnalysis) {
      update(invalidViralAnalysis.node.id, { status: "failed", dirty: true, error: invalidViralAnalysis.reason });
      setNotice(t("canvas.nodeInvalid", {
        name: invalidViralAnalysis.node.data.label || invalidViralAnalysis.node.id,
        reason: invalidViralAnalysis.reason,
      }));
      return;
    }
    const invalidReferenceAudio = ordered.map((node) => {
      if (node.type !== "generator" || node.data.mediaKind !== "video") return null;
      const model = modelsByCode.get(String(node.data.modelCode || ""));
      if (!model) return null;
      const profile = String(parseVideoRuntime(model.runtime_rule).upload_profile || "");
      if (!["seedance_2", "minimax_h3"].includes(profile)) return null;
      const upstream = collectUpstreamNodes(node.id, nodesRef.current, edgesRef.current);
      const imageAvailable = Boolean(
        (node.data.referenceImageUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "image"
          || item.data.outputKind === "image"
          || Boolean((item.data.referenceImageUrls as unknown[] | undefined)?.length)
        )
      );
      const videoAvailable = Boolean(
        (node.data.referenceVideoUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "video"
          || item.data.outputKind === "video"
          || Boolean((item.data.referenceVideoUrls as unknown[] | undefined)?.length)
        )
      );
      const audioAvailable = Boolean(
        (node.data.referenceAudioUrls as unknown[] | undefined)?.length
        || upstream.some((item) =>
          item.data.mediaKind === "audio"
          || item.data.outputKind === "audio"
          || Boolean((item.data.referenceAudioUrls as unknown[] | undefined)?.length)
        )
      );
      if (!audioAvailable || imageAvailable || videoAvailable) return null;
      return {
        node,
        errorKey: profile === "minimax_h3"
          ? "canvas.node.referenceVisualRequired"
          : "canvas.node.seedanceAudioNeedsVisual",
      };
    }).find((item): item is { node: CanvasNode; errorKey: string } => Boolean(item));
    if (invalidReferenceAudio) {
      const reason = t(invalidReferenceAudio.errorKey);
      update(invalidReferenceAudio.node.id, { status: "failed", dirty: true, error: reason });
      setNotice(t("canvas.nodeInvalid", {
        name: invalidReferenceAudio.node.data.label || invalidReferenceAudio.node.id,
        reason,
      }));
      return;
    }
    const invalidCompositor = ordered
      .filter((node) => node.type === "compositor")
      .map((node) => ({ node, errorKey: validateCompositorNode(node, nodesRef.current, edgesRef.current) }))
      .find((item) => Boolean(item.errorKey));
    if (invalidCompositor) {
      const message = t(invalidCompositor.errorKey);
      update(invalidCompositor.node.id, { status: "failed", dirty: true, error: message });
      setNotice(message);
      return;
    }
    stopExecutionRef.current = false;
    executionActiveRef.current = true;
    setRunningAll(true);
    setExecutionProgress({ current: 0, total: ordered.length });
    setNotice("");
    let executed = 0;
    let reused = 0;
    let blocked = 0;
    let failed = 0;
    try {
      for (let index = 0; index < ordered.length; index += 1) {
        if (stopExecutionRef.current) break;
        const snapshot = nodesRef.current.find((item) => item.id === ordered[index].id);
        if (!snapshot) continue;
        setExecutionProgress({ current: index + 1, total: ordered.length });
        const directUpstream = edgesRef.current
          .filter((edge) => edge.target === snapshot.id)
          .map((edge) => nodesRef.current.find((item) => item.id === edge.source))
          .filter((item): item is CanvasNode => Boolean(item));
        const unavailableDependency = directUpstream.find((item) =>
          (item.type === "generator" || item.type === "compositor")
          && (item.data.status !== "succeeded" || !(item.data.mediaKind === "text" ? item.data.outputText : item.data.outputUrl))
        );
        if (unavailableDependency) {
          blocked += 1;
          update(snapshot.id, {
            status: "blocked",
            dirty: true,
            error: t("canvas.upstreamFailed", { name: unavailableDependency.data.label || unavailableDependency.id }),
          });
          continue;
        }
        const signature = nodeRunSignature(snapshot.id, nodesRef.current, edgesRef.current);
        if (
          !snapshot.data.dirty
          && snapshot.data.status === "succeeded"
          && Boolean(snapshot.data.mediaKind === "text" ? snapshot.data.outputText : snapshot.data.outputUrl)
          && snapshot.data.lastRunSignature === signature
        ) {
          reused += 1;
          continue;
        }
        await run(snapshot.id);
        const completed = nodesRef.current.find((item) => item.id === snapshot.id);
        if (completed?.data.status === "succeeded") executed += 1;
        else failed += 1;
      }
      if (stopExecutionRef.current) {
        setNotice(t("canvas.executionStopped"));
      } else if (blocked > 0 || failed > 0) {
        setNotice(t("canvas.executionFinishedWithBlocked", { executed, reused, failed, blocked }));
      } else {
        setNotice(t("canvas.executionFinished", { executed, reused }));
      }
    } finally {
      setRunningAll(false);
      setExecutionProgress({ current: 0, total: 0 });
      stopExecutionRef.current = false;
      executionActiveRef.current = false;
    }
  }, [authenticated, audioModels, chatModels, imageModels, run, t, update, videoModels]);

  const runOnly = useCallback(async (id: string) => {
    update(id, { dirty: true, status: "idle", error: "" });
    await executeNodes(new Set([id]));
  }, [executeNodes, update]);

  const runFrom = useCallback(async (id: string) => {
    markDirtyFrom(id);
    await executeNodes(new Set([id, ...collectDownstreamIDs(id, edgesRef.current)]));
  }, [executeNodes, markDirtyFrom]);

  const configureStory = useCallback((id: string, requestedCount: number, requestedDuration: number, requestedNarrationMode?: StoryNarrationMode) => {
    const selectedNode = nodesRef.current.find((node) => node.id === id);
    const groupID = String(selectedNode?.data.storyGroupID || "");
    const inputNode = nodesRef.current.find((node) => node.data.storyGroupID === groupID && node.data.storyRole === "input");
    if (!selectedNode || !inputNode || !groupID) return;

    const segmentCount = STORY_SEGMENT_COUNT_OPTIONS.includes(requestedCount as (typeof STORY_SEGMENT_COUNT_OPTIONS)[number])
      ? requestedCount
      : 4;
    const groupNodes = nodesRef.current.filter((node) => node.data.storyGroupID === groupID);
    const scriptNode = groupNodes.find((node) => node.data.storyRole === "script");
    const narrationTextNode = groupNodes.find((node) => node.data.storyRole === "narrationText");
    const narrationNode = groupNodes.find((node) => node.data.storyRole === "narration");
    const finalNode = groupNodes.find((node) => node.data.storyRole === "final");
    if (!scriptNode || !narrationTextNode || !narrationNode || !finalNode) return;

    const existingKeyframes = new Map(
      groupNodes
        .filter((node) => node.data.storyRole === "keyframe")
        .map((node) => [Number(node.data.storySegmentIndex || 0), node])
    );
    const existingVideos = new Map(
      groupNodes
        .filter((node) => node.data.storyRole === "video")
        .map((node) => [Number(node.data.storySegmentIndex || 0), node])
    );
    const existingVideoModel = videoModels.find((model) =>
      groupNodes.some((node) => node.data.storyRole === "video" && node.data.modelCode === model.code)
    ) || preferredVideoModel(videoModels);
    const durationOptions = storyDurationOptions(existingVideoModel);
    const segmentDuration = durationOptions.includes(requestedDuration)
      ? requestedDuration
      : preferredStoryDuration(existingVideoModel);
    const narrationMode = normalizeStoryNarrationMode(requestedNarrationMode || inputNode.data.storyNarrationMode);
    const narrationModeLabel = t(`canvas.story.narrationMode.${narrationMode}`);
    const narrationInstruction = t(`canvas.story.narrationInstruction.${narrationMode}`);
    const baseX = inputNode.position.x;
    const baseY = inputNode.position.y;
    const branchGap = 420;
    const sharedPatch = { storySegmentCount: segmentCount, storySegmentDuration: segmentDuration, storyNarrationMode: narrationMode };
    const resetInput: CanvasNode = {
      ...inputNode,
      data: {
        ...inputNode.data,
        ...sharedPatch,
        storyDurationOptions: durationOptions.length ? durationOptions : [segmentDuration],
      },
    };
    const resetScript = storyNodeNeedsReset(scriptNode, {
      ...sharedPatch,
      prompt: t("canvas.story.scriptPrompt", { count: segmentCount, duration: segmentDuration, mode: narrationModeLabel, instruction: narrationInstruction }),
    });
    resetScript.position = { x: baseX + 400, y: baseY };
    const resetNarrationText = storyNodeNeedsReset(narrationTextNode, {
      ...sharedPatch,
      prompt: t("canvas.story.narrationTextPrompt", {
        count: segmentCount,
        duration: segmentDuration,
        total: segmentCount * segmentDuration,
        mode: narrationModeLabel,
        instruction: narrationInstruction,
      }),
    });
    resetNarrationText.position = { x: baseX + 800, y: baseY + segmentCount * branchGap + 40 };
    const resetNarration = storyNodeNeedsReset(narrationNode, {
      ...sharedPatch,
      // TTS 上游会直接朗读 prompt；旁白节点只应朗读前一节点整理出的正文。
      prompt: "",
    });
    resetNarration.position = { x: baseX + 1200, y: baseY + segmentCount * branchGap + 40 };
    const resetFinal = storyNodeNeedsReset(finalNode, {
      ...sharedPatch,
      composeMode: "auto",
    });
    resetFinal.position = { x: baseX + 1640, y: baseY + Math.max(120, (segmentCount - 1) * branchGap / 2) };

    const defaultImageModel = imageModels[0];
    const defaultVideoModel = existingVideoModel;
    const keyframes: CanvasNode[] = [];
    const videos: CanvasNode[] = [];
    for (let index = 1; index <= segmentCount; index += 1) {
      const existingKeyframe = existingKeyframes.get(index);
      const keyframe = storyNodeNeedsReset(
        existingKeyframe || {
          id: newNodeID(),
          type: "generator",
          position: { x: baseX + 800, y: baseY + (index - 1) * branchGap },
          data: {
            label: "",
            mediaKind: "image",
            modelCode: defaultImageModel?.code || "",
            params: canvasModelDefaults("image", defaultImageModel),
            status: "idle",
          },
        },
        {
          label: t("canvas.node.storyKeyframeIndexed", { index, count: segmentCount }),
          mediaKind: "image",
          storyGroupID: groupID,
          storyRole: "keyframe",
          storySegmentIndex: index,
          ...sharedPatch,
          prompt: t("canvas.story.keyframePrompt", { index, count: segmentCount }),
        }
      );
      keyframe.position = { x: baseX + 800, y: baseY + (index - 1) * branchGap };
      keyframes.push(keyframe);

      const existingVideo = existingVideos.get(index);
      const selectedVideoModel = videoModels.find((model) => model.code === existingVideo?.data.modelCode) || defaultVideoModel;
      const videoParams = normalizeCanvasParamsForModel(
        {
          ...canvasModelDefaults("video", selectedVideoModel),
          ...(existingVideo?.data.params || {}),
          duration: segmentDuration,
        },
        selectedVideoModel?.input_schema,
        selectedVideoModel?.default_params
      );
      const video = storyNodeNeedsReset(
        existingVideo || {
          id: newNodeID(),
          type: "generator",
          position: { x: baseX + 1200, y: baseY + (index - 1) * branchGap },
          data: {
            label: "",
            mediaKind: "video",
            modelCode: selectedVideoModel?.code || "",
            params: videoParams,
            status: "idle",
          },
        },
        {
          label: t("canvas.node.storyVideoIndexed", { index, count: segmentCount }),
          mediaKind: "video",
          modelCode: selectedVideoModel?.code || "",
          params: videoParams,
          storyGroupID: groupID,
          storyRole: "video",
          storySegmentIndex: index,
          ...sharedPatch,
          prompt: t("canvas.story.videoPrompt", { index, count: segmentCount, duration: segmentDuration }),
        }
      );
      video.position = { x: baseX + 1200, y: baseY + (index - 1) * branchGap };
      videos.push(video);
    }

    const groupNodeIDs = new Set(groupNodes.map((node) => node.id));
    const preservedGroupIDs = new Set([
      resetInput.id,
      resetScript.id,
      resetNarrationText.id,
      resetNarration.id,
      resetFinal.id,
      ...keyframes.map((node) => node.id),
      ...videos.map((node) => node.id),
    ]);
    const unrelatedNodes = nodesRef.current.filter((node) => node.data.storyGroupID !== groupID);
    const retainedExternalEdges = edgesRef.current.filter((edge) => {
      const sourceInGroup = groupNodeIDs.has(edge.source);
      const targetInGroup = groupNodeIDs.has(edge.target);
      if (sourceInGroup && targetInGroup) return false;
      if (sourceInGroup && !preservedGroupIDs.has(edge.source)) return false;
      if (targetInGroup && !preservedGroupIDs.has(edge.target)) return false;
      return true;
    });
    const connectStory = (source: CanvasNode, target: CanvasNode): CanvasEdge => ({
      id: `edge_${crypto.randomUUID()}`,
      source: source.id,
      target: target.id,
      type: "smoothstep",
      animated: true,
      style: { stroke: "#22d3ee", strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: "#22d3ee" },
    });
    const internalEdges: CanvasEdge[] = [
      connectStory(resetInput, resetScript),
      connectStory(resetScript, resetNarrationText),
      connectStory(resetNarrationText, resetNarration),
    ];
    keyframes.forEach((keyframe, index) => {
      internalEdges.push(connectStory(index === 0 ? resetScript : keyframes[index - 1], keyframe));
      internalEdges.push(connectStory(keyframe, videos[index]));
      internalEdges.push(connectStory(videos[index], resetFinal));
    });
    internalEdges.push(connectStory(resetNarration, resetFinal));

    nodesRef.current = [
      ...unrelatedNodes,
      resetInput,
      resetScript,
      ...keyframes,
      ...videos,
      resetNarrationText,
      resetNarration,
      resetFinal,
    ];
    edgesRef.current = [...retainedExternalEdges, ...internalEdges];
    setNodes(nodesRef.current);
    setEdges(edgesRef.current);
    setNotice(t("canvas.story.structureUpdated", {
      count: segmentCount,
      duration: segmentDuration,
      total: segmentCount * segmentDuration,
    }));
  }, [imageModels, setEdges, setNodes, t, videoModels]);

  const configureViral = useCallback((id: string, requestedCount: number, requestedDuration: number) => {
    const briefNode = nodesRef.current.find((node) => node.id === id && node.data.viralRole === "brief");
    const groupID = String(briefNode?.data.viralGroupID || "");
    if (!briefNode || !groupID) return;
    const segmentCount = VIRAL_SEGMENT_COUNT_OPTIONS.includes(requestedCount as (typeof VIRAL_SEGMENT_COUNT_OPTIONS)[number])
      ? requestedCount
      : 3;
    const groupNodes = nodesRef.current.filter((node) => node.data.viralGroupID === groupID);
    const referenceNode = groupNodes.find((node) => node.data.viralRole === "reference");
    const brandNode = groupNodes.find((node) => node.data.viralRole === "brand");
    const analysisNode = groupNodes.find((node) => node.data.viralRole === "analysis");
    const finalNode = groupNodes.find((node) => node.data.viralRole === "final");
    if (!referenceNode || !brandNode || !analysisNode || !finalNode) return;

    const existingKeyframes = new Map(
      groupNodes.filter((node) => node.data.viralRole === "keyframe").map((node) => [Number(node.data.viralSegmentIndex || 0), node])
    );
    const existingVideos = new Map(
      groupNodes.filter((node) => node.data.viralRole === "video").map((node) => [Number(node.data.viralSegmentIndex || 0), node])
    );
    const selectedVideoModel = videoModels.find((model) =>
      groupNodes.some((node) => node.data.viralRole === "video" && node.data.modelCode === model.code)
    ) || preferredVideoModel(videoModels);
    const durationOptions = storyDurationOptions(selectedVideoModel);
    const segmentDuration = durationOptions.includes(requestedDuration)
      ? requestedDuration
      : preferredStoryDuration(selectedVideoModel);
    const sharedPatch = { viralSegmentCount: segmentCount, viralSegmentDuration: segmentDuration };
    const baseX = briefNode.position.x;
    const baseY = briefNode.position.y;
    const gap = 430;
    const resetBrief: CanvasNode = {
      ...briefNode,
      data: {
        ...briefNode.data,
        ...sharedPatch,
        viralDurationOptions: durationOptions.length ? durationOptions : [segmentDuration],
      },
    };
    resetBrief.position = { x: baseX, y: baseY };
    const resetReference = { ...referenceNode, position: { x: baseX, y: baseY + 330 } };
    const resetBrand = { ...brandNode, position: { x: baseX, y: baseY + 660 } };
    const resetAnalysis = storyNodeNeedsReset(analysisNode, {
      ...sharedPatch,
      prompt: t("canvas.viral.analysisPrompt", { count: segmentCount, duration: segmentDuration }),
    });
    resetAnalysis.position = { x: baseX + 400, y: baseY + 220 };
    const resetFinal = storyNodeNeedsReset(finalNode, {
      ...sharedPatch,
      composeMode: "concat",
      outputSize: "keep",
    });
    resetFinal.position = { x: baseX + 1640, y: baseY + Math.max(160, (segmentCount - 1) * gap / 2) };

    const defaultImageModel = imageModels[0];
    const keyframes: CanvasNode[] = [];
    const videos: CanvasNode[] = [];
    for (let index = 1; index <= segmentCount; index += 1) {
      const oldKeyframe = existingKeyframes.get(index);
      const imageModel = imageModels.find((model) => model.code === oldKeyframe?.data.modelCode) || defaultImageModel;
      const imageParams = aspectRatioParams(
        imageModel,
        { ...(imageModel?.default_params || {}), ...(oldKeyframe?.data.params || {}) },
        "9:16"
      );
      const keyframe = storyNodeNeedsReset(
        oldKeyframe || {
          id: newNodeID(),
          type: "generator",
          position: { x: baseX + 800, y: baseY + (index - 1) * gap },
          data: { label: "", mediaKind: "image", status: "idle" },
        },
        {
          label: t("canvas.viral.keyframeIndexed", { index, count: segmentCount }),
          mediaKind: "image",
          modelCode: imageModel?.code || "",
          params: imageParams,
          viralGroupID: groupID,
          viralRole: "keyframe",
          viralSegmentIndex: index,
          ...sharedPatch,
          prompt: t("canvas.viral.keyframePrompt", { index, count: segmentCount }),
          referenceImageLabel: t("canvas.node.brandMaterial"),
        }
      );
      keyframe.position = { x: baseX + 800, y: baseY + (index - 1) * gap };
      keyframes.push(keyframe);

      const oldVideo = existingVideos.get(index);
      const videoModel = videoModels.find((model) => model.code === oldVideo?.data.modelCode) || selectedVideoModel;
      const videoParams = aspectRatioParams(
        videoModel,
        normalizeCanvasParamsForModel(
          {
            ...canvasModelDefaults("video", videoModel),
            ...(oldVideo?.data.params || {}),
            duration: segmentDuration,
          },
          videoModel?.input_schema,
          videoModel?.default_params
        ),
        "9:16"
      );
      const video = storyNodeNeedsReset(
        oldVideo || {
          id: newNodeID(),
          type: "generator",
          position: { x: baseX + 1200, y: baseY + (index - 1) * gap },
          data: { label: "", mediaKind: "video", status: "idle" },
        },
        {
          label: t("canvas.viral.videoIndexed", { index, count: segmentCount }),
          mediaKind: "video",
          modelCode: videoModel?.code || "",
          params: videoParams,
          viralGroupID: groupID,
          viralRole: "video",
          viralSegmentIndex: index,
          ...sharedPatch,
          prompt: t("canvas.viral.videoPrompt", { index, count: segmentCount, duration: segmentDuration }),
          referenceImageLabel: t("canvas.node.avatarAndFirstFrame"),
        }
      );
      video.position = { x: baseX + 1200, y: baseY + (index - 1) * gap };
      videos.push(video);
    }

    const groupNodeIDs = new Set(groupNodes.map((node) => node.id));
    const preservedGroupIDs = new Set([
      resetBrief.id,
      resetReference.id,
      resetBrand.id,
      resetAnalysis.id,
      resetFinal.id,
      ...keyframes.map((node) => node.id),
      ...videos.map((node) => node.id),
    ]);
    const unrelatedNodes = nodesRef.current.filter((node) => node.data.viralGroupID !== groupID);
    const retainedExternalEdges = edgesRef.current.filter((edge) => {
      const sourceInGroup = groupNodeIDs.has(edge.source);
      const targetInGroup = groupNodeIDs.has(edge.target);
      if (sourceInGroup && targetInGroup) return false;
      if (sourceInGroup && !preservedGroupIDs.has(edge.source)) return false;
      if (targetInGroup && !preservedGroupIDs.has(edge.target)) return false;
      return true;
    });
    const connectViral = (source: CanvasNode, target: CanvasNode): CanvasEdge => ({
      id: `edge_${crypto.randomUUID()}`,
      source: source.id,
      target: target.id,
      type: "smoothstep",
      animated: true,
      style: { stroke: "#f97316", strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: "#f97316" },
    });
    const internalEdges: CanvasEdge[] = [
      connectViral(resetBrief, resetAnalysis),
      connectViral(resetReference, resetAnalysis),
      connectViral(resetBrand, resetAnalysis),
    ];
    keyframes.forEach((keyframe, index) => {
      internalEdges.push(connectViral(resetAnalysis, keyframe));
      internalEdges.push(connectViral(resetBrand, keyframe));
      if (index > 0) internalEdges.push(connectViral(keyframes[index - 1], keyframe));
      internalEdges.push(connectViral(resetAnalysis, videos[index]));
      internalEdges.push(connectViral(keyframe, videos[index]));
      internalEdges.push(connectViral(videos[index], resetFinal));
    });
    nodesRef.current = [
      ...unrelatedNodes,
      resetBrief,
      resetReference,
      resetBrand,
      resetAnalysis,
      ...keyframes,
      ...videos,
      resetFinal,
    ];
    edgesRef.current = [...retainedExternalEdges, ...internalEdges];
    setNodes(nodesRef.current);
    setEdges(edgesRef.current);
    setNotice(t("canvas.viral.structureUpdated", {
      count: segmentCount,
      duration: segmentDuration,
      total: segmentCount * segmentDuration,
    }));
  }, [imageModels, setEdges, setNodes, t, videoModels]);

  const openOutputMenu = useCallback((sourceID: string, point: { x: number; y: number }) => {
    const bounds = editorRef.current?.getBoundingClientRect();
    if (!bounds) return;
    const flowPoint = screenToFlowPosition(point);
    const menuWidth = 216;
    const menuHeight = 238;
    setOutputMenu({
      sourceID,
      left: Math.max(12, Math.min(bounds.width - menuWidth - 12, point.x - bounds.left + 18)),
      top: Math.max(12, Math.min(bounds.height - menuHeight - 12, point.y - bounds.top - 32)),
      nodePosition: { x: flowPoint.x + 72, y: flowPoint.y - 48 },
    });
  }, [screenToFlowPosition]);

  const actions = useMemo<NodeActions>(
    () => ({
      chatModels,
      imageModels,
      videoModels,
      audioModels,
      update,
      remove,
      run: runOnly,
      runFrom,
      upload,
      uploadReference,
      openAssetLibrary,
      openOutputMenu,
      openResultPreview: setResultPreview,
      configureStory,
      configureViral,
    }),
    [audioModels, chatModels, configureStory, configureViral, imageModels, openAssetLibrary, openOutputMenu, remove, runFrom, runOnly, update, upload, uploadReference, videoModels]
  );

  const onConnect = useCallback((connection: Connection) => {
    connectionCompletedRef.current = true;
    if (connection.source === connection.target) return;
    if (!connection.source || !connection.target) return;
    if (createsCycle(connection.source, connection.target, edgesRef.current)) {
      setNotice(t("canvas.cycleNotAllowed"));
      return;
    }
    if (edgesRef.current.some((edge) => edge.source === connection.source && edge.target === connection.target)) return;
    const next = addEdge({
      ...connection,
      type: "smoothstep",
      animated: true,
      style: { stroke: "#22d3ee", strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: "#22d3ee" },
    }, edgesRef.current);
    edgesRef.current = next;
    setEdges(next);
    markDirtyFrom(connection.target);
  }, [markDirtyFrom, setEdges, t]);

  const onConnectEnd = useCallback((event: MouseEvent | TouchEvent) => {
    const sourceID = connectionSourceRef.current;
    if (!connectionCompletedRef.current && sourceID) {
      const touch = "changedTouches" in event ? event.changedTouches[0] : undefined;
      openOutputMenu(sourceID, {
        x: touch?.clientX ?? (event as MouseEvent).clientX,
        y: touch?.clientY ?? (event as MouseEvent).clientY,
      });
    }
    connectionSourceRef.current = "";
    connectionCompletedRef.current = false;
  }, [openOutputMenu]);

  const appendSingleNode = useCallback((kind: NewNodeKind, options?: { sourceID?: string; position?: { x: number; y: number } }) => {
    const existing = nodesRef.current;
    const rightMost = existing.reduce((maximum, node) => Math.max(maximum, node.position.x), 0);
    const position = options?.position || (existing.length
      ? { x: rightMost + 340, y: 80 + (existing.length % 3) * 70 }
      : { x: 80, y: 80 });
    let node: CanvasNode;
    if (kind === "text") {
      node = {
        id: newNodeID(),
        type: "textInput",
        position,
        data: { label: title.trim() || t("canvas.node.textInput"), prompt: "" },
      };
    } else if (kind === "compositor") {
      node = {
        id: newNodeID(),
        type: "compositor",
        position,
        data: {
          label: t("canvas.node.compositor"),
          composeMode: "auto",
          outputSize: "keep",
          status: "idle",
        },
      };
    } else {
      const mediaKind: GeneratorKind =
        kind === "textGenerator" ? "text" : kind === "videoGenerator" ? "video" : kind === "audioGenerator" ? "audio" : "image";
      const defaultModel =
        mediaKind === "text" ? chatModels[0] : mediaKind === "video" ? preferredVideoModel(videoModels) : mediaKind === "audio" ? audioModels[0] : imageModels[0];
      const defaultParams = canvasModelDefaults(mediaKind, defaultModel);
      node = {
        id: newNodeID(),
        type: "generator",
        position,
        data: {
          label:
            mediaKind === "text"
              ? t("canvas.node.textGeneration")
              : mediaKind === "video"
              ? t("canvas.node.videoGeneration")
              : mediaKind === "audio"
                ? t("canvas.node.audioGeneration")
                : t("canvas.node.imageGeneration"),
          mediaKind,
          modelCode: defaultModel?.code || "",
          params: defaultParams,
          status: "idle",
        },
      };
    }
    const selected = [...existing].reverse().find((item) => item.selected);
    const previous = (options?.sourceID ? existing.find((item) => item.id === options.sourceID) : undefined)
      || selected
      || existing[existing.length - 1];
    let autoEdge: CanvasEdge | null = null;
    if (previous) {
      const source = previous.id;
      const target = node.id;
      if (!createsCycle(source, target, edgesRef.current)) {
        autoEdge = {
          id: `edge_${crypto.randomUUID()}`,
          source,
          target,
          type: "smoothstep",
          animated: true,
          style: { stroke: "#22d3ee", strokeWidth: 2 },
          markerEnd: { type: MarkerType.ArrowClosed, color: "#22d3ee" },
        };
      }
    }
    nodesRef.current = [
      ...existing.map((item) => ({ ...item, selected: false })),
      { ...node, selected: true },
    ];
    if (autoEdge) edgesRef.current = [...edgesRef.current, autoEdge];
    setNodes(nodesRef.current);
    if (autoEdge) setEdges(edgesRef.current);
    setNodePaletteOpen(false);
    setOutputMenu(null);
    setShowEmptyWelcome(false);
  }, [audioModels, chatModels, imageModels, setEdges, setNodes, t, title, videoModels]);

  const appendTemplate = useCallback((templateID: string, requestedFlowName?: string) => {
    const originX = nodesRef.current.length
      ? Math.max(...nodesRef.current.map((node) => node.position.x)) + 360
      : 80;
    const originY = 80;
    const templateDefinition = ALL_TEMPLATE_DEFINITIONS.find((item) => item.id === templateID);
    const flowName = requestedFlowName || (templateDefinition ? t(templateDefinition.titleKey) : t("canvas.node.textInput"));
    if (nodesRef.current.length === 0) {
      workflowNameRef.current = flowName;
      titleManuallyEditedRef.current = false;
      setTitle(flowName);
    }
    const text = (prompt = "", label = flowName): CanvasNode => ({
      id: newNodeID(),
      type: "textInput",
      position: { x: originX, y: originY },
      data: { label, prompt },
    });
    const generator = (kind: GeneratorKind, offsetY = 0, label?: string, column = 1): CanvasNode => {
      const models = kind === "text" ? chatModels : kind === "video" ? videoModels : kind === "audio" ? audioModels : imageModels;
      const defaultModel = kind === "video" ? preferredVideoModel(models) : models[0];
      const defaultParams = canvasModelDefaults(kind, defaultModel);
      return {
        id: newNodeID(),
        type: "generator",
        position: { x: originX + 430 * column, y: originY + offsetY },
        data: {
          label:
            label ||
            (kind === "text"
              ? t("canvas.node.textGeneration")
              : kind === "video"
              ? t("canvas.node.videoGeneration")
              : kind === "audio"
                ? t("canvas.node.audioGeneration")
                : t("canvas.node.imageGeneration")),
          mediaKind: kind,
          modelCode: defaultModel?.code || "",
          params: defaultParams,
          status: "idle",
        },
      };
    };
    const compositor = (offsetY = 0, label?: string): CanvasNode => ({
      id: newNodeID(),
      type: "compositor",
      position: { x: originX + 1290, y: originY + offsetY },
      data: {
        label: label || t("canvas.node.compositor"),
        composeMode: "auto",
        outputSize: "keep",
        status: "idle",
        dirty: true,
      },
    });
    let nextNodes: CanvasNode[] = [];
    let nextEdges: CanvasEdge[] = [];
    let storyBootstrap: { inputID: string; segmentCount: number; segmentDuration: number } | null = null;
    let viralBootstrap: { inputID: string; segmentCount: number; segmentDuration: number } | null = null;
    const connect = (source: CanvasNode, target: CanvasNode): CanvasEdge => ({
      id: `edge_${crypto.randomUUID()}`,
      source: source.id,
      target: target.id,
      type: "smoothstep",
      animated: true,
      style: { stroke: "#22d3ee", strokeWidth: 2 },
      markerEnd: { type: MarkerType.ArrowClosed, color: "#22d3ee" },
    });
    if (templateID === "text-image" || templateID === "text-video") {
      const input = text();
      const output = generator(templateID === "text-video" ? "video" : "image");
      nextNodes = [input, output];
      nextEdges = [connect(input, output)];
    } else if (templateID === "image-image") {
      const input = text(t("canvas.template.imageImagePrompt"));
      const output = generator("image");
      output.data.referenceImageLabel = t("canvas.node.sourceImages");
      nextNodes = [input, output];
      nextEdges = [connect(input, output)];
    } else if (templateID === "image-video") {
      const input = text(t("canvas.template.firstFrameVideoPrompt"));
      const output = generator("video", 0, t("canvas.node.firstFrameVideo"));
      output.data.referenceImageLabel = t("canvas.node.avatarAndFirstFrame");
      output.data.referenceVideoLabel = t("canvas.node.motionReference");
      output.data.referenceAudioLabel = t("canvas.node.referenceAudio");
      nextNodes = [input, output];
      nextEdges = [connect(input, output)];
    } else if (templateID === "text-image-mix") {
      const textNode = text();
      const copyNode = generator("text", 0, t("canvas.node.marketingCopy"));
      const output = generator("image", 0, t("canvas.node.copyIllustration"), 2);
      nextNodes = [textNode, copyNode, output];
      nextEdges = [connect(textNode, copyNode), connect(copyNode, output)];
    } else if (templateID === "ecommerce-visual-pack") {
      const textNode = text();
      const mainImage = generator("image", 0, t("canvas.node.productMainImage"));
      const detailImage = generator("image", 300, t("canvas.node.productDetailPoster"));
      mainImage.data.referenceImageLabel = t("canvas.node.productReferences");
      detailImage.data.referenceImageLabel = t("canvas.node.productReferences");
      nextNodes = [textNode, mainImage, detailImage];
      nextEdges = [
        connect(textNode, mainImage),
        connect(textNode, detailImage),
      ];
    } else if (templateID === "social-campaign") {
      const textNode = text();
      const socialImage = generator("image", 0, t("canvas.node.socialImage"));
      const socialVideo = generator("video", 300, t("canvas.node.socialVideo"));
      nextNodes = [textNode, socialImage, socialVideo];
      nextEdges = [connect(textNode, socialImage), connect(textNode, socialVideo)];
    } else if (templateID === "product-showcase-video") {
      const textNode = text();
      const keyVisual = generator("image", 100, t("canvas.node.productKeyVisual"));
      keyVisual.data.referenceImageLabel = t("canvas.node.productReferences");
      const videoNode = generator("video", 100, t("canvas.node.productVideo"), 2);
      nextNodes = [textNode, keyVisual, videoNode];
      nextEdges = [connect(textNode, keyVisual), connect(keyVisual, videoNode)];
    } else if (templateID === "brand-visual-kit") {
      const textNode = text();
      const logoNode = generator("image", 0, t("canvas.node.logoConcept"));
      const posterNode = generator("image", 300, t("canvas.node.brandPoster"));
      nextNodes = [textNode, logoNode, posterNode];
      nextEdges = [connect(textNode, logoNode), connect(textNode, posterNode)];
    } else if (templateID === "photo-restoration") {
      const textNode = text(t("canvas.template.photoRestorePrompt"));
      const restoreNode = generator("image", 0, t("canvas.node.restoredPhoto"));
      restoreNode.data.referenceImageLabel = t("canvas.node.oldPhoto");
      nextNodes = [textNode, restoreNode];
      nextEdges = [connect(textNode, restoreNode)];
    } else if (templateID === "story-short-video") {
      const storyGroupID = `story_${crypto.randomUUID()}`;
      const storyVideoModel = preferredVideoModel(videoModels);
      const durationOptions = storyDurationOptions(storyVideoModel);
      const segmentDuration = preferredStoryDuration(storyVideoModel);
      const segmentCount = 4;
      const narrationMode: StoryNarrationMode = "smart";
      const narrationModeLabel = t(`canvas.story.narrationMode.${narrationMode}`);
      const narrationInstruction = t(`canvas.story.narrationInstruction.${narrationMode}`);
      const textNode = text();
      textNode.data = {
        ...textNode.data,
        storyGroupID,
        storyRole: "input",
        storySegmentCount: segmentCount,
        storySegmentDuration: segmentDuration,
        storyDurationOptions: durationOptions.length ? durationOptions : [segmentDuration],
        storyNarrationMode: narrationMode,
      };
      const scriptNode = generator("text", 0, t("canvas.node.storyScript"));
      scriptNode.data = {
        ...scriptNode.data,
        storyGroupID,
        storyRole: "script",
        storySegmentCount: segmentCount,
        storySegmentDuration: segmentDuration,
        storyNarrationMode: narrationMode,
        prompt: t("canvas.story.scriptPrompt", { count: segmentCount, duration: segmentDuration, mode: narrationModeLabel, instruction: narrationInstruction }),
      };
      const narrationTextNode = generator("text", 300, t("canvas.node.storyNarrationText"), 2);
      narrationTextNode.data = {
        ...narrationTextNode.data,
        storyGroupID,
        storyRole: "narrationText",
        storySegmentCount: segmentCount,
        storySegmentDuration: segmentDuration,
        storyNarrationMode: narrationMode,
        prompt: t("canvas.story.narrationTextPrompt", {
          count: segmentCount,
          duration: segmentDuration,
          total: segmentCount * segmentDuration,
          mode: narrationModeLabel,
          instruction: narrationInstruction,
        }),
      };
      const narrationNode = generator("audio", 300, t("canvas.node.storyNarration"), 3);
      const narrationModel = preferredNarrationAudioModel(audioModels);
      narrationNode.data = {
        ...narrationNode.data,
        modelCode: narrationModel?.code || "",
        params: canvasModelDefaults("audio", narrationModel),
        storyGroupID,
        storyRole: "narration",
        storySegmentCount: segmentCount,
        storySegmentDuration: segmentDuration,
        storyNarrationMode: narrationMode,
        prompt: "",
      };
      const finalNode = { ...compositor(100, t("canvas.node.storyFinalVideo")), position: { x: originX + 1720, y: originY + 100 } };
      finalNode.data = {
        ...finalNode.data,
        storyGroupID,
        storyRole: "final",
        storySegmentCount: segmentCount,
        storySegmentDuration: segmentDuration,
        storyNarrationMode: narrationMode,
        composeMode: "auto",
      };
      nextNodes = [textNode, scriptNode, narrationTextNode, narrationNode, finalNode];
      nextEdges = [
        connect(textNode, scriptNode),
        connect(scriptNode, narrationTextNode),
        connect(narrationTextNode, narrationNode),
        connect(narrationNode, finalNode),
      ];
      storyBootstrap = { inputID: textNode.id, segmentCount, segmentDuration };
    } else if (templateID === "viral-remake") {
      const viralGroupID = `viral_${crypto.randomUUID()}`;
      const viralVideoModel = preferredVideoModel(videoModels);
      const durationOptions = storyDurationOptions(viralVideoModel);
      const configuredCount = Number(workspaceRuntime.default_segment_count || 3);
      const segmentCount = VIRAL_SEGMENT_COUNT_OPTIONS.includes(configuredCount as (typeof VIRAL_SEGMENT_COUNT_OPTIONS)[number])
        ? configuredCount
        : 3;
      const configuredDuration = Number(workspaceRuntime.default_segment_duration || 0);
      const segmentDuration = durationOptions.includes(configuredDuration)
        ? configuredDuration
        : preferredStoryDuration(viralVideoModel);
      const briefNode = text(t("canvas.viral.defaultBrief"), t("canvas.node.viralRemakeBrief"));
      briefNode.data = {
        ...briefNode.data,
        viralGroupID,
        viralRole: "brief",
        viralSegmentCount: segmentCount,
        viralSegmentDuration: segmentDuration,
        viralDurationOptions: durationOptions.length ? durationOptions : [segmentDuration],
      };
      const referenceNode: CanvasNode = {
        id: newNodeID(),
        type: "imageInput",
        position: { x: originX, y: originY + 330 },
        data: {
          label: t("canvas.node.viralReference"),
          prompt: t("canvas.viral.referenceNote"),
          mediaKind: "video",
          viralGroupID,
          viralRole: "reference",
        },
      };
      const brandNode: CanvasNode = {
        id: newNodeID(),
        type: "imageInput",
        position: { x: originX, y: originY + 660 },
        data: {
          label: t("canvas.node.brandMaterial"),
          prompt: t("canvas.viral.brandNote"),
          mediaKind: "image",
          viralGroupID,
          viralRole: "brand",
        },
      };
      const analysisNode = generator("text", 0, t("canvas.node.viralAnalysis"));
      const analysisModel = preferredMultimodalChatModel(chatModels);
      analysisNode.data = {
        ...analysisNode.data,
        modelCode: analysisModel?.code || "",
        params: canvasModelDefaults("text", analysisModel),
        viralGroupID,
        viralRole: "analysis",
        viralSegmentCount: segmentCount,
        viralSegmentDuration: segmentDuration,
        prompt: t("canvas.viral.analysisPrompt", { count: segmentCount, duration: segmentDuration }),
      };
      const finalNode = compositor(100, t("canvas.viral.finalVideo"));
      finalNode.data = {
        ...finalNode.data,
        viralGroupID,
        viralRole: "final",
        viralSegmentCount: segmentCount,
        viralSegmentDuration: segmentDuration,
        composeMode: "concat",
        outputSize: "keep",
      };
      nextNodes = [briefNode, referenceNode, brandNode, analysisNode, finalNode];
      nextEdges = [
        connect(briefNode, analysisNode),
        connect(referenceNode, analysisNode),
        connect(brandNode, analysisNode),
      ];
      viralBootstrap = { inputID: briefNode.id, segmentCount, segmentDuration };
    } else if (templateID === "multi-image") {
      const textNode = text(t("canvas.template.multiImagePrompt"));
      const outputA = generator("image", 0, t("canvas.node.imageOptionA"));
      const outputB = generator("image", 300, t("canvas.node.imageOptionB"));
      nextNodes = [textNode, outputA, outputB];
      nextEdges = [connect(textNode, outputA), connect(textNode, outputB)];
    } else {
      setNotice(t("canvas.unsupportedTemplate", { name: flowName }));
      return;
    }
    nodesRef.current = [...nodesRef.current, ...nextNodes];
    edgesRef.current = [...edgesRef.current, ...nextEdges];
    setNodes(nodesRef.current);
    setEdges(edgesRef.current);
    setShowEmptyWelcome(false);
    if (storyBootstrap) {
      const { inputID, segmentCount, segmentDuration } = storyBootstrap;
      window.setTimeout(() => configureStory(inputID, segmentCount, segmentDuration), 0);
    }
    if (viralBootstrap) {
      const { inputID, segmentCount, segmentDuration } = viralBootstrap;
      window.setTimeout(() => configureViral(inputID, segmentCount, segmentDuration), 0);
    }
    window.setTimeout(() => void setViewport({ x: Math.min(0, 180 - originX), y: 80, zoom: 1 }, { duration: 350 }), 50);
  }, [audioModels, chatModels, configureStory, configureViral, imageModels, setEdges, setNodes, setViewport, t, videoModels, workspaceRuntime]);

  const bootstrapInitialTemplate = useCallback(() => {
    if (!initialTemplateID) return;
    const definition = ALL_TEMPLATE_DEFINITIONS.find((item) => item.id === initialTemplateID);
    const flowName = definition ? t(definition.titleKey) : initialTemplateID;
    workflowNameRef.current = flowName;
    titleManuallyEditedRef.current = false;
    setTitle(flowName);
    appendTemplate(initialTemplateID, flowName);
  }, [appendTemplate, initialTemplateID, t]);

  useEffect(() => {
    if (!initialTemplateID || !modelCatalogReady || !workspaceConfigReady || initialTemplateAppliedRef.current) return;
    initialTemplateAppliedRef.current = true;
    bootstrapInitialTemplate();
  }, [bootstrapInitialTemplate, initialTemplateID, modelCatalogReady, workspaceConfigReady]);

  const availableTemplates = useMemo<CanvasTemplate[]>(() => {
    if (managedTemplates.length) {
      return managedTemplates.map((template) => {
        const definition = ALL_TEMPLATE_DEFINITIONS.find((item) => item.id === (template.template_id || template.id));
        const source = DEFAULT_TEMPLATE_ZH[template.id];
        if (!definition || !source || template.name !== source.name) return template;
        return {
          ...template,
          name: t(definition.titleKey),
          description: template.description === source.description ? t(definition.descKey) : template.description,
        };
      });
    }
    return ALL_TEMPLATE_DEFINITIONS.map((item) => ({
      id: item.id,
      name: t(item.titleKey),
      description: t(item.descKey),
      template_id: item.id,
    }));
  }, [managedTemplates, t]);

  const resetCanvas = useCallback((showWelcome: boolean) => {
    setCanvasID("");
    titleManuallyEditedRef.current = false;
    if (showWelcome) {
      workflowNameRef.current = t("canvas.untitled");
      setTitle(t("canvas.untitled"));
    } else {
      const time = new Date().toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" });
      const blankTitle = `${t("canvas.untitled")} · ${time}`;
      workflowNameRef.current = blankTitle;
      setTitle(blankTitle);
    }
    const initialNodes: CanvasNode[] = showWelcome
      ? []
      : [{
          id: newNodeID(),
          type: "textInput",
          position: { x: 80, y: 100 },
          selected: true,
          data: { label: t("canvas.node.textInput"), prompt: "" },
        }];
    nodesRef.current = initialNodes;
    edgesRef.current = [];
    setNodes(initialNodes);
    setEdges([]);
    setViewport({ x: 0, y: 0, zoom: 1 });
    setNotice(showWelcome ? "" : t("canvas.blankCreated"));
    setNodePaletteOpen(false);
    setOutputMenu(null);
    setShowEmptyWelcome(showWelcome);
  }, [locale, setEdges, setNodes, setViewport, t]);

  const newCanvas = useCallback(() => {
    resetCanvas(true);
    if (initialTemplateID) window.setTimeout(bootstrapInitialTemplate, 0);
  }, [bootstrapInitialTemplate, initialTemplateID, resetCanvas]);
  const newBlankCanvas = useCallback(() => resetCanvas(false), [resetCanvas]);

  const documentSnapshot = useCallback((): CanvasDocument => ({
    version: 1,
    nodes: nodesRef.current,
    edges: edgesRef.current,
    viewport: getViewport(),
  }), [getViewport]);

  const save = useCallback(async (silent = false) => {
    setSaving(true);
    if (!silent) setNotice("");
    try {
      const effectiveTitle = titleManuallyEditedRef.current
        ? truncateCanvasTitle(title, 64)
        : automaticCanvasTitle(nodesRef.current, workflowNameRef.current || title);
      if (effectiveTitle && effectiveTitle !== title) setTitle(effectiveTitle);
      if (!authenticated) {
        const now = new Date().toISOString();
        const publicID = canvasID.startsWith("local_") ? canvasID : `local_${crypto.randomUUID()}`;
        const existing = readLocalCanvases();
        const previous = existing.find((item) => item.public_id === publicID);
        const item: CanvasDetail = {
          public_id: publicID,
          workflow_code: workflowCode,
          title: effectiveTitle || t("canvas.untitled"),
          document: documentSnapshot(),
          created_at: previous?.created_at || now,
          updated_at: now,
        };
        writeLocalCanvases([item, ...existing.filter((entry) => entry.public_id !== publicID)]);
        setCanvasID(publicID);
        if (!silent) setNotice(t("canvas.savedLocally"));
        refreshHistory();
        return;
      }
      const serverCanvasID = canvasID && !canvasID.startsWith("local_") ? canvasID : "";
      const item = await api<CanvasDetail>(serverCanvasID ? `/api/canvases/${serverCanvasID}` : "/api/canvases", {
        method: serverCanvasID ? "PUT" : "POST",
        body: JSON.stringify({ workflow_code: workflowCode, title: effectiveTitle || t("canvas.untitled"), document: documentSnapshot() }),
      });
      setCanvasID(item.public_id);
      setTitle(item.title);
      if (!silent) setNotice(t("canvas.saved"));
      refreshHistory();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t("canvas.saveFailed"));
    } finally {
      setSaving(false);
    }
  }, [authenticated, canvasID, documentSnapshot, refreshHistory, t, title, workflowCode]);

  const loadCanvas = useCallback(async (id: string) => {
    try {
      const item = id.startsWith("local_") || !authenticated
        ? readLocalCanvases().find((entry) => entry.public_id === id)
        : await api<CanvasDetail>(`/api/canvases/${id}`);
      if (!item) throw new Error(t("canvas.loadFailed"));
      const document = item.document || { version: 1, nodes: [], edges: [], viewport: { x: 0, y: 0, zoom: 1 } };
      setCanvasID(item.public_id);
      workflowNameRef.current = item.title;
      titleManuallyEditedRef.current = true;
      setTitle(item.title);
      nodesRef.current = Array.isArray(document.nodes) ? normalizeCanvasNodes(document.nodes) : [];
      edgesRef.current = Array.isArray(document.edges) ? document.edges : [];
      setNodes(nodesRef.current);
      setEdges(edgesRef.current);
      setShowEmptyWelcome(false);
      setHistoryOpen(false);
      window.setTimeout(() => {
        if (document.viewport) void setViewport(document.viewport);
        else void fitView({ padding: 0.3, maxZoom: 0.72 });
      }, 50);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t("canvas.loadFailed"));
    }
  }, [authenticated, fitView, setEdges, setNodes, setViewport, t]);

  const deleteCanvas = useCallback(async (event: React.MouseEvent, id: string) => {
    event.stopPropagation();
    if (!window.confirm(t("canvas.deleteConfirm"))) return;
    try {
      if (id.startsWith("local_") || !authenticated) {
        writeLocalCanvases(readLocalCanvases().filter((item) => item.public_id !== id));
      } else {
        await api(`/api/canvases/${id}`, { method: "DELETE" });
      }
      if (canvasID === id) newCanvas();
      refreshHistory();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t("canvas.deleteFailed"));
    }
  }, [authenticated, canvasID, newCanvas, refreshHistory, t]);

  const exportCanvas = useCallback(() => {
    const blob = new Blob([JSON.stringify({ title, ...documentSnapshot() }, null, 2)], { type: "application/json;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = `${title || t("canvas.title")}.starai-canvas.json`;
    link.click();
    URL.revokeObjectURL(link.href);
  }, [documentSnapshot, t, title]);

  const importCanvas = useCallback(async (file: File) => {
    try {
      const parsed = JSON.parse(await file.text()) as CanvasDocument & { title?: string };
      if (!Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) throw new Error(t("canvas.invalidFile"));
      setCanvasID("");
      const importedTitle = parsed.title || file.name.replace(/\.starai-canvas\.json$|\.json$/i, "") || t("canvas.importCanvas");
      workflowNameRef.current = importedTitle;
      titleManuallyEditedRef.current = Boolean(parsed.title);
      setTitle(importedTitle);
      nodesRef.current = normalizeCanvasNodes(parsed.nodes);
      edgesRef.current = parsed.edges;
      setNodes(nodesRef.current);
      setEdges(parsed.edges);
      setShowEmptyWelcome(false);
      setImportOpen(false);
      window.setTimeout(() => {
        if (parsed.viewport) void setViewport({ ...parsed.viewport, zoom: 1 });
        else void setViewport({ x: 0, y: 0, zoom: 1 });
      }, 50);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t("canvas.importFailed"));
    }
  }, [setEdges, setNodes, setViewport, t]);

  const importCanvasDocument = useCallback((template: CanvasTemplate) => {
    if (!template.document) {
      appendTemplate(template.template_id || template.id, template.name);
      setImportOpen(false);
      return;
    }
    const document = template.document;
    if (!Array.isArray(document.nodes) || !Array.isArray(document.edges)) {
      setNotice(t("canvas.invalidFile"));
      return;
    }
    setCanvasID("");
    const templateTitle = template.name || t("canvas.untitled");
    workflowNameRef.current = templateTitle;
    titleManuallyEditedRef.current = template.id === "pasted";
    setTitle(templateTitle);
    const templateNodes = normalizeCanvasNodes(template.id === "pasted"
      ? document.nodes
      : document.nodes.map((node) => node.type === "textInput"
        ? { ...node, data: { ...node.data, label: template.name || node.data.label } }
        : node));
    nodesRef.current = templateNodes;
    edgesRef.current = document.edges;
    setNodes(templateNodes);
    setEdges(document.edges);
    setShowEmptyWelcome(false);
    setImportOpen(false);
    window.setTimeout(() => {
      if (document.viewport) void setViewport({ ...document.viewport, zoom: 1 });
      else void setViewport({ x: 0, y: 0, zoom: 1 });
    }, 50);
  }, [appendTemplate, setEdges, setNodes, setViewport, t]);

  const importFromCode = useCallback(() => {
    try {
      const parsed = JSON.parse(importCode) as CanvasDocument & { title?: string };
      if (!Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) throw new Error(t("canvas.invalidFile"));
      importCanvasDocument({
        id: "pasted",
        name: parsed.title || t("canvas.importedCanvas"),
        document: parsed,
      });
      setImportCode("");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t("canvas.invalidFile"));
    }
  }, [importCanvasDocument, importCode, t]);

  const runAll = useCallback(async () => {
    await executeNodes();
  }, [executeNodes]);

  useEffect(() => {
    if (!resultPreview) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [resultPreview]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const editing = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        void save();
        return;
      }
      if (!editing && (event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault();
        void runAll();
      }
      if (event.key === "Escape") {
        setOutputMenu(null);
        setNodePaletteOpen(false);
        setResultPreview(null);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [runAll, save]);

  useEffect(() => {
    if (titleManuallyEditedRef.current || showEmptyWelcome || nodes.length === 0) return;
    const nextTitle = automaticCanvasTitle(nodes, workflowNameRef.current || title);
    if (nextTitle && nextTitle !== title) setTitle(nextTitle);
  }, [nodes, showEmptyWelcome, title]);

  useEffect(() => {
    const nodeRunning = nodes.some((node) => node.data.status === "pending" || node.data.status === "running");
    if (saving || runningAll || nodeRunning || (!canvasID && (showEmptyWelcome || nodes.length === 0))) return;
    const fingerprint = JSON.stringify({
      title,
      nodes: nodes.map((node) => ({ id: node.id, type: node.type, position: node.position, data: node.data })),
      edges,
    });
    if (fingerprint === lastAutoSaveFingerprintRef.current) return;
    if (autoSaveTimerRef.current) clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = setTimeout(() => {
      lastAutoSaveFingerprintRef.current = fingerprint;
      void save(true);
    }, 700);
    return () => {
      if (autoSaveTimerRef.current) clearTimeout(autoSaveTimerRef.current);
    };
  }, [canvasID, edges, nodes, runningAll, save, saving, showEmptyWelcome, title]);

  const deleteSelected = useCallback(() => {
    const selectedIDs = new Set(nodesRef.current.filter((node) => node.selected).map((node) => node.id));
    if (selectedIDs.size === 0) {
      setNotice(t("canvas.selectNodeToDelete"));
      return;
    }
    nodesRef.current = nodesRef.current.filter((node) => !selectedIDs.has(node.id));
    edgesRef.current = edgesRef.current.filter((edge) => !selectedIDs.has(edge.source) && !selectedIDs.has(edge.target));
    setNodes(nodesRef.current);
    setEdges(edgesRef.current);
  }, [setEdges, setNodes, t]);

  const filteredTemplates = NODE_TEMPLATES.filter((item) => `${t(item.titleKey)}${t(item.descKey)}`.toLowerCase().includes(nodeSearch.trim().toLowerCase()));

  return (
    <CanvasNodeActions.Provider value={actions}>
      <div ref={editorRef} className="relative min-h-0 w-full flex-1 overflow-hidden overscroll-none bg-[#eef3f8] dark:bg-[#080d14]">
        <input
          ref={importRef}
          type="file"
          accept=".json,.starai-canvas.json,application/json"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void importCanvas(file);
            event.target.value = "";
          }}
        />
        <ReactFlow<CanvasNode, CanvasEdge>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onEdgesDelete={(deletedEdges) => {
            deletedEdges.forEach((edge) => markDirtyFrom(edge.target));
          }}
          onConnect={onConnect}
          onConnectStart={(_, params) => {
            connectionSourceRef.current = params.nodeId || "";
            connectionCompletedRef.current = false;
            setOutputMenu(null);
          }}
          onConnectEnd={onConnectEnd}
          onPaneClick={() => setOutputMenu(null)}
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          minZoom={0.18}
          maxZoom={2.5}
          deleteKeyCode={["Backspace", "Delete"]}
          selectionOnDrag={!touchNavigation}
          panOnDrag={touchNavigation ? true : [1, 2]}
          panOnScroll={!touchNavigation}
          zoomOnPinch
          zoomOnDoubleClick={!touchNavigation}
          preventScrolling
          colorMode={flowColorMode}
          defaultEdgeOptions={{ type: "smoothstep", animated: true }}
          className="infinite-canvas-flow"
        >
          <Background variant={BackgroundVariant.Lines} gap={44} size={1} color="rgba(100,116,139,0.14)" />
          {showMiniMap && nodes.length > 0 && (
            <MiniMap
              pannable
              zoomable
              ariaLabel={t("canvas.navigator")}
              className="!bottom-16 !right-4 !hidden !h-24 !w-36 !rounded-xl !border !border-gray-200 !bg-white/85 sm:!block dark:!border-white/10 dark:!bg-gray-900/85"
              nodeColor={(node) => node.type === "generator" ? "#22d3ee" : node.type === "imageInput" ? "#34d399" : "#60a5fa"}
            />
          )}

          <Panel position="top-left" className="!m-3 flex flex-col gap-2 sm:!m-4">
            <button type="button" onClick={newCanvas} className="inline-flex h-10 items-center justify-center gap-2 rounded-2xl border border-cyan-400/30 bg-cyan-500/10 px-5 text-sm font-semibold text-cyan-600 backdrop-blur hover:bg-cyan-500/15 dark:text-cyan-300">
              <Plus size={16} /> {t("canvas.new")}
            </button>
            <div className="relative">
              <button type="button" onClick={() => setHistoryOpen((value) => !value)} className="inline-flex h-9 items-center gap-2 rounded-xl border border-gray-200 bg-white/85 px-3 text-xs text-gray-600 shadow-sm backdrop-blur dark:border-white/10 dark:bg-gray-900/85 dark:text-gray-300">
                <RotateCcw size={14} /> {t("canvas.history")} <ChevronDown size={13} />
              </button>
              {historyOpen && (
                <div className="absolute left-0 top-11 z-30 w-72 overflow-hidden rounded-2xl border border-gray-200 bg-white p-2 shadow-xl dark:border-white/10 dark:bg-gray-900">
                  {history.length ? history.map((item) => (
                    <button key={item.public_id} type="button" onClick={() => void loadCanvas(item.public_id)} className="group flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-white/5">
                      <div className="min-w-0 flex-1">
                        <div title={item.title} className="truncate text-xs font-medium text-gray-800 dark:text-gray-100">{item.title}</div>
                        <div className="mt-0.5 text-[10px] text-gray-400">{formatDate(item.updated_at)}</div>
                      </div>
                      <span onClick={(event) => void deleteCanvas(event, item.public_id)} className="rounded-lg p-1 text-gray-300 opacity-0 hover:bg-red-50 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-red-500/10">
                        <Trash2 size={13} />
                      </span>
                    </button>
                  )) : <div className="px-3 py-5 text-center text-xs text-gray-400">{t("canvas.noHistory")}</div>}
                </div>
              )}
            </div>
          </Panel>

          <Panel position="top-center" className="!m-3 hidden items-center gap-3 md:!flex">
            <input
              value={title}
              onChange={(event) => {
                titleManuallyEditedRef.current = true;
                setTitle(event.target.value);
              }}
              maxLength={64}
              title={title}
              className="nodrag w-44 truncate rounded-xl border border-transparent bg-transparent px-3 py-2 text-center text-xs font-medium text-gray-500 outline-none hover:border-gray-200 focus:border-cyan-300 focus:bg-white/80 dark:text-gray-300 dark:focus:bg-gray-900/80"
              aria-label={t("canvas.title")}
            />
            <div className="flex h-9 w-56 items-center gap-2 rounded-xl border border-gray-200 bg-white/80 px-3 backdrop-blur dark:border-white/10 dark:bg-gray-900/80">
              <Search size={14} className="text-cyan-500" />
              <input value={nodeSearch} onChange={(event) => setNodeSearch(event.target.value)} placeholder={t("canvas.searchNodes")} className="nodrag min-w-0 flex-1 bg-transparent text-xs outline-none dark:text-gray-100" />
            </div>
          </Panel>

          {nodes.length === 0 && showEmptyWelcome && (
            <Panel position="top-left" className="pointer-events-none !inset-0 !m-0 flex !w-full items-center justify-center">
              <div className="pointer-events-auto flex w-[min(760px,calc(100vw-2rem))] flex-col items-center">
                <button type="button" onClick={() => setImportOpen(true)} className="mb-4 flex flex-col items-center text-gray-400 hover:text-cyan-600">
                  <span className="mb-2 flex h-11 w-11 items-center justify-center rounded-full border border-gray-300 dark:border-white/15"><Plus size={20} /></span>
                  <span className="text-sm font-semibold">{t("canvas.empty")}</span>
                  <span className="mt-1 text-[11px]">{t("canvas.emptyDesc")}</span>
                  <span className="mt-3 inline-flex items-center gap-1.5 rounded-xl border border-cyan-300 bg-cyan-500/10 px-4 py-2 text-xs font-semibold text-cyan-600 dark:text-cyan-300"><Upload size={14} /> {t("canvas.importCanvas")}</span>
                </button>
                <div className="grid w-full grid-cols-2 gap-1.5 sm:grid-cols-4 sm:gap-2">
                  {filteredTemplates.map((item) => {
                    const Icon = item.icon;
                    return (
                      <button key={item.id} type="button" onClick={() => appendTemplate(item.id, t(item.titleKey))} className="flex min-w-0 items-center gap-2 rounded-xl border border-gray-200/80 bg-white/85 p-2 text-left shadow-sm backdrop-blur transition hover:-translate-y-0.5 hover:border-cyan-300 hover:shadow-md sm:gap-3 sm:rounded-2xl sm:p-3 dark:border-white/10 dark:bg-gray-900/85">
                        <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg sm:h-9 sm:w-9 sm:rounded-xl ${TEMPLATE_TONES[item.tone]}`}><Icon size={16} /></span>
                        <span className="min-w-0">
                          <span className="block line-clamp-2 text-[10px] font-semibold leading-tight text-gray-800 sm:truncate sm:text-xs dark:text-gray-100">{t(item.titleKey)}</span>
                          <span className="mt-0.5 hidden truncate text-[10px] text-gray-400 sm:block">{t(item.descKey)}</span>
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            </Panel>
          )}

          <Panel position="bottom-center" className="!bottom-3 !m-0 max-w-[calc(100vw-2rem)]">
            <div className="relative">
              {nodePaletteOpen && (
                <div className="absolute bottom-12 left-1/2 z-40 grid w-[min(620px,calc(100vw-2rem))] -translate-x-1/2 grid-cols-2 gap-2 rounded-2xl border border-gray-200 bg-white/95 p-3 shadow-2xl backdrop-blur sm:grid-cols-6 dark:border-white/10 dark:bg-gray-900/95">
                  {NEW_NODE_OPTIONS.map((item) => {
                    const Icon = item.icon;
                    return (
                      <button key={item.kind} type="button" onClick={() => appendSingleNode(item.kind)} className="flex min-h-16 flex-col items-center justify-center gap-1.5 rounded-xl border border-gray-100 bg-gray-50 px-2 py-2 text-center text-[10px] font-medium text-gray-600 transition hover:border-cyan-300 hover:bg-cyan-50 hover:text-cyan-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300 dark:hover:bg-cyan-500/10 dark:hover:text-cyan-300">
                        <Icon size={17} />
                        <span>{t(item.key)}</span>
                      </button>
                    );
                  })}
                </div>
              )}
              <div className="flex items-center gap-0.5 rounded-2xl border border-gray-200 bg-white/90 p-1.5 shadow-lg backdrop-blur sm:gap-1 dark:border-white/10 dark:bg-gray-900/90">
              <button type="button" title={t("canvas.toolbar.organize")} aria-label={t("canvas.toolbar.organize")} onClick={() => void fitView({ padding: 0.3, maxZoom: 0.72, duration: 400 })} className="flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs text-gray-500 hover:bg-gray-100 sm:px-2.5 dark:text-gray-300 dark:hover:bg-white/10"><AlignCenter size={14} /><span className="hidden sm:inline">{t("canvas.toolbar.organize")}</span></button>
              <button type="button" title={t("canvas.toolbar.save")} aria-label={t("canvas.toolbar.save")} onClick={() => void save()} disabled={saving} className="flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs text-gray-500 hover:bg-gray-100 disabled:opacity-50 sm:px-2.5 dark:text-gray-300 dark:hover:bg-white/10">{saving ? <LoaderCircle size={14} className="animate-spin" /> : <Save size={14} />}<span className="hidden sm:inline">{saving ? t("common.saving") : t("canvas.toolbar.save")}</span></button>
              <button type="button" title={t("canvas.toolbar.export")} aria-label={t("canvas.toolbar.export")} onClick={exportCanvas} className="flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs text-gray-500 hover:bg-gray-100 sm:px-2.5 dark:text-gray-300 dark:hover:bg-white/10"><Download size={14} /><span className="hidden sm:inline">{t("canvas.toolbar.export")}</span></button>
              <button type="button" title={t("canvas.toolbar.import")} aria-label={t("canvas.toolbar.import")} onClick={() => setImportOpen(true)} className="flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs text-gray-500 hover:bg-gray-100 sm:px-2.5 dark:text-gray-300 dark:hover:bg-white/10"><Upload size={14} /><span className="hidden sm:inline">{t("canvas.toolbar.import")}</span></button>
              <button type="button" title={t("canvas.toolbar.clear")} aria-label={t("canvas.toolbar.clear")} onClick={newCanvas} className="flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs text-gray-500 hover:bg-red-50 hover:text-red-500 sm:px-2.5 dark:text-gray-300 dark:hover:bg-red-500/10"><Trash2 size={14} /><span className="hidden sm:inline">{t("canvas.toolbar.clear")}</span></button>
              <button type="button" title={t("canvas.toolbar.addNode")} aria-label={t("canvas.toolbar.addNode")} aria-expanded={nodePaletteOpen} onClick={() => setNodePaletteOpen((value) => !value)} className={`flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-2 text-xs sm:px-2.5 ${nodePaletteOpen ? "bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300" : "text-gray-500 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/10"}`}><Plus size={14} /><span className="hidden sm:inline">{t("canvas.toolbar.addNode")}</span></button>
              <button
                type="button"
                onClick={() => void runAll()}
                disabled={!runningAll && !nodes.some((node) => node.type === "generator" || node.type === "compositor")}
                className={`ml-1 flex h-8 items-center gap-1.5 whitespace-nowrap rounded-xl px-3 text-xs font-semibold text-white disabled:opacity-40 ${runningAll ? "bg-red-500 hover:bg-red-600" : "bg-emerald-500 hover:bg-emerald-600"}`}
              >
                {runningAll ? <X size={14} /> : <Play size={14} fill="currentColor" />}
                {runningAll
                  ? t("canvas.toolbar.stop", { current: executionProgress.current, total: executionProgress.total })
                  : t("canvas.toolbar.runWorkflow")}
              </button>
              </div>
            </div>
            {notice && <div className="mx-auto mt-2 w-fit rounded-full bg-gray-900/80 px-3 py-1 text-[10px] text-white shadow dark:bg-white/90 dark:text-gray-900">{notice}</div>}
          </Panel>

          <Panel position="bottom-left" className="!bottom-12 !m-3 sm:!bottom-3 sm:!m-4">
            <button type="button" onClick={() => setHelpOpen((value) => !value)} className="flex h-9 items-center gap-2 rounded-xl border border-cyan-300 bg-white/85 px-3 text-xs font-medium text-cyan-600 shadow-sm backdrop-blur dark:bg-gray-900/85 dark:text-cyan-300">
              <CircleHelp size={15} /> {t("canvas.help")}
            </button>
            {helpOpen && (
              <div className="absolute bottom-12 left-0 w-72 rounded-2xl border border-gray-200 bg-white p-4 text-xs leading-relaxed text-gray-500 shadow-xl dark:border-white/10 dark:bg-gray-900 dark:text-gray-300">
                <div className="mb-2 font-semibold text-gray-800 dark:text-gray-100">{t("canvas.helpTitle")}</div>
                <ol className="list-decimal space-y-1.5 pl-4">
                  <li>{t("canvas.helpStep1")}</li>
                  <li>{t("canvas.helpStep2")}</li>
                  <li>{t("canvas.helpStep3")}</li>
                  <li>{t("canvas.helpStep4")}</li>
                  <li>{t("canvas.helpStep5")}</li>
                </ol>
              </div>
            )}
          </Panel>

          <Panel position="bottom-right" className="!bottom-3 !right-3 !m-0 hidden items-center gap-1 sm:!flex">
            <button
              type="button"
              title={t("canvas.navigator")}
              aria-label={t("canvas.navigator")}
              aria-pressed={showMiniMap}
              onClick={() => setShowMiniMap((value) => !value)}
              className={`flex h-9 w-9 items-center justify-center rounded-xl border shadow-sm backdrop-blur ${showMiniMap && nodes.length > 0 ? "border-cyan-300 bg-cyan-50 text-cyan-600 dark:bg-cyan-500/15 dark:text-cyan-300" : "border-gray-200 bg-white/85 text-gray-500 dark:border-white/10 dark:bg-gray-900/85 dark:text-gray-300"}`}
            >
              <MapIcon size={15} />
            </button>
            <button
              type="button"
              title={t("canvas.deleteSelected")}
              aria-label={t("canvas.deleteSelected")}
              onClick={deleteSelected}
              disabled={!nodes.some((node) => node.selected)}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-gray-200 bg-white/85 text-gray-500 shadow-sm backdrop-blur hover:border-red-200 hover:bg-red-50 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-40 dark:border-white/10 dark:bg-gray-900/85 dark:text-gray-300 dark:hover:bg-red-500/10"
            >
              <Trash2 size={15} />
            </button>
          </Panel>
        </ReactFlow>

        {resultPreview && createPortal(
          <div
            role="dialog"
            aria-modal="true"
            aria-label={resultPreview.title}
            className="fixed inset-0 z-[220] flex h-[100dvh] w-screen items-center justify-center overflow-hidden bg-black/80 p-3 sm:p-5"
            onClick={() => setResultPreview(null)}
          >
            <div
              className={`relative flex max-h-[calc(100dvh-2rem)] w-full items-center justify-center overflow-hidden rounded-2xl bg-black shadow-2xl ${
                resultPreview.kind === "video" ? "max-w-6xl" : resultPreview.kind === "audio" ? "max-w-2xl p-8" : "max-w-5xl"
              }`}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="absolute left-3 top-3 z-20 max-w-[calc(100%-7rem)] truncate rounded-lg bg-black/60 px-2.5 py-1.5 text-xs font-medium text-white backdrop-blur">
                {resultPreview.title}
              </div>
              <div className="absolute right-3 top-3 z-20 flex gap-2">
                <button
                  type="button"
                  onClick={() => void downloadCanvasResult(
                    resultPreview.url,
                    `starai-${resultPreview.kind}-${Date.now()}.${resultPreview.kind === "image" ? "png" : resultPreview.kind === "video" ? "mp4" : "mp3"}`
                  )}
                  title={t("common.download")}
                  aria-label={t("common.download")}
                  className="flex h-9 w-9 items-center justify-center rounded-xl border border-white/20 bg-gray-950/80 text-white shadow backdrop-blur hover:bg-gray-900"
                >
                  <Download size={16} />
                </button>
                <button
                  type="button"
                  onClick={() => setResultPreview(null)}
                  title={t("common.close")}
                  aria-label={t("common.close")}
                  className="flex h-9 w-9 items-center justify-center rounded-xl border border-white/20 bg-gray-950/80 text-white shadow backdrop-blur hover:bg-gray-900"
                >
                  <X size={16} />
                </button>
              </div>
              {resultPreview.kind === "video" ? (
                <video src={resultPreview.url} controls autoPlay className="h-auto max-h-[88dvh] w-full object-contain" />
              ) : resultPreview.kind === "audio" ? (
                <audio src={resultPreview.url} controls autoPlay className="w-full" />
              ) : (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={resultPreview.url} alt={resultPreview.title} className="h-auto max-h-[88dvh] w-auto max-w-full object-contain" />
              )}
            </div>
          </div>,
          document.body
        )}

        {outputMenu && (
          <div
            className="absolute z-50 w-[216px] rounded-2xl border border-cyan-300/50 bg-white/95 p-2.5 shadow-2xl backdrop-blur dark:border-cyan-400/25 dark:bg-[#171d27]/95"
            style={{ left: outputMenu.left, top: outputMenu.top }}
            onPointerDown={(event) => event.stopPropagation()}
          >
            <div className="mb-2 flex items-center gap-2 px-1 text-[11px] font-semibold text-gray-700 dark:text-gray-200">
              <span className="h-2 w-2 rounded-full bg-cyan-500" />
              {t("canvas.node.chooseNext")}
            </div>
            <div className="grid grid-cols-2 gap-1.5">
              {OUTPUT_NODE_OPTIONS.map((item) => {
                const Icon = item.icon;
                return (
                  <button
                    key={item.kind}
                    type="button"
                    onClick={() => appendSingleNode(item.kind, { sourceID: outputMenu.sourceID, position: outputMenu.nodePosition })}
                    className="flex min-h-16 flex-col items-start justify-center gap-1.5 rounded-xl border border-gray-100 bg-gray-50 px-3 py-2 text-left text-[10px] font-medium text-gray-600 transition hover:border-cyan-300 hover:bg-cyan-50 hover:text-cyan-600 dark:border-white/10 dark:bg-white/5 dark:text-gray-300 dark:hover:bg-cyan-500/10 dark:hover:text-cyan-300"
                  >
                    <Icon size={16} />
                    <span>{t(item.key)}</span>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {importOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm" onClick={() => setImportOpen(false)}>
            <div className="w-full max-w-md overflow-hidden rounded-2xl border border-white/10 bg-white shadow-2xl dark:bg-[#151b25]" onClick={(event) => event.stopPropagation()}>
              <div className="flex items-center justify-between px-5 py-4">
                <div className="flex items-center gap-2 font-semibold text-gray-900 dark:text-gray-100"><FolderOpen size={17} />{t("canvas.importDialog.title")}</div>
                <button type="button" onClick={() => setImportOpen(false)} className="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 dark:hover:bg-white/10"><X size={15} /></button>
              </div>
              <div className="flex gap-1 border-b border-gray-100 px-4 dark:border-white/10">
                {(["templates", "history", "code"] as const).map((tab) => (
                  <button key={tab} type="button" onClick={() => setImportTab(tab)} className={`border-b-2 px-3 py-2 text-xs ${importTab === tab ? "border-cyan-500 font-semibold text-cyan-600" : "border-transparent text-gray-400"}`}>
                    {t(`canvas.importDialog.${tab}`)}
                  </button>
                ))}
              </div>
              <div className="max-h-[52vh] min-h-72 overflow-y-auto overscroll-contain p-4">
                {importTab === "templates" && (
                  <div className="space-y-2">
                    <button type="button" onClick={() => { newBlankCanvas(); setImportOpen(false); }} className="flex w-full items-center gap-3 rounded-xl border border-cyan-200 bg-cyan-50/50 p-3 text-left transition hover:border-cyan-400 dark:border-cyan-500/20 dark:bg-cyan-500/10">
                      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-white text-cyan-600 shadow-sm dark:bg-white/10 dark:text-cyan-300"><Plus size={16} /></span>
                      <span className="min-w-0">
                        <span className="block text-xs font-semibold text-gray-800 dark:text-gray-100">{t("canvas.startBlank")}</span>
                        <span className="mt-1 block text-[10px] text-gray-400">{t("canvas.startBlankDesc")}</span>
                      </span>
                    </button>
                    {availableTemplates.map((template) => (
                      <button key={template.id} type="button" onClick={() => importCanvasDocument(template)} className="flex w-full items-center gap-3 rounded-xl border border-gray-100 p-3 text-left transition hover:border-cyan-300 hover:bg-cyan-50/50 dark:border-white/10 dark:hover:bg-cyan-500/10">
                        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-300"><FileJson size={16} /></span>
                        <span className="min-w-0">
                          <span className="block truncate text-xs font-semibold text-gray-800 dark:text-gray-100">{template.name}</span>
                          <span className="mt-1 block line-clamp-2 text-[10px] leading-relaxed text-gray-400">{template.description || t("canvas.importDialog.templateDesc")}</span>
                        </span>
                      </button>
                    ))}
                  </div>
                )}
                {importTab === "history" && (
                  <div className="space-y-2">
                    {history.length ? history.map((item) => (
                      <button key={item.public_id} type="button" onClick={() => { void loadCanvas(item.public_id); setImportOpen(false); }} className="flex w-full items-center gap-3 rounded-xl border border-gray-100 p-3 text-left hover:border-cyan-300 dark:border-white/10">
                        <RotateCcw size={15} className="shrink-0 text-cyan-500" />
                        <span className="min-w-0 flex-1"><span title={item.title} className="block truncate text-xs font-medium dark:text-gray-100">{item.title}</span><span className="mt-0.5 block text-[10px] text-gray-400">{formatDate(item.updated_at)}</span></span>
                      </button>
                    )) : <div className="py-20 text-center text-xs text-gray-400">{t("canvas.noHistory")}</div>}
                  </div>
                )}
                {importTab === "code" && (
                  <div className="space-y-3">
                    <textarea value={importCode} onChange={(event) => setImportCode(event.target.value)} placeholder={t("canvas.importDialog.codePlaceholder")} className="h-40 w-full resize-none rounded-xl border border-gray-200 bg-gray-50 p-3 font-mono text-[10px] leading-relaxed outline-none focus:border-cyan-300 dark:border-white/10 dark:bg-white/5 dark:text-gray-100" />
                    <div className="flex items-center justify-between gap-3">
                      <button type="button" onClick={() => importRef.current?.click()} className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-gray-200 px-3 text-xs text-gray-500 hover:bg-gray-50 dark:border-white/10 dark:text-gray-300 dark:hover:bg-white/5"><Upload size={13} />{t("canvas.importDialog.selectFile")}</button>
                      <button type="button" onClick={importFromCode} disabled={!importCode.trim()} className="h-9 rounded-xl bg-cyan-500 px-4 text-xs font-semibold text-white disabled:opacity-40">{t("canvas.importDialog.import")}</button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        {assetLibraryOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm" onClick={() => setAssetLibraryOpen(false)}>
            <div className="flex max-h-[76vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-white/10 bg-white shadow-2xl dark:bg-[#151b25]" onClick={(event) => event.stopPropagation()}>
              <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-white/10">
                <div>
                  <div className="flex items-center gap-2 font-semibold text-gray-900 dark:text-gray-100"><FolderOpen size={17} />{t("canvas.assetLibrary")}</div>
                  <div className="mt-1 text-[10px] text-gray-400">
                    {assetTargetKind === "video"
                      ? t("canvas.assetLibraryVideoHint")
                      : assetTargetKind === "audio"
                        ? t("canvas.assetLibraryAudioHint")
                        : t("canvas.assetLibraryImageHint")}
                  </div>
                </div>
                <button type="button" onClick={() => setAssetLibraryOpen(false)} className="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 dark:hover:bg-white/10"><X size={15} /></button>
              </div>
              <form className="flex gap-2 px-4 py-3" onSubmit={(event) => { event.preventDefault(); void loadAssetLibrary(assetQuery, assetTargetKind); }}>
                <div className="flex h-9 flex-1 items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 dark:border-white/10 dark:bg-white/5">
                  <Search size={14} className="text-gray-400" />
                  <input value={assetQuery} onChange={(event) => setAssetQuery(event.target.value)} placeholder={t("canvas.assetLibrarySearch")} className="min-w-0 flex-1 bg-transparent text-xs outline-none dark:text-gray-100" />
                </div>
                <button type="submit" className="h-9 rounded-xl bg-cyan-500 px-4 text-xs font-semibold text-white hover:bg-cyan-600">{t("common.search")}</button>
              </form>
              <div className="min-h-72 flex-1 overflow-y-auto px-4 pb-4">
                {assetLoading ? (
                  <div className="flex h-72 items-center justify-center text-sm text-gray-400"><LoaderCircle size={20} className="mr-2 animate-spin" />{t("canvas.assetLibraryLoading")}</div>
                ) : assetItems.length ? (
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
                    {assetItems.map((asset) => (
                      <button key={asset.public_id} type="button" onClick={() => selectAsset(asset)} className="group overflow-hidden rounded-xl border border-gray-200 bg-gray-50 text-left transition hover:border-cyan-400 hover:shadow-md dark:border-white/10 dark:bg-white/5">
                        <div className="aspect-square overflow-hidden bg-gray-100 dark:bg-gray-950/40">
                          {assetTargetKind === "video"
                            ? <video src={asset.url} muted preload="metadata" className="h-full w-full object-cover" />
                            : assetTargetKind === "audio"
                              ? <div className="flex h-full items-center justify-center p-2"><audio src={asset.url} controls preload="metadata" className="w-full" /></div>
                            // eslint-disable-next-line @next/next/no-img-element
                            : <img src={asset.url} alt="" loading="lazy" className="h-full w-full object-cover transition group-hover:scale-105" />}
                        </div>
                        <div className="truncate px-2.5 py-2 text-[11px] font-medium text-gray-700 dark:text-gray-200">{asset.name || asset.public_id}</div>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="flex h-72 flex-col items-center justify-center text-xs text-gray-400"><FolderOpen size={28} className="mb-2 opacity-50" />{t("canvas.assetLibraryEmpty")}</div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </CanvasNodeActions.Provider>
  );
}

export function InfiniteCanvasWorkspace({
  authenticated = false,
  workflowCode = "infinite_canvas",
  initialTemplateID = "",
}: {
  authenticated?: boolean;
  workflowCode?: string;
  initialTemplateID?: string;
}) {
  return (
    <ReactFlowProvider>
      <CanvasEditor authenticated={authenticated} workflowCode={workflowCode} initialTemplateID={initialTemplateID} />
    </ReactFlowProvider>
  );
}
