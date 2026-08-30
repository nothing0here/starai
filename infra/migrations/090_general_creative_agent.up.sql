INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
) VALUES (
  'general_creative_agent',
  '通用智能体',
  '通过聊天理解创作意图，自动生成图片或视频任务。',
  'tool',
  '✦',
  '[]'::jsonb,
  '{"type":"object","properties":{"prompt":{"type":"string","title":"创作需求"},"asset_ids":{"type":"array","title":"参考素材"}}}'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{"theme":"amber","hero_tags":["聊天创作","图片生成","视频生成"],"feature_tags":["自然语言理解","自动选择模型","支持参考素材","生成前可确认"],"help":"直接描述你的创作需求，通用智能体会理解意图并生成图片或视频任务。"}'::jsonb,
  '{"agent_mode":"creative_chat","generation_type":"mixed","input_capabilities":{"allow_text_only":true,"support_reference_image":true,"support_reference_video":true,"support_reference_audio":true},"flow_options":{"enable_step_confirm":true,"enable_autopilot":true,"allow_prompt_edit":true}}'::jsonb,
  true,
  -100,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;
