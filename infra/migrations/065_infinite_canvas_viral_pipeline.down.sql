UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE((
    SELECT jsonb_agg(
      CASE
        WHEN item->>'id' = 'viral-remake'
          AND item->>'name' = '爆款复刻'
          AND item->>'description' = '多模态拆解爆款参考，生成多关键帧、多片段并合成为原创短视频'
          THEN item || '{"description":"分析复刻需求，生成主视觉并延展为短视频"}'::jsonb
        ELSE item
      END
      ORDER BY ordinal
    )
    FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb))
      WITH ORDINALITY AS template(item, ordinal)
  ), '[]'::jsonb),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas'
  AND jsonb_typeof(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) = 'array';
