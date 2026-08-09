INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
) VALUES (
  'video_remake',
  '视频复刻',
  '上传原片、品牌或商品素材与可选原片音频，AI 自动拆镜、替换主体、生成分段视频并合成为完整成片。',
  'video',
  '🎬',
  '[]'::jsonb,
  '{
    "type":"object",
    "properties":{
      "brief":{"type":"string","title":"视频复刻目标"},
      "reference_video":{"type":"string","title":"原片参考视频"},
      "brand_assets":{"type":"array","title":"品牌 / 商品素材","items":{"type":"string"}},
      "source_audio":{"type":"string","title":"原片音频（可选）"},
      "segment_count":{"type":"integer","title":"视频片段数","enum":[3,4,6],"default":3},
      "segment_duration":{"type":"integer","title":"单段时长","default":8}
    },
    "required":["reference_video"]
  }'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{
    "theme":"violet",
    "hero_tags":["智能拆镜","产品替换","原片节奏复刻"],
    "feature_tags":["参考视频分镜分析","商品与主体一致性","原片音频节奏对齐","多片段音画合成"],
    "canvas_templates":[{
      "id":"video-remake",
      "name":"视频复刻",
      "description":"原片拆镜、替换商品或主体、按原片节奏分段生成并完成音画合成",
      "template_id":"video-remake"
    }],
    "help":"上传待复刻的原视频、需要保留外观的商品或品牌素材；还可上传从原片提取的音频。系统会智能拆镜并提取结构化镜头脚本，生成替换后的分镜首帧，按模型时长分段生成视频，最后拼接片段并合成原片音频。"
  }'::jsonb,
  '{
    "agent_mode":"infinite_canvas",
    "generation_type":"video",
    "preset_code":"video_remake",
    "system_workspace":true,
    "workspace_variant":"video_remake",
    "default_template_id":"video-remake",
    "default_segment_count":3,
    "default_segment_duration":8,
    "input_capabilities":{
      "allow_text_only":false,
      "support_reference_image":true,
      "support_multiple_references":true,
      "support_reference_video":true,
      "support_reference_audio":true
    },
    "flow_options":{
      "enable_step_confirm":true,
      "enable_autopilot":true,
      "allow_prompt_edit":true
    }
  }'::jsonb,
  true,
  -89,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;
