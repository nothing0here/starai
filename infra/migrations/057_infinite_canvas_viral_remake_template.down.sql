UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE(
    (
      SELECT jsonb_agg(item)
      FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) AS item
      WHERE item->>'id' <> 'viral-remake'
    ),
    '[]'::jsonb
  ),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas';
