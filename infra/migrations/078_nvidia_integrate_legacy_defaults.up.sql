-- Some legacy NVIDIA models were saved without the optional provider label.
-- Detect the official Integrate base URL as well so their generation limits
-- are normalized too.
UPDATE models
SET default_params = default_params
  || CASE WHEN default_params ? 'temperature' THEN '{}'::jsonb ELSE '{"temperature": 1}'::jsonb END
  || CASE WHEN default_params ? 'top_p' THEN '{}'::jsonb ELSE '{"top_p": 0.95}'::jsonb END
  || CASE WHEN default_params ? 'max_tokens' THEN '{}'::jsonb ELSE '{"max_tokens": 1024}'::jsonb END
WHERE new_api_extra_params #>> '{connection,base_url}' = 'https://integrate.api.nvidia.com/v1'
  AND request_mode = 'chat_completions';
