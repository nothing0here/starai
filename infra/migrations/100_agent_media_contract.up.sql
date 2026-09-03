-- Keep the current Qwen model but expose its supported male and female system
-- voices so the Agent can bind the confirmed character gender to TTS.
UPDATE models
SET input_schema = jsonb_set(
      jsonb_set(
        jsonb_set(
          input_schema,
          '{properties,voice,enum}',
          '["longanfengyue","longanyuanfei","longanlingxi","longanxiaoxin","longanhuan_v3.6","longjielidou_v3.6","longpaopao_v3.6","longhuohuo_v3.6","longchuanshu_v3.6","loongmary","loongeva_v3.6","loongjohn"]'::jsonb,
          true
        ),
        '{properties,voice,enumLabels}',
        '{"longanfengyue":"女声 · 龙安风悦","longanyuanfei":"女声 · 龙安元妃","longanlingxi":"女声 · 龙安灵希","longanxiaoxin":"女声 · 龙安小昕","longanhuan_v3.6":"女声 · 龙安欢","longjielidou_v3.6":"男童 · 龙杰力豆","longpaopao_v3.6":"女童 · 龙泡泡","longhuohuo_v3.6":"男童 · 龙火火","longchuanshu_v3.6":"男声 · 龙川叔","loongmary":"女声 · Mary","loongeva_v3.6":"女声 · Eva","loongjohn":"男声 · John"}'::jsonb,
        true
      ),
      '{properties,voice,x-option-genders}',
      '{"longanfengyue":"female","longanyuanfei":"female","longanlingxi":"female","longanxiaoxin":"female","longanhuan_v3.6":"female","longjielidou_v3.6":"male","longpaopao_v3.6":"female","longhuohuo_v3.6":"male","longchuanshu_v3.6":"male","loongmary":"female","loongeva_v3.6":"female","loongjohn":"male"}'::jsonb,
      true
    ),
    runtime_rule = jsonb_set(
      jsonb_set(runtime_rule, '{upstream,include}', '["voice","format","sample_rate","instruction"]'::jsonb, true),
      '{upstream,map,instruction}', '"input.instruction"'::jsonb, true
    ),
    updated_at = now()
WHERE code = 'qwen-audio-3-0-tts-flash';

-- Selecting a provider in the admin means the feature is intended to be live.
-- The switch remains independently available for administrators who want it off.
UPDATE system_configs
SET value = 'true'::jsonb, updated_at = now()
WHERE key = 'web_search_enabled'
  AND EXISTS (
    SELECT 1 FROM system_configs provider
    WHERE provider.key = 'web_search_provider'
      AND trim(both '"' from provider.value::text) IN ('tavily','brave','searxng','hybrid','redfox')
  );
