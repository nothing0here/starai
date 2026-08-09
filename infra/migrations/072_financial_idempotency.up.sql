CREATE TABLE gallery_purchases (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  gallery_public_id VARCHAR(32) NOT NULL,
  amount NUMERIC(18,6) NOT NULL,
  wallet_transaction_id BIGINT REFERENCES wallet_transactions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT gallery_purchases_amount_positive CHECK (amount > 0),
  CONSTRAINT gallery_purchases_user_item_unique UNIQUE (user_id, gallery_public_id)
);
CREATE INDEX idx_gallery_purchases_user_time ON gallery_purchases(user_id, created_at DESC);

ALTER TABLE wallet_transactions
  ADD CONSTRAINT wallet_transactions_amount_positive CHECK (amount > 0) NOT VALID;
ALTER TABLE cash_transactions
  ADD CONSTRAINT cash_transactions_amount_positive CHECK (amount > 0) NOT VALID;
ALTER TABLE wallets
  ADD CONSTRAINT wallets_frozen_compute_nonnegative CHECK (frozen_compute >= 0) NOT VALID;
