-- Expose the existing video-creation canvas as a first-class, configurable agent.
WITH general AS (
  SELECT runtime_config
  FROM workflow_definitions
  WHERE code = 'general_creative_agent'
), selected AS (
  SELECT
    COALESCE(
      (SELECT m.code FROM models m, general g WHERE m.code = g.runtime_config->>'analysis_model_code' AND m.is_enabled AND m.category = 'chat' AND m.request_mode IN ('chat_completions', 'responses') LIMIT 1),
      (SELECT m.code FROM models m WHERE m.is_enabled AND m.category = 'chat' AND m.code <> 'multi_collab_chat' AND m.request_mode IN ('chat_completions', 'responses') ORDER BY m.sort_order, m.id LIMIT 1)
    ) AS analysis_code,
    COALESCE(
      (SELECT m.code FROM models m, general g WHERE m.code = g.runtime_config->>'image_model_code' AND m.is_enabled AND m.request_mode = 'images' LIMIT 1),
      (SELECT m.code FROM models m WHERE m.is_enabled AND m.request_mode = 'images' ORDER BY m.sort_order, m.id LIMIT 1)
    ) AS image_code,
    COALESCE(
      (SELECT m.code FROM models m, general g WHERE m.code = g.runtime_config->>'video_model_code' AND m.is_enabled AND m.request_mode = 'video' LIMIT 1),
      (SELECT m.code FROM models m WHERE m.is_enabled AND m.request_mode = 'video' ORDER BY m.sort_order, m.id LIMIT 1)
    ) AS video_code,
    COALESCE(
      (SELECT m.code FROM models m, general g WHERE m.code = g.runtime_config->>'speech_model_code' AND m.is_enabled AND m.request_mode = 'audio' LIMIT 1),
      (SELECT m.code FROM models m WHERE m.is_enabled AND m.request_mode = 'audio' AND lower(m.code || ' ' || m.display_name) ~ '(speech|tts|voice|语音|配音)' ORDER BY m.sort_order, m.id LIMIT 1)
    ) AS audio_code
)
INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order
)
SELECT
  'video_creation',
  '视频创作',
  '输入创作需求，生成视频脚本、分镜、关键帧、视频片段、配音与完整成片。',
  'video',
  '🎬',
  '[]'::jsonb,
  '{"prompt":{"type":"string","required":true},"reference_images":{"type":"array"},"reference_videos":{"type":"array"},"reference_audios":{"type":"array"}}'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{"theme":"blue","hero_tags":["脚本分镜","多段视频","配音成片"],"feature_tags":["分镜确认","单镜重跑","多模型可选","失败可续传"],"canvas_templates":[{"id":"story-short-video","name":"视频创作","description":"创作需求生成视频脚本、分镜、关键帧、视频片段与完整成片","template_id":"story-short-video"}],"steps":[{"icon":"📝","title":"脚本创作","subtitle":"根据视频类型与发布平台生成完整脚本"},{"icon":"🎞️","title":"分镜确认","subtitle":"拆解结构化分镜，确认后再生成媒体"},{"icon":"🎨","title":"逐镜生成","subtitle":"生成关键帧、视频片段与多角色配音"},{"icon":"🎬","title":"合成成片","subtitle":"自动拼接片段并合成配音"}]}'::jsonb,
  jsonb_build_object(
    'agent_mode', 'infinite_canvas',
    'system_workspace', true,
    'generation_type', 'video',
    'preset_code', 'video_creation',
    'analysis_model_code', selected.analysis_code,
    'image_model_code', selected.image_code,
    'video_model_code', selected.video_code,
    'audio_model_code', selected.audio_code,
    'default_segment_count', 4,
    'default_segment_duration', 8,
    'default_story_review_required', true
  ),
  selected.analysis_code IS NOT NULL AND selected.image_code IS NOT NULL AND selected.video_code IS NOT NULL,
  17
FROM selected
ON CONFLICT (code) DO NOTHING;
