DELETE FROM workflow_definitions
WHERE code = 'infinite_canvas'
  AND runtime_config->>'system_workspace' = 'true';
