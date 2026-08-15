#!/usr/bin/env bash
# 批量释放卡住的任务：将长时间停留在 pending/running 的任务与工作流标记失败，
# 并解冻对应的冻结算力余额（逻辑与运维守卫 ReconcileFrozenBalances 一致，
# 但不再要求 6 小时阈值，阈值由 STUCK_MINUTES 控制）。
#
# 用法（仓库根目录执行）:
#   bash scripts/release-stuck-tasks.sh            # 默认只看汇总，不执行（dry-run）
#   bash scripts/release-stuck-tasks.sh 30         # 预览：卡住超过 30 分钟的任务
#   RELEASE_CONFIRM=1 bash scripts/release-stuck-tasks.sh 30   # 实际执行
#
# 可选环境变量:
#   ENV_FILE      默认 .env.production
#   COMPOSE_FILE  默认 infra/docker/docker-compose.prod.yml
#   STUCK_MINUTES 优先级低于第一个位置参数，默认 10
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${ENV_FILE:-.env.production}"
COMPOSE_FILE="${COMPOSE_FILE:-infra/docker/docker-compose.prod.yml}"
STUCK_MINUTES="${1:-${STUCK_MINUTES:-10}}"
RELEASE_CONFIRM="${RELEASE_CONFIRM:-0}"

case "$STUCK_MINUTES" in
  ''|*[!0-9]*)
    echo "STUCK_MINUTES 必须是正整数（分钟），当前值: $STUCK_MINUTES" >&2
    exit 1
    ;;
esac

if [ ! -f "$ENV_FILE" ]; then
  echo "缺少 $ENV_FILE，请先准备环境文件。" >&2
  exit 1
fi

read_env() {
  local key="$1"
  local value
  value="$(awk -F= -v k="$key" '$1 == k { sub(/^[^=]*=/, ""); gsub(/\r$/, ""); print; exit }' "$ENV_FILE" || true)"
  value="${value%\"}"
  value="${value#\"}"
  value="${value%\'}"
  value="${value#\'}"
  printf '%s' "$value"
}

DB_USER="$(read_env POSTGRES_USER)"
DB_NAME="$(read_env POSTGRES_DB)"
DB_USER="${DB_USER:-starai}"
DB_NAME="${DB_NAME:-starai}"

run_psql() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
    psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 "$@"
}

echo "==> 检查 compose 配置与 PostgreSQL 状态"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config >/dev/null
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d postgres >/dev/null

echo "==> 卡住任务汇总（阈值: ${STUCK_MINUTES} 分钟）"
run_psql <<SQL
SELECT kind AS "类型", cnt AS "数量", amount AS "冻结算力"
FROM (
  SELECT '卡住任务(tasks)' AS kind, 1 AS sort,
         COUNT(DISTINCT t.id) AS cnt,
         ROUND(COALESCE(SUM(f.amount),0)::numeric, 4) AS amount
  FROM tasks t
  LEFT JOIN balance_freezes f
    ON f.ref_type='task' AND f.ref_id=t.task_no AND f.status='frozen'
  WHERE t.status IN ('pending','running')
    AND t.created_at < now() - interval '${STUCK_MINUTES} minutes'
  UNION ALL
  SELECT '卡住工作流(workflow_projects)', 2,
         COUNT(DISTINCT wp.id),
         ROUND(COALESCE(SUM(f.amount),0)::numeric, 4)
  FROM workflow_projects wp
  LEFT JOIN balance_freezes f
    ON f.ref_type='workflow' AND f.ref_id=wp.public_id AND f.status='frozen'
  WHERE wp.status IN ('pending','running')
    AND wp.created_at < now() - interval '${STUCK_MINUTES} minutes'
) summary
ORDER BY sort;
SQL

if [ "$RELEASE_CONFIRM" != "1" ] && [ "$RELEASE_CONFIRM" != "true" ]; then
  cat <<EOF

当前为预览模式（dry-run），未做任何修改。
确认无误后执行:
  RELEASE_CONFIRM=1 bash scripts/release-stuck-tasks.sh ${STUCK_MINUTES}

注意: 正在被 worker 实际处理的任务会因行级咨询锁被自动跳过；
执行后建议重启 worker 以释放被僵尸轮询占用的并发槽位:
  docker compose --env-file ${ENV_FILE} -f ${COMPOSE_FILE} restart worker
EOF
  exit 0
fi

echo "==> 开始释放（单事务，逐行加咨询锁，跳过 worker 正在处理的任务）"
run_psql <<SQL
DO \$\$
DECLARE
  rec RECORD;
  freeze_status TEXT;
  released_tasks INT := 0;
  released_workflows INT := 0;
  failed_tasks_no_freeze INT := 0;
