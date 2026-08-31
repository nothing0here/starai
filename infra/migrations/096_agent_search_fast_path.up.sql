INSERT INTO system_configs (key, value) VALUES
  ('web_search_router_model_code', '""'::jsonb),
  ('web_search_cache_ttl_sec', '600'::jsonb)
ON CONFLICT (key) DO NOTHING;
