-- 回滚为按章计费配置
UPDATE workflow_definitions
SET price_rule = '{"billing_type": "per_chapter", "unit_price": 0.2, "planning_price": 0.5, "free_trial_chapters": 3}'::jsonb,
    updated_at = now()
WHERE code = 'ai_novel_workshop';
