-- A rollback cannot reconstruct an administrator's previous malformed rule.
-- Restore a conservative per-second rule only for rows managed by this migration.
UPDATE models
SET price_rule = '{"billing_type":"per_second","currency":"¥","unit_price":28}'::jsonb,
    updated_at = now()
WHERE category = 'video'
  AND price_rule ->> 'billing_type' = 'dynamic'
  AND price_rule ->> 'strategy' = 'seedance_2_tokens'
  AND (
    runtime_rule #>> '{upstream,adapter}' = 'volcengine_seedance_2'
    OR runtime_rule #>> '{video,upload_profile}' = 'seedance_2'
  );
