UPDATE workflow_definitions
SET runtime_config = jsonb_set(
      runtime_config,
      '{audio_strategy}',
      to_jsonb(
        CASE
          WHEN COALESCE(runtime_config->>'narration_model_code', '') <> '' THEN 'hybrid'
          ELSE 'video_native'
        END
      ),
      true
    ),
    updated_at = now()
WHERE runtime_config->>'agent_mode' = 'comic_drama'
  AND COALESCE(runtime_config->>'audio_strategy', '') NOT IN ('video_native', 'tts_only', 'hybrid');
