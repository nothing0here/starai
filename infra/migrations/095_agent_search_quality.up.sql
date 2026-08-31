INSERT INTO system_configs (key, value) VALUES
  ('agent_default_timezone', '"Asia/Shanghai"'::jsonb),
  ('web_search_depth', '"basic"'::jsonb)
ON CONFLICT (key) DO NOTHING;
