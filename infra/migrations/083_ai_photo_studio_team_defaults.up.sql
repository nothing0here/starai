-- 083: AI写真馆默认值与 AI 编辑部头像优化
-- 1) 生成张数默认改为 1 张；2) 团队角色头像替换为 3D 卡通形象图片。

UPDATE workflow_definitions
SET runtime_config = jsonb_set(
        jsonb_set(runtime_config, '{default_count}', '1'),
        '{roles}',
        '[
          {"id": "photo_director", "name": "摄影总监", "avatar": "/assets/photo-studio/photo-director.png", "description": "统筹整场拍摄，把控写真类型、风格与出片质量", "node": "styling"},
          {"id": "stylist", "name": "造型师", "avatar": "/assets/photo-studio/stylist.png", "description": "根据照片与风格倾向设计妆造、服装与拍摄方案", "node": "styling"},
          {"id": "photographer", "name": "摄影师", "avatar": "/assets/photo-studio/photographer.png", "description": "按拍摄方案出片，影棚级布光与构图", "node": "generate"},
          {"id": "retoucher", "name": "修图师", "avatar": "/assets/photo-studio/retoucher.png", "description": "保留人像特征的精修质感，皮肤与光影自然通透", "node": "generate"}
        ]'::jsonb
    ),
    input_schema = jsonb_set(
        jsonb_set(input_schema, '{properties,count,default}', '1'),
        '{required}',
        '["image_url", "photo_type"]'::jsonb
    ),
    updated_at = now()
WHERE code = 'ai_photo_studio';
