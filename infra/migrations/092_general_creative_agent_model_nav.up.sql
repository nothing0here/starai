UPDATE workflow_definitions
SET name = 'Ageng 通用智能体',
    category = 'chat',
    sort_order = -200,
    description = '一个入口完成对话理解、图片、视频、语音和音乐创作。',
    display_config = display_config || '{"theme":"amber","hero_tags":["自然语言创作","连续引用","图片与视频","语音与音乐"],"feature_tags":["智能理解需求","自动衔接结果","后台默认模型","支持自定义模型"],"help":"描述目标并添加可选参考素材，Ageng 通用智能体会理解意图、选择后台默认模型并连续完成图片、视频、语音或音乐创作。"}'::jsonb,
    updated_at = now()
WHERE code = 'general_creative_agent';
