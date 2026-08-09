DROP INDEX IF EXISTS uq_balance_freezes_active_reference;
ALTER TABLE balance_freezes
  DROP CONSTRAINT IF EXISTS chk_balance_freezes_amount_nonnegative;
