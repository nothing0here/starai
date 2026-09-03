UPDATE models
SET input_schema = jsonb_set(
      (input_schema #- '{properties,voice,x-option-genders}'),
      '{properties,voice,enum}', '["longanhuan_v3.6"]'::jsonb, true
    ),
    updated_at = now()
WHERE code = 'qwen-audio-3-0-tts-flash';
