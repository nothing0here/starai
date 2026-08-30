UPDATE workflow_definitions
SET runtime_config = runtime_config - 'speech_model_code' - 'music_model_code',
    description = '通过聊天理解创作意图，自动生成图片或视频任务。',
    display_config = display_config
      || '{"hero_tags":["聊天创作","图片生成","视频生成"],"help":"直接描述你的创作需求，通用智能体会理解意图并生成图片或视频任务。"}'::jsonb,
    updated_at = now()
WHERE code = 'general_creative_agent';
