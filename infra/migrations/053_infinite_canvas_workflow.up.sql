-- Register the built-in infinite canvas in the same workflow catalog used by
-- Admin > Workflows. Runtime execution remains handled by the canvas workspace.

INSERT INTO workflow_definitions (
  code,
  name,
  description,
  category,
  icon,
  nodes,
  input_schema,
  price_rule,
  display_config,
  runtime_config,
  is_enabled,
  sort_order
) VALUES (
  'infinite_canvas',
  '无限画布',
  '自由连接文字、图片和视频生成节点，搭建可复用的 AI 创作流程',
  'image',
  '∞',
  '[]'::jsonb,
  '{}'::jsonb,
  '{"billing_type":"dynamic"}'::jsonb,
  '{
    "theme":"cyan",
    "hero_tags":["无限画布","节点编排","多模型创作"],
    "feature_tags":["文字与素材节点","图片与视频生成","保存与历史","导入与导出"]
  }'::jsonb,
  '{
    "agent_mode":"infinite_canvas",
    "generation_type":"image",
    "system_workspace":true
  }'::jsonb,
  true,
  -100
)
ON CONFLICT (code) DO NOTHING;
