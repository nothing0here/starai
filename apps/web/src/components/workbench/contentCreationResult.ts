function textValue(value: unknown) {
  if (Array.isArray(value)) return value.map(String).filter(Boolean).join(" ");
  return typeof value === "string" || typeof value === "number" ? String(value).trim() : "";
}

export function socialPublishText(raw: string) {
  const source = raw.trim();
  if (!source) return "";

  const unfenced = source.replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, "").trim();
  try {
    const parsed = JSON.parse(unfenced) as Record<string, unknown>;
    const post = (parsed.content_post && typeof parsed.content_post === "object"
      ? parsed.content_post
      : parsed) as Record<string, unknown>;
    const sections = [
      textValue(post.title),
      textValue(post.hook),
      textValue(post.body ?? post.content),
      textValue(post.cta),
      textValue(post.hashtags ?? post.tags),
    ].filter(Boolean);
    if (sections.length) return sections.join("\n\n");
  } catch {
    // Plain text and Markdown are the normal canvas output.
  }

  return source.split(/\n\s*(?:---\s*)?(?:#{1,6}\s*)?(?:配图(?:规划|方案|提示词)|image plan|visual plan)\s*(?:---|[:：])?/i, 1)[0].trim();
}
