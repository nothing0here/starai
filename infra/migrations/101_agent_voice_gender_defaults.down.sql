UPDATE models
SET input_schema = input_schema #- '{properties,voice,x-agent-default-by-gender}',
    updated_at = now()
WHERE code = 'qwen-audio-3-0-tts-flash';
