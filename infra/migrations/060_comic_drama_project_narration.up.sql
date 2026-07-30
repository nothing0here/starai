-- Every AI comic run belongs to a project, even when the user starts directly
-- from the workbench. Existing orphaned runs are backfilled without changing
-- their workflow or work records.
DO $$
DECLARE
  run RECORD;
  project_public_id TEXT;
  project_name TEXT;
BEGIN
  FOR run IN
    SELECT wp.id, wp.user_id, wp.inputs, wp.created_at, wp.updated_at
    FROM workflow_projects wp
    JOIN workflow_definitions wd ON wd.id = wp.workflow_id
    WHERE (wd.runtime_config->>'agent_mode' = 'comic_drama' OR wd.code = 'ai_comic_drama')
      AND COALESCE(wp.inputs->>'comic_project_id', '') = ''
  LOOP
    project_public_id := 'cdp_' || substr(md5(run.id::text || ':' || run.user_id::text || ':' || run.created_at::text), 1, 20);
    project_name := LEFT(COALESCE(NULLIF(BTRIM(run.inputs->>'user_prompt'), ''), NULLIF(BTRIM(run.inputs->>'prompt'), ''), '未命名漫剧'), 100);

    INSERT INTO comic_drama_projects
      (public_id, user_id, workflow_code, name, description, cover_url, style_snapshot,
       orientation, quality, last_workflow_project_id, created_at, updated_at)
    VALUES
      (project_public_id, run.user_id, 'ai_comic_drama', project_name, project_name,
       COALESCE(run.inputs->>'image_url', run.inputs->'reference_images'->>0, ''),
       COALESCE(run.inputs->'comic_style', '{}'::jsonb),
       COALESCE(NULLIF(run.inputs->>'orientation', ''), 'landscape'),
       COALESCE(NULLIF(UPPER(run.inputs->>'quality'), ''), '480P'),
       run.id, run.created_at, run.updated_at)
    ON CONFLICT (public_id) DO NOTHING;

    UPDATE workflow_projects
    SET inputs = jsonb_set(inputs, '{comic_project_id}', to_jsonb(project_public_id), true)
    WHERE id = run.id;

    UPDATE works
    SET metadata = jsonb_set(metadata, '{comic_project_id}', to_jsonb(project_public_id), true)
    WHERE user_id = run.user_id
      AND metadata->>'source' = 'ai_comic_drama'
      AND metadata->>'final_video_url' = (
        SELECT outputs->>'final_video_url' FROM workflow_projects WHERE id = run.id
      );
  END LOOP;
END $$;

UPDATE workflow_definitions
SET runtime_config = jsonb_set(runtime_config, '{narration_model_code}', '"audio_minimax_speech_28_hd"'::jsonb, true),
    nodes = CASE
      WHEN EXISTS (
        SELECT 1 FROM jsonb_array_elements(nodes) node WHERE node->>'id' = 'narrations'
      ) THEN nodes
      ELSE COALESCE(nodes, '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
        'id', 'narrations',
        'type', 'audio',
        'name', '对白与旁白配音',
        'model_code', 'audio_minimax_speech_28_hd',
        'cost', 0
      ))
    END,
    updated_at = now()
WHERE code = 'ai_comic_drama';
