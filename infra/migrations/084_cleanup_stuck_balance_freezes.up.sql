-- 清理存量卡死的余额冻结，规则与运行时对账守卫（operational guard）保持一致：
-- 1) 引用任务/工作流已终态（succeeded/failed/canceled）但冻结仍滞留：
--    succeeded 且有实际成本 -> 补扣费；其余 -> 全额解冻；
-- 2) 工作流项目卡在 waiting_confirm 超 24 小时（用户停在确认页离开，历史案例：freeze #54）
--    或 pending/running 超 6 小时无推进 -> 全额解冻并终结项目（卡死项目无权威结算结果，不向用户收费）。

BEGIN;

-- 1a) 补扣费：引用对象已成功且产生实际成本
WITH stuck AS (
    SELECT f.id, f.user_id, f.amount, f.ref_type, f.ref_id,
           CASE WHEN f.ref_type='task' THEN t.actual_cost ELSE p.actual_cost END AS actual_cost,
           CASE WHEN f.ref_type='task' THEN
                CASE COALESCE(t.type,'image')
                    WHEN 'video' THEN 'video_usage'
                    WHEN 'audio' THEN 'audio_usage'
                    ELSE 'image_usage'
                END
           ELSE 'workflow_usage' END AS tx_type
    FROM balance_freezes f
    LEFT JOIN tasks t ON f.ref_type='task' AND t.task_no=f.ref_id
    LEFT JOIN workflow_projects p ON f.ref_type='workflow' AND p.public_id=f.ref_id
    WHERE f.status='frozen' AND f.ref_type IN ('task','workflow')
      AND (
        (f.ref_type='task' AND t.status='succeeded' AND COALESCE(t.actual_cost,0) > 0)
        OR (f.ref_type='workflow' AND p.status='succeeded' AND COALESCE(p.actual_cost,0) > 0)
      )
),
settled AS (
    UPDATE wallets w
    SET compute_balance = w.compute_balance - agg.total_actual,
        frozen_compute  = GREATEST(w.frozen_compute - agg.total_amount, 0),
        updated_at      = now()
    FROM (
        SELECT user_id, SUM(actual_cost) AS total_actual, SUM(amount) AS total_amount
        FROM stuck GROUP BY user_id
    ) agg
    WHERE w.user_id = agg.user_id
    RETURNING w.user_id, w.compute_balance AS balance_after
),
marked AS (
    UPDATE balance_freezes f
    SET status='charged', released_at=now()
    FROM stuck s
    WHERE f.id=s.id AND f.status='frozen'
    RETURNING f.id
)
INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
SELECT s.user_id, s.tx_type, 'out', s.actual_cost, st.balance_after, s.ref_type, s.ref_id, '冻结滞留补结算(迁移清理)'
FROM stuck s
JOIN settled st ON st.user_id = s.user_id;

-- 1b) 全额解冻：其余引用对象已终态的滞留冻结（失败/取消，或成功但无成本）
WITH releasable AS (
    SELECT f.id, f.user_id, f.amount
    FROM balance_freezes f
    LEFT JOIN tasks t ON f.ref_type='task' AND t.task_no=f.ref_id
    LEFT JOIN workflow_projects p ON f.ref_type='workflow' AND p.public_id=f.ref_id
    WHERE f.status='frozen' AND f.ref_type IN ('task','workflow')
      AND (
        t.status IN ('succeeded','failed','canceled')
        OR p.status IN ('succeeded','failed','canceled')
      )
),
wallets_fixed AS (
    UPDATE wallets w
    SET frozen_compute = GREATEST(w.frozen_compute - agg.total_amount, 0), updated_at=now()
    FROM (
        SELECT user_id, SUM(amount) AS total_amount FROM releasable GROUP BY user_id
    ) agg
    WHERE w.user_id = agg.user_id
)
UPDATE balance_freezes f
SET status='released', released_at=now()
FROM releasable r
WHERE f.id=r.id AND f.status='frozen';

-- 2) 卡死工作流：waiting_confirm 超 24 小时 / pending、running 超 6 小时无推进
WITH stuck_projects AS (
    SELECT f.id AS freeze_id, f.user_id, f.amount, p.id AS project_id, p.status AS project_status
    FROM balance_freezes f
    JOIN workflow_projects p ON p.public_id = f.ref_id
    WHERE f.status='frozen' AND f.ref_type='workflow'
      AND (
        (p.status='waiting_confirm' AND p.updated_at < now() - interval '24 hours')
        OR (p.status IN ('pending','running') AND p.updated_at < now() - interval '6 hours')
      )
),
wallets_released AS (
    UPDATE wallets w
    SET frozen_compute = GREATEST(w.frozen_compute - agg.total_amount, 0), updated_at=now()
    FROM (
        SELECT user_id, SUM(amount) AS total_amount FROM stuck_projects GROUP BY user_id
    ) agg
    WHERE w.user_id = agg.user_id
),
freezes_released AS (
    UPDATE balance_freezes f
    SET status='released', released_at=now()
    FROM stuck_projects sp
    WHERE f.id=sp.freeze_id AND f.status='frozen'
)
UPDATE workflow_projects p
SET status='failed',
    error_message=CASE WHEN sp.project_status='waiting_confirm'
        THEN 'Workflow confirmation timed out (migration cleanup)'
        ELSE 'Workflow stalled and timed out (migration cleanup)' END,
    finished_at=now(), updated_at=now()
FROM stuck_projects sp
WHERE p.id=sp.project_id AND p.status IN ('pending','running','waiting_confirm');

COMMIT;
