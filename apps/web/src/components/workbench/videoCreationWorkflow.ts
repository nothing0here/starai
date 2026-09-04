export function storyStoryboardSegments(text: string, expectedCount = 0): Record<string, unknown>[] {
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
  let parsed: unknown;
  try {
    parsed = JSON.parse(candidate);
  } catch {
    return [];
  }
  const record = parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : null;
  const candidates = Array.isArray(parsed)
    ? parsed
    : Array.isArray(record?.segments)
      ? record.segments
      : Array.isArray(record?.shots)
        ? record.shots
        : Array.isArray(record?.storyboard)
          ? record.storyboard
          : [];
  const segments = candidates.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object" && !Array.isArray(item));
  if (expectedCount > 0 && segments.length !== expectedCount) return [];
  const requiredFields = ["scene", "camera", "image_prompt", "video_prompt"];
  return segments.every((segment, index) =>
    Number(segment.segment_index ?? segment.index) === index + 1
    && requiredFields.every((field) => String(segment[field] || "").trim())
  ) ? segments : [];
}

export function muteVideoNativeAudio(params: Record<string, unknown>, useSeparateVoiceover: boolean) {
  if (!useSeparateVoiceover) return params;
  const next = { ...params };
  for (const key of ["audio", "generate_audio", "with_audio", "enable_audio"]) {
    if (key in next) next[key] = false;
  }
  return next;
}

export function changedStoryboardIndexes(before: string, after: string, expectedCount: number) {
  const previous = storyStoryboardSegments(before, expectedCount);
  const next = storyStoryboardSegments(after, expectedCount);
  if (next.length === 0) return [];
  if (previous.length === 0) return next.map((_, index) => index + 1);
  return next.flatMap((segment, index) => JSON.stringify(segment) === JSON.stringify(previous[index]) ? [] : [index + 1]);
}

const WORKFLOW_BY_CANVAS_TEMPLATE: Record<string, string> = {
  "content-image-post": "content_image_post",
  "story-short-video": "video_creation",
  "viral-remake": "viral_remake",
  "one-click-viral-remake": "one_click_viral_remake",
  "video-remake": "video_remake",
};

export function canvasTemplateEnabled(templateID: string, enabledWorkflowCodes: ReadonlySet<string> | null) {
  const workflowCode = WORKFLOW_BY_CANVAS_TEMPLATE[templateID];
  return !workflowCode || enabledWorkflowCodes === null || enabledWorkflowCodes.has(workflowCode);
}
