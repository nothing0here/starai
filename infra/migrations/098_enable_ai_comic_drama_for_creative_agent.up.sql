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
    ) AS narration_code
)
UPDATE workflow_definitions AS workflow
SET name = 'AI 漫剧 - S2.0',
    description = '输入故事或角色参考图，AI 自动完成剧本、分镜、关键帧、分段视频、配音和成片合成。',
    runtime_config = workflow.runtime_config || jsonb_build_object(
      'analysis_model_code', selected.analysis_code,
      'dialogue_model_codes', jsonb_build_array(selected.analysis_code),
      'image_model_code', selected.image_code,
      'video_model_code', selected.video_code,
      'generation_model_code', selected.video_code,
      'narration_model_code', selected.narration_code,
      'audio_strategy', CASE WHEN selected.narration_code IS NULL THEN 'video_native' ELSE 'hybrid' END
    ),
    nodes = (
      SELECT jsonb_agg(
        CASE node->>'id'
          WHEN 'comic_plan' THEN jsonb_set(node, '{model_code}', to_jsonb(selected.analysis_code), true)
          WHEN 'keyframes' THEN jsonb_set(node, '{model_code}', to_jsonb(selected.image_code), true)
          WHEN 'video_segments' THEN jsonb_set(node, '{model_code}', to_jsonb(selected.video_code), true)
          WHEN 'narrations' THEN jsonb_set(node, '{model_code}', to_jsonb(COALESCE(selected.narration_code, '')), true)
          ELSE node
        END
        ORDER BY ordinality
      )
      FROM jsonb_array_elements(workflow.nodes) WITH ORDINALITY AS item(node, ordinality)
    ),
    is_enabled = selected.analysis_code IS NOT NULL AND selected.image_code IS NOT NULL AND selected.video_code IS NOT NULL,
    updated_at = now()
FROM selected
WHERE workflow.code = 'ai_comic_drama';
