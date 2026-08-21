-- AI试衣间：复用现有多参考图图片模型链路。

WITH compatible_model AS (
  SELECT code
  FROM models
  WHERE category = 'image'
    AND is_enabled = true
    AND (
      lower(code || ' ' || new_api_model) LIKE '%nano_banana%'
      OR lower(code || ' ' || new_api_model) LIKE '%gpt-image-2%'
      OR lower(code || ' ' || new_api_model) LIKE '%gemini%'
      OR COALESCE(runtime_rule->'image'->>'max_reference_images', runtime_rule->>'max_reference_images', '0') IN ('2','3','4','5','6','7','8','9','10','14','20')
    )
  ORDER BY sort_order ASC, id ASC
  LIMIT 1
)
INSERT INTO workflow_definitions (
  code, name, description, category, icon, nodes, input_schema, runtime_config,
  display_config, price_rule, is_enabled, sort_order, created_at, updated_at
)
SELECT
  'ai_virtual_tryon',
  'AI试衣间',
  '上传人物照片和服装商品图，使用 Nano Banana、GPT Image 2 或 Gemini 多参考图模型，快速生成自然的视觉试穿效果。',
  'workflow',
  '👗',
  jsonb_build_array(jsonb_build_object(
    'id', 'try_on', 'type', 'image', 'name', 'AI试穿生成',
    'model_code', COALESCE((SELECT code FROM compatible_model), ''),
    'prompt_template', '', 'cost', 0
  )),
  '{
    "type":"object",
    "required":["person_image_url","person_asset_id","garment_image_url","garment_asset_id","consent_confirmed"],
    "properties":{
      "person_image_url":{"type":"string","title":"人物照片"},
      "person_asset_id":{"type":"string","title":"人物素材ID"},
      "garment_image_url":{"type":"string","title":"服装图片"},
      "garment_asset_id":{"type":"string","title":"服装素材ID"},
      "garment_category":{"type":"string","title":"服装类型","enum":["auto","tops","bottoms","one-pieces"],"default":"auto"},
      "garment_photo_type":{"type":"string","title":"商品图类型","enum":["auto","flat-lay","model"],"default":"auto"},
      "count":{"type":"integer","title":"生成张数","enum":[1,2,4],"default":1},
      "image_size":{"type":"string","title":"清晰度","enum":["1K","2K"],"default":"1K"},
      "aspect_ratio":{"type":"string","title":"画面比例","default":"3:4"},
      "prompt":{"type":"string","title":"穿着要求"},
      "consent_confirmed":{"type":"boolean","title":"人物照片授权确认"}
    }
  }'::jsonb,
  jsonb_build_object(
    'agent_mode', 'virtual_try_on',
    'tryon_engine', 'multi_reference',
    'generation_type', 'image',
    'preset_code', 'virtual_try_on',
    'generation_model_code', COALESCE((SELECT code FROM compatible_model), ''),
    'require_image', true,
    'default_count', 1,
    'candidate_count', 1,
    'creative_scenes', jsonb_build_array('main_image'),
    'roles', '[
      {"id":"stylist","name":"穿搭顾问","avatar":"👔","description":"理解服装类型和穿着要求","node":"try_on"},
      {"id":"garment","name":"服装分析师","avatar":"🧵","description":"识别版型、颜色、纹理与细节","node":"try_on"},
      {"id":"tryon","name":"试衣摄影师","avatar":"📷","description":"调用多参考图模型完成试穿","node":"try_on"},
      {"id":"quality","name":"质检师","avatar":"✅","description":"检查人物和服装一致性","node":"try_on"}
    ]'::jsonb,
    'input_capabilities', '{"allow_text_only":false,"support_reference_image":true,"support_multiple_references":true,"support_first_last_frame":false}'::jsonb,
    'flow_options', '{"enable_step_confirm":false,"enable_autopilot":true,"allow_prompt_edit":true}'::jsonb
  ),
  '{
    "theme":"rose",
    "hero_tags":["双图试穿","人物保真","服装还原"],
    "feature_tags":["Nano Banana","GPT Image 2","Gemini","结果可下载"],
    "steps":[
      {"icon":"🧍","title":"上传人物照","subtitle":"上传清晰的单人半身或全身照片","tags":["授权确认","人物保真"]},
      {"icon":"👗","title":"上传服装图","subtitle":"使用清晰的单件平铺图或商品图","tags":["服装识别","细节提取"]},
      {"icon":"✨","title":"AI智能试穿","subtitle":"多参考图模型替换目标服装区域","tags":["双图生成","区域控制"]},
      {"icon":"📥","title":"结果交付","subtitle":"在线查看并下载试穿结果","tags":["历史记录","结果下载"]}
    ],
    "input":{"image_label":"人物与服装","placeholder":"上传人物照和服装图，立即开始试穿","modes":["智能托管"]},
    "help":"分别上传人物照片和服装商品图，选择服装类型、模型、清晰度与张数。AI试穿仅供视觉参考，不代表真实尺码和版型。"
  }'::jsonb,
  '{"billing_type":"model_actual","unit_price":0.1}'::jsonb,
  EXISTS(SELECT 1 FROM compatible_model),
  91,
  now(),
  now()
ON CONFLICT (code) DO NOTHING;
