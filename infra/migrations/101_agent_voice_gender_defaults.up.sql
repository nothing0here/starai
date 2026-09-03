UPDATE models
SET input_schema = jsonb_set(
      input_schema,
      '{properties,voice,x-agent-default-by-gender}',
      '{"male":"longchuanshu_v3.6","female":"longanhuan_v3.6"}'::jsonb,
      true
    ),
    updated_at = now()
WHERE code = 'qwen-audio-3-0-tts-flash';
