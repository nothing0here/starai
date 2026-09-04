-- Manage content creation as a system workspace while keeping its Agent-callable
-- simple_pipeline runtime intact.

UPDATE workflow_definitions
SET runtime_config = COALESCE(runtime_config, '{}'::jsonb) || '{"system_workspace":true}'::jsonb,
    display_config = jsonb_set(
      COALESCE(display_config, '{}'::jsonb),
      '{canvas_templates}',
      '[{"id":"content-image-post","name":"内容创作","description":"文字内容与图片混合创作，适配公众号、小红书和今日头条","template_id":"content-image-post"}]'::jsonb,
      true
    ),
    updated_at = now()
WHERE code = 'content_image_post';
