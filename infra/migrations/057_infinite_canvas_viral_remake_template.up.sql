-- Add the executable viral-remake canvas workflow without overwriting
-- templates that administrators have already customized.

UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb) ||
  jsonb_build_array(
    jsonb_build_object(
      'id', 'viral-remake',
      'name', '爆款复刻',
      'description', '爆款参考与品牌素材生成复刻主视觉并延展为短视频',
      'template_id', 'viral-remake'
    )
  ),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas'
  AND NOT EXISTS (
    SELECT 1
    FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) AS existing(item)
    WHERE existing.item->>'id' = 'viral-remake'
  );
