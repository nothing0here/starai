UPDATE workflow_definitions
SET name = 'Ageng 通用智能体',
    display_config = display_config || '{"help":"描述目标并添加可选参考素材，Ageng 通用智能体会理解意图、选择后台默认模型并连续完成图片、视频、语音或音乐创作。"}'::jsonb,
    updated_at = now()
WHERE code = 'general_creative_agent';
