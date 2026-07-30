UPDATE workflow_definitions
SET runtime_config = runtime_config - 'narration_model_code',
    nodes = COALESCE((
      SELECT jsonb_agg(node ORDER BY ord)
      FROM jsonb_array_elements(nodes) WITH ORDINALITY AS item(node, ord)
      WHERE node->>'id' <> 'narrations'
    ), '[]'::jsonb),
    updated_at = now()
WHERE code = 'ai_comic_drama';
