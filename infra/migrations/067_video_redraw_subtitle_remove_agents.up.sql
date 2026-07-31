WITH redraw_model AS (
  SELECT code
  FROM models
  WHERE category = 'video' AND is_enabled = true
    AND (
      runtime_rule @> '{"capabilities":{"video_redraw":true}}'::jsonb
      OR lower(runtime_rule::text) ~ 'video_redraw|video-to-video|style_transfer|stylize'
      OR lower(code || ' ' || display_name) ~ 'redraw|video.?to.?video|转绘'
    )
  ORDER BY CASE WHEN runtime_rule @> '{"capabilities":{"video_redraw":true}}'::jsonb THEN 0 ELSE 1 END, sort_order, id
  LIMIT 1
), selected AS (
  SELECT COALESCE((SELECT code FROM redraw_model), '') AS model_code
)
INSERT INTO workflow_definitions (
  code, name, description, icon, category, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
)
SELECT
  'video_redraw', '一键转绘',
  '上传视频并选择目标画风，通过视频转视频模型保持动作与人物一致性完成风格转绘。',
  '🪄', 'video',
  jsonb_build_array(jsonb_build_object('id','redraw','type','video','name','AI 视频转绘','model_code',selected.model_code,'prompt_template','','cost',0)),
  jsonb_build_object(
    'type','object','required',jsonb_build_array('video_url'),
    'properties',jsonb_build_object(
      'video_url',jsonb_build_object('type','string','title','源视频'),
      'prompt',jsonb_build_object('type','string','title','转绘要求'),
      'style_strength',jsonb_build_object('type','number','title','风格强度','minimum',0,'maximum',1,'default',0.65),
      'preserve_motion',jsonb_build_object('type','boolean','title','保留动作','default',true),
      'preserve_identity',jsonb_build_object('type','boolean','title','保留人物身份','default',true),
      'preserve_audio',jsonb_build_object('type','boolean','title','保留原音','default',true)
    )
  ),
  jsonb_build_object('billing_type','model_actual','unit_price',0),
  jsonb_build_object(
    'theme','violet','hero_tags',jsonb_build_array('视频转绘','风格迁移','动作保持'),
    'feature_tags',jsonb_build_array('人物一致','运动一致','风格参考','保留原音'),
    'input',jsonb_build_object('image_label','源视频','placeholder','描述目标画风，例如：日系动漫、厚涂插画、赛博朋克电影感','modes',jsonb_build_array('智能托管')),
    'help','上传源视频，可补充风格参考图和画风描述。系统调用后台指定的视频转视频模型，并按模型实际费用结算。'
  ),
  jsonb_build_object(
    'agent_mode','video_redraw','generation_type','video','preset_code','video_redraw',
    'generation_model_code',selected.model_code,'default_style_strength',0.65,
    'preserve_motion',true,'preserve_identity',true,'preserve_audio',true,
    'max_input_duration_sec',300,'max_input_size_mb',500,'redraw_operation','video_redraw',
    'redraw_prompt','Redraw the source video in the requested visual style while preserving timing, motion, composition, subject identity and scene continuity. Avoid flicker and temporal inconsistency.',
    'default_count',1,
    'input_capabilities',jsonb_build_object('allow_text_only',false,'support_reference_image',true,'support_multiple_references',false,'support_first_last_frame',false),
    'flow_options',jsonb_build_object('enable_step_confirm',false,'enable_autopilot',true,'allow_prompt_edit',true)
  ),
  selected.model_code <> '', 77, now(), now()
FROM selected
ON CONFLICT (code) DO NOTHING;

WITH repair_model AS (
  SELECT code
  FROM models
  WHERE category = 'video' AND is_enabled = true
    AND (
      runtime_rule @> '{"capabilities":{"subtitle_remove":true}}'::jsonb
      OR lower(runtime_rule::text) ~ 'subtitle_remove|remove_subtitle|inpaint'
      OR lower(code || ' ' || display_name) ~ 'subtitle|inpaint|去字幕'
    )
  ORDER BY CASE WHEN runtime_rule @> '{"capabilities":{"subtitle_remove":true}}'::jsonb THEN 0 ELSE 1 END, sort_order, id
  LIMIT 1
), selected AS (
  SELECT COALESCE((SELECT code FROM repair_model), '') AS model_code
)
INSERT INTO workflow_definitions (
  code, name, description, icon, category, nodes, input_schema, price_rule,
  display_config, runtime_config, is_enabled, sort_order, created_at, updated_at
)
SELECT
  'subtitle_remove', '一键去字幕',
  '自动识别独立字幕轨或烧录硬字幕，并输出清理后的完整视频。',
  '🧹', 'video',
  jsonb_build_array(jsonb_build_object('id','subtitle_remove','type','video','name','AI 视频去字幕','model_code',selected.model_code,'prompt_template','','cost',0)),
  jsonb_build_object(
    'type','object','required',jsonb_build_array('video_url'),
    'properties',jsonb_build_object(
      'video_url',jsonb_build_object('type','string','title','源视频'),
      'subtitle_mode',jsonb_build_object('type','string','title','字幕类型','enum',jsonb_build_array('auto','soft_track','hardcoded_ai'),'default','auto'),
      'subtitle_region',jsonb_build_object('type','string','title','字幕区域','enum',jsonb_build_array('bottom_15','bottom_25','bottom_35','full'),'default','bottom_25'),
      'protect_watermark',jsonb_build_object('type','boolean','title','保护水印','default',true)
    )
  ),
  jsonb_build_object('billing_type','model_actual','unit_price',0),
  jsonb_build_object(
    'theme','emerald','hero_tags',jsonb_build_array('自动识别','软字幕无损','硬字幕修复'),
    'feature_tags',jsonb_build_array('字幕区域','保护水印','保留原音','结果预览'),
    'input',jsonb_build_object('image_label','源视频','placeholder','可选：说明字幕位置、需要保护的水印或画面区域','modes',jsonb_build_array('智能托管')),
    'help','自动模式优先无损移除独立字幕轨；没有字幕轨时调用后台配置的 AI 修复模型清除画面硬字幕。'
  ),
  jsonb_build_object(
    'agent_mode','subtitle_remove','generation_type','video','preset_code','subtitle_remove',
    'generation_model_code',selected.model_code,'default_subtitle_mode','auto',
    'default_subtitle_region','bottom_25','protect_watermark',true,'preserve_audio',true,
    'max_input_duration_sec',300,'max_input_size_mb',500,'subtitle_remove_operation','subtitle_remove',
    'subtitle_remove_prompt','Remove burned-in subtitles from the selected region and naturally reconstruct the background. Preserve people, products, logos, watermarks outside the subtitle region, motion, timing and audio.',
    'default_count',1,
    'input_capabilities',jsonb_build_object('allow_text_only',false,'support_reference_image',false,'support_multiple_references',false,'support_first_last_frame',false),
    'flow_options',jsonb_build_object('enable_step_confirm',false,'enable_autopilot',true,'allow_prompt_edit',true)
  ),
  true, 78, now(), now()
FROM selected
ON CONFLICT (code) DO NOTHING;
