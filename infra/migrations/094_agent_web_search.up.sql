INSERT INTO system_configs (key, value) VALUES
  ('web_search_enabled', 'false'::jsonb),
  ('web_search_provider', '"tavily"'::jsonb),
  ('web_search_api_key', '""'::jsonb),
  ('web_search_base_url', '""'::jsonb),
  ('web_search_max_results', '5'::jsonb),
  ('web_search_timeout_sec', '12'::jsonb),
  ('web_search_daily_limit', '100'::jsonb)
ON CONFLICT (key) DO NOTHING;
