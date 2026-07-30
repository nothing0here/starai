-- Default template library for the built-in infinite canvas. Administrators
-- can edit this array later from the workflow advanced JSON editor.

UPDATE workflow_definitions
SET display_config = jsonb_set(
  COALESCE(display_config, '{}'::jsonb),
  '{canvas_templates}',
  '[
    {"id":"text-image","name":"文字生图片","description":"文本提示词连接图片生成节点","template_id":"text-image"},
    {"id":"image-image","name":"图片生图片","description":"参考图片连接图片生成节点","template_id":"image-image"},
    {"id":"text-image-mix","name":"文案与配图","description":"文字与参考图片共同生成新图片","template_id":"text-image-mix"},
    {"id":"multi-image","name":"多图对比","description":"多个参考素材连接双图片生成节点","template_id":"multi-image"},
    {"id":"text-video","name":"文字生视频","description":"文本提示词连接视频生成节点","template_id":"text-video"},
    {"id":"image-video","name":"图片生视频","description":"首帧或参考图片连接视频生成节点","template_id":"image-video"}
  ]'::jsonb,
  true
),
updated_at = now()
WHERE code = 'infinite_canvas'
  AND NOT (COALESCE(display_config, '{}'::jsonb) ? 'canvas_templates');
