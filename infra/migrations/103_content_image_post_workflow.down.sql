UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
      COALESCE(workflow.display_config, '{}'::jsonb),
      '{canvas_templates}',
      COALESCE((
        SELECT jsonb_agg(item ORDER BY ordinal)
        FROM jsonb_array_elements(CASE WHEN jsonb_typeof(workflow.display_config->'canvas_templates') = 'array' THEN workflow.display_config->'canvas_templates' ELSE '[]'::jsonb END)
          WITH ORDINALITY AS template(item, ordinal)
        WHERE item->>'id' <> 'content-image-post'
      ), '[]'::jsonb),
      true
    ),
    updated_at = now()
WHERE workflow.code = 'infinite_canvas';

DELETE FROM workflow_definitions WHERE code = 'content_image_post';
