-- Global switch for the public API documentation center.
INSERT INTO system_configs (key, value)
VALUES ('api_docs_enabled', 'true'::jsonb)
ON CONFLICT (key) DO NOTHING;
