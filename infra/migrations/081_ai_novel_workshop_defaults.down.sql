UPDATE workflow_definitions
SET is_enabled = false,
    input_schema = jsonb_set(
      jsonb_set(
        input_schema,
        '{properties,genre,enum}',
        '["玄幻", "都市", "言情", "悬疑", "科幻", "历史", "游戏"]'::jsonb,
        true
      ),
      '{properties,word_count_target}',
      '{"type":"string","title":"目标篇幅","enum":["短篇·3万字左右","中篇·5万字左右","长篇·10万字以上","超长篇·50万字以上"],"default":"中篇·5万字左右","x-widget":"option_menu"}'::jsonb,
      true
    ),
    updated_at = now()
WHERE code = 'ai_novel_workshop';
