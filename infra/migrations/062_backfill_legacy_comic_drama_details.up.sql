-- Migration 061 targets the current comic_plan output key. Older completed
-- projects stored the same structure under comic_drama, so backfill both.
WITH comic_runs AS (
  SELECT
    wp.id AS workflow_project_id,
    wp.inputs,
    COALESCE(wp.outputs->'comic_plan', wp.outputs->'comic_drama', '{}'::jsonb) AS plan,
    cp.id AS comic_project_id
  FROM workflow_projects wp
  JOIN comic_drama_projects cp
    ON cp.public_id = wp.inputs->>'comic_project_id'
   AND cp.user_id = wp.user_id
  JOIN workflow_definitions wd ON wd.id = wp.workflow_id
  WHERE wd.code = 'ai_comic_drama'
),
asset_rows AS (
  SELECT
    run.comic_project_id,
    run.inputs,
    source.asset_type,
    source.item,
    source.ordinality
  FROM comic_runs run
  CROSS JOIN LATERAL (
    SELECT 'character'::text AS asset_type, item, ordinality
    FROM jsonb_array_elements(COALESCE(run.plan->'characters', '[]'::jsonb))
      WITH ORDINALITY AS value(item, ordinality)
    UNION ALL
    SELECT 'prop'::text, item, ordinality
    FROM jsonb_array_elements(COALESCE(run.plan->'props', '[]'::jsonb))
      WITH ORDINALITY AS value(item, ordinality)
    UNION ALL
    SELECT 'location'::text, item, ordinality
    FROM jsonb_array_elements(COALESCE(run.plan->'locations', '[]'::jsonb))
      WITH ORDINALITY AS value(item, ordinality)
  ) source
)
INSERT INTO comic_drama_assets
  (public_id, project_id, asset_type, asset_code, name, description, visual_prompt,
   reference_asset_ids, metadata, status, created_at, updated_at)
SELECT
  'cda_' || substr(md5(asset_rows.comic_project_id::text || ':' || asset_rows.asset_type || ':' ||
    COALESCE(NULLIF(asset_rows.item->>'code', ''), upper(asset_rows.asset_type) || '_' || lpad(asset_rows.ordinality::text, 2, '0'))), 1, 24),
  asset_rows.comic_project_id,
  asset_rows.asset_type,
  COALESCE(NULLIF(asset_rows.item->>'code', ''), upper(asset_rows.asset_type) || '_' || lpad(asset_rows.ordinality::text, 2, '0')),
  COALESCE(NULLIF(asset_rows.item->>'name', ''), NULLIF(asset_rows.item->>'code', ''), '未命名资产'),
  COALESCE(asset_rows.item->>'description', ''),
  COALESCE(asset_rows.item->>'visual_prompt', ''),
  CASE
    WHEN asset_rows.asset_type = 'character' AND asset_rows.ordinality = 1
      THEN COALESCE(asset_rows.item->'reference_asset_ids', asset_rows.inputs->'reference_asset_ids', '[]'::jsonb)
    ELSE COALESCE(asset_rows.item->'reference_asset_ids', '[]'::jsonb)
  END,
  asset_rows.item,
  'ready',
  now(),
  now()
FROM asset_rows
ON CONFLICT (project_id, asset_type, asset_code) DO NOTHING;

WITH comic_runs AS (
  SELECT
    wp.id AS workflow_project_id,
    COALESCE(wp.outputs->'comic_plan', wp.outputs->'comic_drama', '{}'::jsonb) AS plan,
    cp.id AS comic_project_id
  FROM workflow_projects wp
  JOIN comic_drama_projects cp
    ON cp.public_id = wp.inputs->>'comic_project_id'
   AND cp.user_id = wp.user_id
  JOIN workflow_definitions wd ON wd.id = wp.workflow_id
  WHERE wd.code = 'ai_comic_drama'
)
INSERT INTO comic_drama_storyboards
  (project_id, workflow_project_id, shot_id, seq, title, duration_sec,
   character_codes, prop_codes, location_code, data, created_at, updated_at)
SELECT
  run.comic_project_id,
  run.workflow_project_id,
  COALESCE(NULLIF(storyboard.item->>'id', ''), 'S' || lpad(storyboard.ordinality::text, 2, '0')),
  storyboard.ordinality - 1,
  COALESCE(storyboard.item->>'title', ''),
  CASE
    WHEN COALESCE(storyboard.item->>'duration_sec', '') ~ '^[0-9]+([.][0-9]+)?$'
      THEN (storyboard.item->>'duration_sec')::numeric
    ELSE 5
  END,
  COALESCE(storyboard.item->'character_codes', '[]'::jsonb),
  COALESCE(storyboard.item->'prop_codes', '[]'::jsonb),
  COALESCE(storyboard.item->>'location_code', ''),
  storyboard.item,
  now(),
  now()
FROM comic_runs run
CROSS JOIN LATERAL
  jsonb_array_elements(COALESCE(run.plan->'storyboards', '[]'::jsonb))
  WITH ORDINALITY AS storyboard(item, ordinality)
ON CONFLICT (workflow_project_id, shot_id) DO NOTHING;
