UPDATE workflow_definitions
SET runtime_config = jsonb_set(
  COALESCE(runtime_config, '{}'::jsonb),
  '{roles}',
  '[
    {"id":"stylist","name":"穿搭顾问","avatar":"/assets/photo-studio/stylist.png","description":"理解服装类型和穿着要求","node":"try_on"},
    {"id":"garment","name":"服装分析师","avatar":"/assets/photo-studio/photo-director.png","description":"识别版型、颜色、纹理与细节","node":"try_on"},
    {"id":"tryon","name":"试衣摄影师","avatar":"/assets/photo-studio/photographer.png","description":"调用多参考图模型完成试穿","node":"try_on"},
    {"id":"quality","name":"质检师","avatar":"/assets/photo-studio/retoucher.png","description":"检查人物和服装一致性","node":"try_on"}
  ]'::jsonb,
  true
), updated_at = now()
WHERE code = 'ai_virtual_tryon';
