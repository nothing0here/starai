UPDATE workflow_definitions
SET is_enabled = true,
    input_schema = jsonb_set(
      jsonb_set(
        input_schema,
        '{properties,genre,enum}',
        '["玄幻", "都市", "言情", "悬疑", "科幻", "历史", "武侠", "游戏", "现实"]'::jsonb,
        true
      ),
      '{properties,word_count_target}',
      '{"type":"string","title":"目标篇幅","enum":["短篇·3万字内","中篇·约15万字","长篇·50万字以上"],"default":"中篇·约15万字","x-widget":"option_menu"}'::jsonb,
      true
    ),
    updated_at = now()
WHERE code = 'ai_novel_workshop';
