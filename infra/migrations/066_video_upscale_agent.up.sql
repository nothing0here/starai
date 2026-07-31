WITH upscale_model AS (
  SELECT code
  FROM models
  WHERE category = 'video'
    AND is_enabled = true
    AND (
      lower(code) LIKE '%upscale%'
      OR lower(code) LIKE '%video_enhance%'
      OR lower(code) LIKE '%super_resolution%'
      OR lower(display_name) LIKE '%upscale%'
      OR display_name LIKE '%超分%'
      OR lower(runtime_rule::text) LIKE '%upscale%'
      OR lower(runtime_rule::text) LIKE '%super_resolution%'
    )
  ORDER BY
    CASE
      WHEN lower(code) LIKE '%upscale%' OR lower(runtime_rule::text) LIKE '%upscale%' THEN 0
      ELSE 1
    END,
    sort_order,
    id
  LIMIT 1
),
selected AS (
  SELECT COALESCE((SELECT code FROM upscale_model), '') AS model_code
)
INSERT INTO workflow_definitions (
  code,
  name,
  description,
  icon,
  category,
  nodes,
  input_schema,
  price_rule,
  display_config,
  runtime_config,
  is_enabled,
  sort_order,
  created_at,
  updated_at
)
SELECT
  'video_upscale',
  '一键视频高清',
  '上传低清视频，通过 AI 超分增强输出 720P、1K 或 2K 高清视频。',
  '✨',
  'video',
  jsonb_build_array(
    jsonb_build_object(
      'id', 'upscale',
      'type', 'video',
      'name', 'AI 视频高清',
      'model_code', selected.model_code,
      'prompt_template', '',
      'cost', 0
    )
  ),
  jsonb_build_object(
    'type', 'object',
    'required', jsonb_build_array('video_url', 'target_resolution'),
    'properties', jsonb_build_object(
      'video_url', jsonb_build_object('type', 'string', 'title', '源视频'),
      'target_resolution', jsonb_build_object('type', 'string', 'title', '目标清晰度', 'enum', jsonb_build_array('720P', '1K', '2K'), 'default', '720P'),
      'preserve_audio', jsonb_build_object('type', 'boolean', 'title', '保留原音', 'default', true),
      'enhancement_mode', jsonb_build_object('type', 'string', 'title', '增强模式', 'enum', jsonb_build_array('balanced', 'detail', 'denoise'), 'default', 'balanced')
    )
  ),
  jsonb_build_object('billing_type', 'model_actual', 'unit_price', 0),
  jsonb_build_object(
    'theme', 'cyan',
    'hero_tags', jsonb_build_array('AI超分', '视频高清', '画质增强'),
    'feature_tags', jsonb_build_array('720P', '1K', '2K', '保留原音'),
    'steps', jsonb_build_array(
      jsonb_build_object('icon', '🎬', 'title', '上传源视频', 'subtitle', '上传文件或从资产库引用已有视频', 'tags', jsonb_build_array('格式校验', '时长检查')),
      jsonb_build_object('icon', '✨', 'title', 'AI 超分增强', 'subtitle', '降噪、去压缩瑕疵并恢复自然细节', 'tags', jsonb_build_array('模型计费', '实时进度')),
      jsonb_build_object('icon', '📥', 'title', '高清结果', 'subtitle', '在线预览并下载高清成片', 'tags', jsonb_build_array('720P', '1K', '2K'))
    ),
    'input', jsonb_build_object('image_label', '源视频', 'placeholder', '可选：补充降噪、人物细节或画面增强要求', 'modes', jsonb_build_array('智能托管')),
    'help', '上传源视频或从资产库选择视频，选择目标清晰度后运行。系统会保留原始内容、时长和构图，并按后台配置的超分模型实际计费。'
  ),
  jsonb_build_object(
    'agent_mode', 'video_upscale',
    'generation_type', 'video',
    'preset_code', 'video_upscale',
    'generation_model_code', selected.model_code,
    'supported_resolutions', jsonb_build_array('720P', '1K', '2K'),
    'default_target_resolution', '720P',
    'preserve_audio', true,
    'default_enhancement_mode', 'balanced',
    'max_input_duration_sec', 300,
    'max_input_size_mb', 500,
    'upscale_operation', 'upscale',
    'default_count', 1,
    'input_capabilities', jsonb_build_object(
      'allow_text_only', false,
      'support_reference_image', false,
      'support_multiple_references', false,
      'support_first_last_frame', false
    ),
    'flow_options', jsonb_build_object(
      'enable_step_confirm', false,
      'enable_autopilot', true,
      'allow_prompt_edit', true
    )
  ),
  selected.model_code <> '',
  76,
  now(),
  now()
FROM selected
ON CONFLICT (code) DO NOTHING;
