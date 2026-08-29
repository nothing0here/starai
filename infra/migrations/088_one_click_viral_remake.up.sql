INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
) VALUES (
  'one_click_viral_remake',
  '一键爆款复刻',
  '导入 TikTok 爆款视频和商品图片，AI 自动拆解钩子、节奏与镜头结构，生成原创带货短视频。',
  'video',
  '✨',
  '[]'::jsonb,
  '{
    "type":"object",
    "properties":{
      "brief":{"type":"string","title":"商品卖点与复刻要求"},
      "reference_video":{"type":"string","title":"TikTok 爆款视频"},
      "product_images":{"type":"array","title":"商品图片","maxItems":9,"items":{"type":"string"}},
      "segment_count":{"type":"integer","title":"视频片段数","enum":[3,4,6],"default":3},
      "segment_duration":{"type":"integer","title":"单段时长","default":8}
    },
    "required":["reference_video","product_images"]
  }'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{
    "theme":"orange",
    "hero_tags":["TikTok 链接导入","爆款结构拆解","原创带货成片"],
    "feature_tags":["TikTok 视频解析","最多 9 张商品图","关键帧一致性","多片段自动合成"],
    "canvas_templates":[{
      "id":"one-click-viral-remake",
      "name":"一键爆款复刻",
      "description":"TikTok 视频 + 商品图 → 自动拆解 → 原创带货成片",
      "template_id":"one-click-viral-remake"
    }],
    "help":"粘贴公开的 TikTok 视频链接或上传参考视频，再添加 1-9 张商品图片和复刻要求。系统会拆解爆款结构，生成原创关键帧与视频片段并自动合成为完整短视频。"
  }'::jsonb,
  '{
    "agent_mode":"infinite_canvas",
    "generation_type":"video",
    "preset_code":"one_click_viral_remake",
    "system_workspace":true,
    "workspace_variant":"one_click_viral_remake",
    "default_template_id":"one-click-viral-remake",
    "default_segment_count":3,
    "default_segment_duration":8,
    "input_capabilities":{
      "allow_text_only":false,
      "support_tiktok_url":true,
      "support_reference_image":true,
      "support_multiple_references":true,
      "support_reference_video":true
    },
    "flow_options":{
      "enable_step_confirm":true,
      "enable_autopilot":true,
      "allow_prompt_edit":true
    }
  }'::jsonb,
  true,
  -91,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;
