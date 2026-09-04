type VideoAnalysisModel = {
  code: string;
  display_name?: string;
  tags?: string[];
  runtime_rule?: Record<string, unknown>;
};

export function supportsVideoAnalysis(model?: VideoAnalysisModel) {
  if (!model) return false;
  const capabilities = (model.runtime_rule?.capabilities || {}) as Record<string, unknown>;
  if (capabilities.video_input === true || capabilities.video_understanding === true || capabilities.video_analysis === true) return true;
  return /(gemini|qwen[^\s]*[-_]?vl|video[^\s]*(?:understand|analysis)|(?:understand|analysis)[^\s]*video)/i.test(
    `${model.code} ${model.display_name || ""} ${(model.tags || []).join(" ")}`
  );
}
