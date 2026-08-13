-- Existing NVIDIA Integrate chat models created before the preset did not
-- receive generation limits or the Ultra thinking mapping. Keep every custom
-- administrator value and only add missing safe defaults.
UPDATE models
SET default_params = default_params
  || CASE WHEN default_params ? 'temperature' THEN '{}'::jsonb ELSE '{"temperature": 1}'::jsonb END
  || CASE WHEN default_params ? 'top_p' THEN '{}'::jsonb ELSE '{"top_p": 0.95}'::jsonb END
  || CASE WHEN default_params ? 'max_tokens' THEN '{}'::jsonb ELSE '{"max_tokens": 1024}'::jsonb END
WHERE new_api_extra_params #>> '{connection,provider}' = 'nvidia'
  AND request_mode = 'chat_completions';

UPDATE models
SET runtime_rule = runtime_rule
  || jsonb_build_object(
    'capabilities', COALESCE(runtime_rule->'capabilities', '{}'::jsonb) || '{"deep_think": true}'::jsonb,
    'reasoning', jsonb_build_object(
      'mode', 'nvidia_chat_template',
      'default_enabled', false,
      'default_budget', 1024,
      'max_budget', 4096
    )
  )
WHERE new_api_model = 'nvidia/nemotron-3-ultra-550b-a55b'
  AND new_api_extra_params #>> '{connection,provider}' = 'nvidia'
  AND NOT (runtime_rule ? 'reasoning');
