-- Seedance 2.0 is billed by generated output tokens, not by a flat 28 credits/second.
-- Preserve administrators' existing dynamic rules; only repair legacy/non-dynamic rows.
UPDATE models
SET price_rule = '{
  "billing_type": "dynamic",
  "strategy": "seedance_2_tokens",
  "currency": "¥",
  "points_per_cny": 1,
  "platform_multiplier": 1,
  "default_resolution": "720p",
  "default_input_video_seconds": 4,
  "video_min_token_multiplier": 1.8,
  "tokens_per_second": {
    "480p": 10044,
    "720p": 21600,
    "1080p": 48600,
    "4k": 194400
  },
  "rates_per_m_tokens": {
    "480p": {"without_video": 46, "with_video": 28},
    "720p": {"without_video": 46, "with_video": 28},
    "1080p": {"without_video": 51, "with_video": 31},
    "4k": {"without_video": 26, "with_video": 16}
  },
  "fallback_cost": 4.97
}'::jsonb,
updated_at = now()
WHERE category = 'video'
  AND (
    runtime_rule #>> '{upstream,adapter}' = 'volcengine_seedance_2'
    OR runtime_rule #>> '{video,upload_profile}' = 'seedance_2'
  )
  AND (
    price_rule ->> 'billing_type' IS DISTINCT FROM 'dynamic'
    OR price_rule ->> 'strategy' IS DISTINCT FROM 'seedance_2_tokens'
  );
