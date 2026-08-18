-- AI小说工坊工作流定义

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
  'ai_novel_workshop',
  'AI小说工坊',
  '一句话创意，让AI帮你写完一整本书。总编领队，故事策划、节奏编排师、章节写手、文学润色师、审校员、档案员多位AI专家协同作战——大纲逐章确认、设定全程追踪、写完自动润色审校，几十万字也不崩设定、不漂文风。',
  'workflow',
  '📖',
  '[
    {
      "id": "planning",
      "type": "llm",
      "name": "故事策划",
      "model_code": "chat_demo_v1",
      "prompt_template": "",
      "cost": 0.1
    },
    {
      "id": "writing",
      "type": "llm",
      "name": "章节创作",
      "model_code": "chat_demo_v1",
      "prompt_template": "",
      "cost": 0.05
    },
    {
      "id": "polishing",
      "type": "llm",
      "name": "润色审校",
      "model_code": "chat_demo_v1",
      "prompt_template": "",
      "cost": 0.03
    },
    {
      "id": "archiving",
      "type": "llm",
      "name": "档案更新",
      "model_code": "chat_demo_v1",
      "prompt_template": "",
      "cost": 0.02
    }
  ]'::jsonb,
  '{
    "type": "object",
    "properties": {
      "prompt": {
        "type": "string",
        "title": "故事创意",
        "placeholder": "例如：写一本关于未来世界的科幻小说，主角是AI研究员...",
        "x-widget": "textarea"
      },
      "genre": {
        "type": "string",
        "title": "题材类型",
        "enum": ["玄幻", "都市", "言情", "悬疑", "科幻", "历史", "游戏"],
        "default": "玄幻",
        "x-widget": "option_menu"
      },
      "word_count_target": {
        "type": "string",
        "title": "目标篇幅",
        "enum": ["短篇·3万字左右", "中篇·5万字左右", "长篇·10万字以上", "超长篇·50万字以上"],
        "default": "中篇·5万字左右",
        "x-widget": "option_menu"
      },
      "style": {
        "type": "string",
        "title": "文风",
        "enum": ["轻松幽默", "严肃正经", "诗意唯美", "节奏紧凑"],
        "default": "轻松幽默",
        "x-widget": "option_menu"
      },
      "language": {
        "type": "string",
        "title": "生成语言",
        "default": "zh-CN"
      }
    }
  }'::jsonb,
  '{
    "agent_mode": "novel_workshop",
    "generation_type": "chat",
    "analysis_model_code": "chat_demo_v1",
    "generation_model_code": "chat_demo_v1",
    "default_count": 1,
    "candidate_count": 1,
    "creative_scenes": ["novel_creation"],
    "batch_size": 10,
    "roles": [
      {
        "id": "chief_editor",
        "name": "总编主管",
        "avatar": "👔",
        "description": "统筹整本书的创作，调度团队、把控节奏与整体质量",
        "node": "planning"
      },
      {
        "id": "story_planner",
        "name": "故事策划",
        "avatar": "📋",
        "description": "出故事方向、搭故事圣经、排卷章大纲，一章章写什么安排得明明白白",
        "node": "planning"
      },
      {
        "id": "rhythm_editor",
        "name": "节奏编排师",
        "avatar": "📊",
        "description": "排一卷的张力曲线：钩子、爆点、留悬念怎么落位，让读者一章接一章停不下来",
        "node": "planning"
      },
      {
        "id": "chapter_writer",
        "name": "章节写手",
        "avatar": "✍️",
        "description": "按大纲和设定档案逐章写正文，上一章结尾接得天衣无缝",
        "node": "writing"
      },
      {
        "id": "polish_writer",
        "name": "文学润色师",
        "avatar": "✨",
        "description": "只改语言不改情节，把套话换成画面，让文字更有质感",
        "node": "polishing"
      },
      {
        "id": "proofreader",
        "name": "审校员",
        "avatar": "✅",
        "description": "每章写完对照设定台账查矛盾：时间线、人物状态、提前泄密一条条把关",
        "node": "polishing"
      },
      {
        "id": "archivist",
        "name": "档案员",
        "avatar": "📁",
        "description": "每章定稿后更新人物状态与摘要，几十万字后设定照样不崩",
        "node": "archiving"
      }
    ],
    "input_capabilities": {
      "allow_text_only": true,
      "support_reference_image": false,
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
    "theme": "indigo",
    "hero_tags": ["文学创作", "AI编辑部", "多角色协同"],
    "feature_tags": ["设定永不崩", "文风指纹统一", "全程对话可控", "整本打包下载"],
    "steps": [
      {
        "icon": "📝",
        "title": "故事策划",
        "subtitle": "总编确定选题、人物设定、卷章大纲",
        "tags": ["创意分析", "角色设定", "大纲规划"]
      },
      {
        "icon": "📖",
        "title": "逐章开写",
        "subtitle": "章节写手按大纲和设定档案逐章写作",
        "tags": ["分章节", "情节展开", "边写边审"]
      },
      {
        "icon": "✍️",
        "title": "润色审校",
        "subtitle": "文学润色师优化文笔，审校员把关设定一致性",
        "tags": ["风格统一", "语言打磨", "台账校验"]
      },
      {
        "icon": "✅",
        "title": "成书交付",
        "subtitle": "档案员更新台账，整本导出Word/TXT文档",
        "tags": ["质量把关", "格式整理", "打包下载"]
      }
    ],
    "input": {
      "image_label": "资产",
      "placeholder": "一句话创意，让AI帮你写完一整本书",
      "modes": ["逐步确认", "智能托管"]
    },
    "help": "说出创意，或贴上旧稿。从头创作就说一句话创意；续写就贴上已有书稿——选好题材和篇幅，编辑部立即开工。策划出方向你来挑，确认规模与大纲后，团队逐章写作、边写边审，右侧书画布实时看到每一章长出来。不满意？动嘴就行——随时在对话里提要求，团队立刻响应。"
  }'::jsonb,
  '{
    "billing_type": "per_chapter",
    "unit_price": 0.2,
    "planning_price": 0.5,
    "free_trial_chapters": 3
  }'::jsonb,
  false,
  100,
  now(),
  now()
);

-- 创建小说工坊专用的扩展字段索引（如果需要查询优化）
-- 注意：不使用子查询，而是在应用层过滤
CREATE INDEX IF NOT EXISTS idx_workflow_projects_novel_progress
  ON workflow_projects((outputs->>'current_chapter'))
  WHERE (outputs->>'current_chapter') IS NOT NULL;
