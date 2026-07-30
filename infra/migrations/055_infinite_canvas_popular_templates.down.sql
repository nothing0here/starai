UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  (
    SELECT COALESCE(jsonb_agg(existing.item), '[]'::jsonb)
    FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) AS existing(item)
    WHERE existing.item->>'id' NOT IN (
      'ecommerce-visual-pack',
      'social-campaign',
      'product-showcase-video',
      'brand-visual-kit',
      'photo-restoration',
      'story-short-video'
    )
  ),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas';
