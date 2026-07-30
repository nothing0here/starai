UPDATE workflow_definitions
SET display_config = COALESCE(display_config, '{}'::jsonb) - 'canvas_templates',
    updated_at = now()
WHERE code = 'infinite_canvas';
