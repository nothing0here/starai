UPDATE workflow_definitions
SET runtime_config = COALESCE(runtime_config, '{}'::jsonb) - 'system_workspace',
    display_config = COALESCE(display_config, '{}'::jsonb) - 'canvas_templates',
    updated_at = now()
WHERE code = 'content_image_post';
