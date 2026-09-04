-- Rename only the untouched built-in template. Administrator-customized entries
-- keep their names and descriptions while retaining the compatible template id.
UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE((
    SELECT jsonb_agg(
      CASE
        WHEN item->>'id' = 'story-short-video'
          AND item->>'name' = '故事短视频'
          AND item->>'description' IN (
            '故事脚本先生成关键帧，再生成短视频',
            '文本需求生成脚本、关键帧、配音、视频并自动合成',
            '故事拆分为多关键帧、多视频片段并合成为完整成片'
          )
          THEN item || '{"name":"视频创作","description":"创作需求生成视频脚本、分镜、关键帧、视频片段与完整成片"}'::jsonb
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
