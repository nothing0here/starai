-- Nemotron models use NVIDIA's OpenAI-compatible endpoint, but require an
-- explicit chat-template flag to reliably suppress upstream reasoning when
-- the workspace Deep Think switch is off. Apply the mapping to legacy and
-- newly-created Nemotron models that do not already have custom reasoning.
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
WHERE new_api_model LIKE 'nvidia/nemotron-%'
  AND new_api_extra_params #>> '{connection,base_url}' = 'https://integrate.api.nvidia.com/v1'
  AND request_mode = 'chat_completions'
  AND NOT (runtime_rule ? 'reasoning');
