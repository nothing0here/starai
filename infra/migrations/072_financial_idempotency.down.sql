ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_frozen_compute_nonnegative;
ALTER TABLE cash_transactions DROP CONSTRAINT IF EXISTS cash_transactions_amount_positive;
ALTER TABLE wallet_transactions DROP CONSTRAINT IF EXISTS wallet_transactions_amount_positive;
DROP TABLE IF EXISTS gallery_purchases;
