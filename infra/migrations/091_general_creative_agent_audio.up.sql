UPDATE workflow_definitions
SET runtime_config = runtime_config
    || jsonb_build_object(
      'speech_model_code', COALESCE(
        (SELECT code FROM models WHERE is_enabled AND category = 'audio' AND code = 'audio_minimax_speech_28_hd' LIMIT 1),
        (SELECT code FROM models WHERE is_enabled AND category = 'audio' AND lower(code || ' ' || display_name) ~ '(speech|tts|voice|语音|配音)' ORDER BY sort_order, id LIMIT 1),
        ''
      ),
      'music_model_code', COALESCE(
        (SELECT code FROM models WHERE is_enabled AND category = 'audio' AND code = 'audio_minimax_music_26' LIMIT 1),
        (SELECT code FROM models WHERE is_enabled AND category = 'audio' AND lower(code || ' ' || display_name) ~ '(music|suno|音乐|歌曲)' ORDER BY sort_order, id LIMIT 1),
        ''
      )
    ),
    description = '通过聊天理解创作意图，自动生成图片、视频、语音或音乐任务。',
    display_config = display_config
      || '{"hero_tags":["聊天创作","图片生成","视频生成","音频生成"],"help":"直接描述创作需求，通用智能体会理解意图并生成图片、视频、语音或音乐任务。"}'::jsonb,
    updated_at = now()
WHERE code = 'general_creative_agent';
