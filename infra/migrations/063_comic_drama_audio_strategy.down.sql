UPDATE workflow_definitions
SET runtime_config = runtime_config - 'audio_strategy',
    updated_at = now()
WHERE runtime_config->>'agent_mode' = 'comic_drama';
