-- 085: 修复迁移 084 清理中被误判为 failed 的卡死工作流项目。
-- 这些项目实际已完成部分步骤（如小说工坊的故事策划），按新口径：
-- 1) 状态改为 canceled（不归为失败），已生成内容保留、可正常打开查看；
-- 2) 按已完成步骤的累计成本补扣费（model_actual：工作流费 + min(上游真实成本, 模型用量费)），
--    与 API accruedWorkflowCost / 用户主动取消的结算口径一致。
-- 迁移 084 已将对应冻结全额解冻退还，本迁移仅补扣已完成步骤的实际费用。
BEGIN;

WITH settlement AS (
    SELECT
        p.id AS project_id,
        p.user_id,
        p.public_id,
        CASE
            WHEN COALESCE(nc.node_cost, 0) <= 0 THEN 0
            ELSE COALESCE(NULLIF((w.price_rule->>'unit_price')::numeric, 0), 0)
                 + LEAST(
                     nc.node_cost,
                     COALESCE(pc.provider_cost, nc.node_cost)
                   )
        END AS actual_cost
    FROM workflow_projects p
    JOIN workflow_definitions w ON w.id = p.workflow_id
    LEFT JOIN LATERAL (
        SELECT COALESCE(SUM(n.cost), 0) AS node_cost
        FROM workflow_node_runs n
        WHERE n.project_id = p.id
    ) nc ON true
    LEFT JOIN LATERAL (
        SELECT COALESCE(SUM(latest_cost), 0) AS provider_cost
        FROM (
            SELECT DISTINCT ON (request_id) provider_cost AS latest_cost
            FROM model_route_attempts
            WHERE status = 'SUCCESS' AND provider_cost IS NOT NULL
              AND (request_id = 'novel_planning_' || p.id
                   OR request_id LIKE 'novel_write_' || p.id || '_ch%'
                   OR request_id LIKE 'novel_polish_' || p.id || '_ch%'
                   OR request_id LIKE 'novel_archive_' || p.id || '_ch%')
            ORDER BY request_id, id DESC
        ) t
    ) pc ON true
    WHERE p.status = 'failed'
      AND p.error_message LIKE '%(migration cleanup)'
),
wallet_fix AS (
    UPDATE wallets w
    SET compute_balance = compute_balance - s.total_cost,
        updated_at = now()
    FROM (
        SELECT user_id, SUM(actual_cost) AS total_cost
        FROM settlement
        WHERE actual_cost > 0
        GROUP BY user_id
    ) s
    WHERE w.user_id = s.user_id
),
charge_tx AS (
    INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
    SELECT s.user_id, 'workflow_usage', 'out', s.actual_cost,
           w.compute_balance - SUM(s.actual_cost) OVER (PARTITION BY s.user_id ORDER BY s.project_id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW),
           'workflow', s.public_id, '超时取消补结算已完成步骤(迁移修复)'
    FROM settlement s
    JOIN wallets w ON w.user_id = s.user_id
    WHERE s.actual_cost > 0
)
UPDATE workflow_projects p
SET status = 'canceled',
    actual_cost = s.actual_cost,
    error_message = '超时自动取消：长时间未确认，已生成内容已保留（迁移修复）',
    finished_at = COALESCE(p.finished_at, now()),
    updated_at = now()
FROM settlement s
WHERE p.id = s.project_id;

COMMIT;
