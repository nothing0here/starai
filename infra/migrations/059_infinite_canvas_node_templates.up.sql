-- Refresh only untouched built-in infinite-canvas template metadata.
-- Custom administrator names/descriptions are preserved.
UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE((
    SELECT jsonb_agg(
      CASE
        WHEN item->>'id' = 'image-image'
          AND item->>'name' = '图片生图片'
          AND item->>'description' = '参考图片连接图片生成节点'
          THEN item || '{"description":"文本需求连接带参考图入口的图片生成节点"}'::jsonb
        WHEN item->>'id' = 'text-image-mix'
          AND item->>'name' = '文案与配图'
          AND item->>'description' = '文字与参考图片共同生成新图片'
          THEN item || '{"description":"文本需求先生成文案，再生成配图"}'::jsonb
        WHEN item->>'id' = 'multi-image'
          AND item->>'name' = '多图对比'
          AND item->>'description' = '多个参考素材连接双图片生成节点'
          THEN item || '{"description":"同一文本需求并行生成两套图片方案"}'::jsonb
        WHEN item->>'id' = 'image-video'
          AND item->>'name' = '图片生视频'
          AND item->>'description' = '首帧或参考图片连接视频生成节点'
          THEN item || '{"name":"首帧生视频","description":"文本需求连接支持人像形象和首帧素材的视频生成节点"}'::jsonb
        WHEN item->>'id' = 'story-short-video'
          AND item->>'name' = '故事短视频'
          AND item->>'description' = '故事脚本先生成关键帧，再生成短视频'
          THEN item || '{"description":"文本需求生成脚本、关键帧、配音、视频并自动合成"}'::jsonb
        WHEN item->>'id' = 'viral-remake'
          AND item->>'name' = '爆款复刻'
          AND item->>'description' = '爆款参考与品牌素材生成复刻主视觉并延展为短视频'
          THEN item || '{"description":"分析复刻需求，生成主视觉并延展为短视频"}'::jsonb
        ELSE item
      END
      ORDER BY ordinal
    )
    FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb))
      WITH ORDINALITY AS template(item, ordinal)
  ), '[]'::jsonb),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas'
  AND jsonb_typeof(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) = 'array';
