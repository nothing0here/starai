-- Collapse duplicate active reservations before enforcing one active freeze per business reference.
WITH grouped AS (
  SELECT user_id, ref_type, ref_id, MIN(id) AS keeper_id, SUM(amount) AS total_amount
  FROM balance_freezes
  WHERE status = 'frozen'
  GROUP BY user_id, ref_type, ref_id
  HAVING COUNT(*) > 1
), updated_keeper AS (
  UPDATE balance_freezes f
  SET amount = g.total_amount
  FROM grouped g
  WHERE f.id = g.keeper_id
  RETURNING f.id
)
UPDATE balance_freezes f
SET status = 'superseded', released_at = now()
FROM grouped g
WHERE f.user_id = g.user_id
  AND f.ref_type = g.ref_type
  AND f.ref_id = g.ref_id
  AND f.status = 'frozen'
  AND f.id <> g.keeper_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_balance_freezes_active_reference
  ON balance_freezes (user_id, ref_type, ref_id)
  WHERE status = 'frozen';

ALTER TABLE balance_freezes
  DROP CONSTRAINT IF EXISTS chk_balance_freezes_amount_nonnegative;
ALTER TABLE balance_freezes
  ADD CONSTRAINT chk_balance_freezes_amount_nonnegative CHECK (amount >= 0);

