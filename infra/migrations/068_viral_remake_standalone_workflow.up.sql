ALTER TABLE infinite_canvases
  ADD COLUMN IF NOT EXISTS workflow_code VARCHAR(64) NOT NULL DEFAULT 'infinite_canvas';

CREATE INDEX IF NOT EXISTS idx_infinite_canvases_user_workflow_updated
  ON infinite_canvases (user_id, workflow_code, updated_at DESC);

INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
) VALUES (
  'viral_remake',
  '爆款复刻',
  '上传爆款参考视频与品牌素材，AI 多模态拆解内容结构，生成原创关键帧、多段视频并合成为完整短视频。',
  'video',
  '🔁',
  '[]'::jsonb,
  '{
    "type":"object",
    "properties":{
      "brief":{"type":"string","title":"复刻目标"},
      "reference_video":{"type":"string","title":"爆款参考视频"},
      "brand_assets":{"type":"array","title":"品牌素材","items":{"type":"string"}},
      "segment_count":{"type":"integer","title":"视频片段数","enum":[3,4,6],"default":3},
      "segment_duration":{"type":"integer","title":"单段时长","default":8}
    },
    "required":["reference_video"]
  }'::jsonb,
  '{"billing_type":"model_actual","unit_price":0}'::jsonb,
  '{
    "theme":"orange",
    "hero_tags":["爆款拆解","原创复刻","多片段成片"],
    "feature_tags":["参考视频分析","品牌素材融合","关键帧一致性","多段视频合成"],
    "canvas_templates":[
      {
        "id":"viral-remake",
        "name":"爆款复刻",
        "description":"多模态拆解爆款参考，生成多关键帧、多片段并合成为原创短视频",
        "template_id":"viral-remake"
      }
    ],
    "help":"上传爆款参考视频和品牌素材，填写复刻目标。系统会先拆解结构、节奏、镜头和视觉语言，再生成原创关键帧与视频片段，最终合成为完整短视频。"
  }'::jsonb,
  '{
    "agent_mode":"infinite_canvas",
    "generation_type":"video",
    "preset_code":"viral_remake",
    "system_workspace":true,
    "workspace_variant":"viral_remake",
    "default_template_id":"viral-remake",
    "default_segment_count":3,
    "default_segment_duration":8,
    "input_capabilities":{
      "allow_text_only":false,
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
  -90,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;

-- Preserve an administrator-customized template definition while promoting it
-- out of the generic infinite-canvas template library.
WITH source_template AS (
  SELECT item
  FROM workflow_definitions AS source,
       jsonb_array_elements(COALESCE(source.display_config->'canvas_templates', '[]'::jsonb)) AS item
  WHERE source.code = 'infinite_canvas'
    AND item->>'id' = 'viral-remake'
  LIMIT 1
)
UPDATE workflow_definitions AS standalone
SET display_config = jsonb_set(
      COALESCE(standalone.display_config, '{}'::jsonb),
      '{canvas_templates}',
      jsonb_build_array(source_template.item || '{"template_id":"viral-remake"}'::jsonb),
      true
    ),
    updated_at = now()
FROM source_template
WHERE standalone.code = 'viral_remake';

UPDATE workflow_definitions AS source
SET display_config = jsonb_set(
      COALESCE(source.display_config, '{}'::jsonb),
      '{canvas_templates}',
      COALESCE(
        (
          SELECT jsonb_agg(item)
          FROM jsonb_array_elements(COALESCE(source.display_config->'canvas_templates', '[]'::jsonb)) AS item
          WHERE item->>'id' <> 'viral-remake'
        ),
        '[]'::jsonb
      ),
      true
    ),
    updated_at = now()
WHERE source.code = 'infinite_canvas';
