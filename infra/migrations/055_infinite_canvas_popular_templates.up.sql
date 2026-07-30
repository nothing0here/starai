-- Add popular, production-ready canvas workflows without replacing templates
-- that administrators have already customized.

UPDATE workflow_definitions AS workflow
SET display_config = jsonb_set(
  COALESCE(workflow.display_config, '{}'::jsonb),
  '{canvas_templates}',
  COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb) ||
  (
    SELECT COALESCE(jsonb_agg(candidate.item), '[]'::jsonb)
    FROM jsonb_array_elements(
      '[
        {"id":"ecommerce-visual-pack","name":"电商视觉套图","description":"商品信息与参考图同时生成主图和详情海报","template_id":"ecommerce-visual-pack"},
        {"id":"social-campaign","name":"社媒图文视频","description":"一份营销文案同时生成社媒配图和短视频","template_id":"social-campaign"},
        {"id":"product-showcase-video","name":"商品展示视频","description":"商品图先生成关键视觉，再延展为展示视频","template_id":"product-showcase-video"},
        {"id":"brand-visual-kit","name":"品牌视觉套件","description":"品牌需求并行生成标志创意和视觉海报","template_id":"brand-visual-kit"},
        {"id":"photo-restoration","name":"老照片修复","description":"参考照片经过修复、上色与高清增强生成新图","template_id":"photo-restoration"},
        {"id":"story-short-video","name":"故事短视频","description":"故事脚本先生成关键帧，再生成短视频","template_id":"story-short-video"}
      ]'::jsonb
    ) AS candidate(item)
    WHERE NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(COALESCE(workflow.display_config->'canvas_templates', '[]'::jsonb)) AS existing(item)
      WHERE existing.item->>'id' = candidate.item->>'id'
    )
  ),
  true
),
updated_at = now()
WHERE workflow.code = 'infinite_canvas';
