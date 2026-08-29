INSERT INTO system_configs (key, value) VALUES
  ('customer_service_mode', '"builtin"'::jsonb),
  ('customer_service_custom_script', '""'::jsonb)
ON CONFLICT (key) DO NOTHING;
