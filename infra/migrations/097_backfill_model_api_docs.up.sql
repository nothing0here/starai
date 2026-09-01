-- Backfill API documentation for models created after the original API docs migration.

INSERT INTO api_docs (
  model_id,
  slug,
  title,
  summary,
  protocol,
  endpoint,
  sdk,
  content,
  is_published,
  sort_order
)
SELECT
  id,
  code,
  display_name,
  COALESCE(description, ''),
  CASE
    WHEN request_mode IN ('chat_completions', 'responses') THEN 'openai-compatible'
    WHEN request_mode = 'images' THEN 'async-image-task'
    WHEN request_mode = 'video' THEN 'async-video-task'
    WHEN request_mode = 'audio' THEN 'async-audio-task'
    ELSE 'custom-compatible'
  END,
  CASE
    WHEN request_mode = 'chat_completions' THEN '/v1/chat/completions'
    WHEN request_mode = 'responses' THEN '/v1/responses'
    WHEN request_mode = 'images' THEN '/v1/images/generations'
    WHEN request_mode = 'video' THEN '/v1/video/generations'
    WHEN request_mode = 'audio' THEN '/v1/audio/speech'
    ELSE COALESCE(NULLIF(new_api_endpoint, ''), '/v1/chat/completions')
  END,
  'curl',
  '{"auto_generated":true}'::jsonb,
  true,
  sort_order
FROM models
ON CONFLICT DO NOTHING;
