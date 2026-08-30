UPDATE workflow_definitions
SET name = '通用智能体',
    category = 'tool',
    sort_order = -100,
    description = '通过聊天理解创作意图，自动生成图片、视频、语音或音乐任务。',
    updated_at = now()
WHERE code = 'general_creative_agent';