BEGIN
  -- 1) 卡住的生成任务：置失败 + 解冻余额（与 failStaleTasks 相同流程）
  FOR rec IN
    SELECT f.id AS freeze_id, f.user_id, f.amount, f.ref_id AS task_no, t.id AS task_id
    FROM balance_freezes f
    JOIN tasks t ON t.task_no = f.ref_id
    WHERE f.status='frozen' AND f.ref_type='task'
      AND t.status IN ('pending','running')
      AND t.created_at < now() - interval '${STUCK_MINUTES} minutes'
    ORDER BY t.created_at ASC
  LOOP
    IF NOT pg_try_advisory_xact_lock(-rec.task_id) THEN
      CONTINUE; -- worker 正在处理，跳过
    END IF;
    PERFORM 1 FROM wallets WHERE user_id=rec.user_id FOR UPDATE;
    SELECT status INTO freeze_status FROM balance_freezes WHERE id=rec.freeze_id FOR UPDATE;
    IF freeze_status IS DISTINCT FROM 'frozen' THEN
      CONTINUE;
    END IF;
    UPDATE tasks SET status='failed', error_code='STALE_TIMEOUT',
      error_message='Task timed out by operational guard',
      finished_at=now(), updated_at=now()
      WHERE task_no=rec.task_no AND status IN ('pending','running');
    IF FOUND THEN
      UPDATE wallets SET frozen_compute=GREATEST(frozen_compute-rec.amount,0), updated_at=now()
        WHERE user_id=rec.user_id;
      UPDATE balance_freezes SET status='released', released_at=now()
        WHERE id=rec.freeze_id AND status='frozen';
      released_tasks := released_tasks + 1;
    END IF;
  END LOOP;

  -- 2) 卡住的工作流项目：同样流程
  FOR rec IN
    SELECT f.id AS freeze_id, f.user_id, f.amount, f.ref_id AS public_id, p.id AS project_id
    FROM balance_freezes f
    JOIN workflow_projects p ON p.public_id = f.ref_id
    WHERE f.status='frozen' AND f.ref_type='workflow'
      AND p.status IN ('pending','running')
      AND p.created_at < now() - interval '${STUCK_MINUTES} minutes'
    ORDER BY p.created_at ASC
  LOOP
    IF NOT pg_try_advisory_xact_lock(rec.project_id) THEN
      CONTINUE;
    END IF;
    PERFORM 1 FROM wallets WHERE user_id=rec.user_id FOR UPDATE;
    SELECT status INTO freeze_status FROM balance_freezes WHERE id=rec.freeze_id FOR UPDATE;
    IF freeze_status IS DISTINCT FROM 'frozen' THEN
      CONTINUE;
    END IF;
    UPDATE workflow_projects SET status='failed',
      error_message='Workflow timed out by operational guard',
      finished_at=now(), updated_at=now()
      WHERE public_id=rec.public_id AND status IN ('pending','running');
    IF FOUND THEN
      UPDATE wallets SET frozen_compute=GREATEST(frozen_compute-rec.amount,0), updated_at=now()
        WHERE user_id=rec.user_id;
      UPDATE balance_freezes SET status='released', released_at=now()
        WHERE id=rec.freeze_id AND status='frozen';
      released_workflows := released_workflows + 1;
    END IF;
  END LOOP;

  -- 3) 兜底：超过阈值仍卡住但没有冻结记录的任务/工作流（异常情况）
  WITH stuck AS (
    UPDATE tasks t SET status='failed', error_code='STALE_TIMEOUT',
      error_message='Task timed out by operational guard',
      finished_at=now(), updated_at=now()
    WHERE t.status IN ('pending','running')
      AND t.created_at < now() - interval '${STUCK_MINUTES} minutes'
      AND NOT EXISTS (
        SELECT 1 FROM balance_freezes f
        WHERE f.ref_type='task' AND f.ref_id=t.task_no AND f.status='frozen'
      )
    RETURNING 1
  )
  SELECT COUNT(*) INTO failed_tasks_no_freeze FROM stuck;

  RAISE NOTICE '释放完成: 任务=% 工作流=% 无冻结记录的卡住任务=%',
    released_tasks, released_workflows, failed_tasks_no_freeze;
END
\$\$;
SQL

echo ""
echo "==> 释放后剩余冻结与在途任务"
run_psql <<SQL
SELECT
  (SELECT COUNT(*) FROM balance_freezes WHERE status='frozen') AS "剩余冻结笔数",
  (SELECT COUNT(*) FROM tasks WHERE status='pending') AS "pending任务",
  (SELECT COUNT(*) FROM tasks WHERE status='running') AS "running任务";
SQL

cat <<EOF

完成。建议接着重启 worker，释放被僵尸轮询占用的并发槽位:
  docker compose --env-file ${ENV_FILE} -f ${COMPOSE_FILE} restart worker
EOF
