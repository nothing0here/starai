-- Per-section visibility switches for the public API documentation center.
INSERT INTO system_configs (key, value)
VALUES (
  'api_docs_operations',
  '{"chat": true, "image": true, "video": true, "audio": true, "platform": true}'::jsonb
)
ON CONFLICT (key) DO NOTHING;
