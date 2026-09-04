-- Agent-callable content + image workflow, backed by the existing resumable
-- simple_pipeline runtime. The same concept is exposed as an editable canvas
-- template without introducing a second execution engine.

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
    ) AS image_code
)
INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order
)
SELECT
  'content_image_post',
  '内容创作',
  '面向微信公众号、小红书、今日头条等平台，生成标题、正文、标签和 2–6 张统一风格配图。',
  'image',
  '▦',
  '[]'::jsonb,
  '{"prompt":{"type":"string","required":true},"platform":{"type":"string"},"image_count":{"type":"integer","minimum":2,"maximum":6,"default":4},"aspect_ratio":{"type":"string","default":"3:4"}}'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{"theme":"cyan","hero_tags":["标题正文","多图配套","可进画布编辑"],"feature_tags":["内容结构化","2-6张配图","风格一致","失败可续传"],"steps":[{"icon":"🧠","title":"内容策划","subtitle":"整理平台、标题、正文、标签和卡片结构"},{"icon":"🧩","title":"卡片拆分","subtitle":"每张图对应独立信息点和视觉提示词"},{"icon":"🎨","title":"逐张生成","subtitle":"保持主体、色彩和版式风格一致"},{"icon":"📦","title":"图文交付","subtitle":"返回正文、标签、卡片文案和全部配图"}]}'::jsonb,
  jsonb_build_object(
    'agent_mode', 'simple_pipeline',
    'generation_type', 'image',
    'preset_code', 'content_image_post',
    'analysis_model_code', selected.analysis_code,
    'generation_model_code', selected.image_code,
    'candidate_count', 3,
    'default_count', 4,
    'creative_scenes', '["content_image_post"]'::jsonb,
    'input_capabilities', '{"allow_text_only":true,"require_reference_image":false,"support_multiple_references":true}'::jsonb,
    'flow_options', '{"enable_step_confirm":true,"enable_autopilot":true,"allow_prompt_edit":true}'::jsonb
  ),
  selected.analysis_code IS NOT NULL AND selected.image_code IS NOT NULL,
  18
FROM selected
ON CONFLICT (code) DO NOTHING;

UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
      COALESCE(workflow.display_config, '{}'::jsonb),
      '{canvas_templates}',
      CASE WHEN jsonb_typeof(workflow.display_config->'canvas_templates') = 'array' THEN workflow.display_config->'canvas_templates' ELSE '[]'::jsonb END ||
        '[{"id":"content-image-post","name":"内容创作","description":"文字内容与图片混合创作，适配公众号、小红书和今日头条","template_id":"content-image-post"}]'::jsonb,
      true
    ),
    updated_at = now()
WHERE workflow.code = 'infinite_canvas'
  AND NOT EXISTS (
    SELECT 1
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(workflow.display_config->'canvas_templates') = 'array' THEN workflow.display_config->'canvas_templates' ELSE '[]'::jsonb END) item
    WHERE item->>'id' = 'content-image-post'
  );
