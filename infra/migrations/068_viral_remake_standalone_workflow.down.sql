WITH standalone_template AS (
  SELECT item
  FROM workflow_definitions AS standalone,
       jsonb_array_elements(COALESCE(standalone.display_config->'canvas_templates', '[]'::jsonb)) AS item
  WHERE standalone.code = 'viral_remake'
    AND item->>'id' = 'viral-remake'
  LIMIT 1
)
UPDATE workflow_definitions AS target
SET display_config = jsonb_set(
      COALESCE(target.display_config, '{}'::jsonb),
      '{canvas_templates}',
      COALESCE(target.display_config->'canvas_templates', '[]'::jsonb)
        || jsonb_build_array(standalone_template.item),
      true
    ),
    updated_at = now()
FROM standalone_template
WHERE target.code = 'infinite_canvas'
  AND NOT EXISTS (
    SELECT 1
    FROM jsonb_array_elements(COALESCE(target.display_config->'canvas_templates', '[]'::jsonb)) AS existing(item)
    WHERE existing.item->>'id' = 'viral-remake'
  );

DELETE FROM workflow_definitions WHERE code = 'viral_remake';

DROP INDEX IF EXISTS idx_infinite_canvases_user_workflow_updated;

ALTER TABLE infinite_canvases
  DROP COLUMN IF EXISTS workflow_code;
