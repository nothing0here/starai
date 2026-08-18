-- AI小说工坊计费调整为：工作流费 + 大模型实际用量费
-- billing_type=model_actual 时 unit_price 表示工作流基础费，
-- 总费用 = unit_price + 大模型实际用量（按模型设定的输入/输出单价 × 真实 token 用量）
UPDATE workflow_definitions
SET price_rule = '{"billing_type": "model_actual", "unit_price": 0.1}'::jsonb,
    updated_at = now()
WHERE code = 'ai_novel_workshop';
