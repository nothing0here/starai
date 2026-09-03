WITH retention AS (
  SELECT COALESCE((
    SELECT CASE
      WHEN jsonb_typeof(value) = 'number' THEN GREATEST(0, (value #>> '{}')::numeric::integer)
      WHEN jsonb_typeof(value) = 'string' AND (value #>> '{}') ~ '^\s*\d+\s*$' THEN (trim(value #>> '{}'))::integer
      ELSE 7
    END
    FROM system_configs
    WHERE key = 'work_retention_days'
  ), 7) AS days
)
UPDATE works
SET expires_at = now() + make_interval(days => retention.days)
FROM retention
WHERE works.expires_at IS NULL
  AND retention.days > 0;
