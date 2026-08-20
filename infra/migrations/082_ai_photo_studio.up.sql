-- AI写真馆工作流定义

INSERT INTO workflow_definitions (
  code,
  name,
  description,
  category,
  icon,
  nodes,
  input_schema,
  runtime_config,
  display_config,
  price_rule,
  is_enabled,
  sort_order,
  created_at,
  updated_at
) VALUES (
  'ai_photo_studio',
  'AI写真馆',
  '上传一张照片，选择写真类型与风格倾向，AI 摄影团队即刻开拍。造型总监量身设计拍摄方案，摄影棚级光影、38种主流风格一键切换，人像特征全程保留，几分钟产出一整套杂志级写真。',
  'workflow',
  '📸',
  '[
    {
      "id": "styling",
      "type": "llm",
      "name": "写真造型设计",
      "model_code": "chat_demo_v1",
      "prompt_template": "",
      "cost": 0.02
    },
    {
      "id": "generate",
      "type": "image",
      "name": "写真拍摄生成",
      "model_code": "image_fast_v1",
      "prompt_template": "",
      "cost": 0
    }
  ]'::jsonb,
  '{
    "type": "object",
    "required": ["image_url", "photo_type", "style"],
    "properties": {
      "image_url": {
        "type": "string",
        "title": "本人照片"
      },
      "photo_type": {
        "type": "string",
        "title": "写真类型",
        "enum": ["写真", "职业照", "证件照"],
        "default": "写真",
        "x-widget": "option_menu"
      },
      "style": {
        "type": "string",
        "title": "风格倾向",
        "enum": ["影棚质感", "杂志大片", "黑白艺术", "韩系简约", "日系清新", "港风胶片", "法式复古", "美式复古", "国风古装", "旗袍风情", "新中式", "森系文艺", "咖啡馆日常", "都市夜景", "海边度假", "校园青春", "轻奢名媛", "甜美少女", "酷飒街头", "运动活力", "赛博霓虹", "暗调情绪", "户外自然", "婚纱浪漫", "雪景冬日", "商务精英", "纯白极简", "毕业季", "古典油画", "二次元动漫", "敦煌飞天", "民族风", "金秋落叶", "樱花春景", "Y2K千禧", "多巴胺糖果", "欧式宫廷", "沙漠戈壁"],
        "default": "影棚质感",
        "x-widget": "option_menu"
      },
      "id_background": {
        "type": "string",
        "title": "证件照底色",
        "enum": ["白色", "蓝色", "红色"],
        "default": "白色",
        "x-widget": "option_menu"
      },
      "count": {
        "type": "integer",
        "title": "生成张数",
        "enum": [1, 2, 4, 6, 8],
        "default": 4,
        "x-widget": "option_menu"
      },
      "prompt": {
        "type": "string",
        "title": "额外要求",
        "placeholder": "可选：补充服装、场景、动作或氛围要求，例如穿白色连衣裙、回眸微笑",
        "x-widget": "textarea"
      },
      "aspect_ratio": {
        "type": "string",
        "title": "画面比例",
        "default": "3:4"
      }
    }
  }'::jsonb,
  '{
    "agent_mode": "photo_studio",
    "generation_type": "image",
    "preset_code": "photo_studio",
    "analysis_model_code": "chat_demo_v1",
    "generation_model_code": "image_fast_v1",
    "require_image": true,
    "default_count": 4,
    "candidate_count": 1,
    "creative_scenes": ["main_image"],
    "roles": [
      {
        "id": "photo_director",
        "name": "摄影总监",
        "avatar": "📸",
        "description": "统筹整场拍摄，把控写真类型、风格与出片质量",
        "node": "styling"
      },
      {
        "id": "stylist",
        "name": "造型师",
        "avatar": "💄",
        "description": "根据照片与风格倾向设计妆造、服装与拍摄方案",
        "node": "styling"
      },
      {
        "id": "photographer",
        "name": "摄影师",
        "avatar": "🎞️",
        "description": "按拍摄方案出片，影棚级布光与构图",
        "node": "generate"
      },
      {
        "id": "retoucher",
        "name": "修图师",
        "avatar": "✨",
        "description": "保留人像特征的精修质感，皮肤与光影自然通透",
        "node": "generate"
      }
    ],
    "input_capabilities": {
      "allow_text_only": false,
      "support_reference_image": true,
      "support_multiple_references": false,
      "support_first_last_frame": false
    },
    "flow_options": {
      "enable_step_confirm": true,
      "enable_autopilot": true,
      "allow_prompt_edit": true
    }
  }'::jsonb,
  '{
    "theme": "fuchsia",
    "hero_tags": ["AI写真", "风格百变", "人像保真"],
    "feature_tags": ["38种主流风格", "写真/职业照/证件照", "影棚级光影", "几分钟出片"],
    "steps": [
      {
        "icon": "🪞",
        "title": "上传照片",
        "subtitle": "上传一张清晰的本人正面照片",
        "tags": ["人像识别", "特征提取"]
      },
      {
        "icon": "💄",
        "title": "造型设计",
        "subtitle": "造型师按写真类型与风格倾向定制拍摄方案",
        "tags": ["妆造方案", "场景布光"]
      },
      {
        "icon": "📸",
        "title": "写真拍摄",
        "subtitle": "摄影师按方案批量出片，人像特征全程保留",
        "tags": ["多张连拍", "风格一致"]
      },
      {
        "icon": "✨",
        "title": "精修交付",
        "subtitle": "修图师打磨质感，整套写真一键下载",
        "tags": ["自然精修", "打包下载"]
      }
    ],
    "input": {
      "image_label": "本人照片",
      "placeholder": "上传照片，选择写真类型与风格，即刻开拍",
      "modes": ["逐步确认", "智能托管"]
    },
    "help": "上传一张清晰的本人正面照片，选择写真类型（写真/职业照/证件照）和风格倾向，挑选出图模型与张数。造型师会先给出拍摄方案，逐步确认模式下可修改方案后再开拍，智能托管则自动完成拍摄与精修。"
  }'::jsonb,
  '{"billing_type": "model_actual", "unit_price": 0.1}'::jsonb,
  true,
  90,
  now(),
  now()
)
ON CONFLICT (code) DO NOTHING;
