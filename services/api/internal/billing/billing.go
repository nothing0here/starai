package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrFreezeNotFound      = errors.New("freeze not found")
)

const InsufficientBalanceMsg = "账户余额不足"

type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) GetWallet(ctx context.Context, userID int64) (compute, frozen float64, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT compute_balance, frozen_compute FROM wallets WHERE user_id=$1`, userID,
	).Scan(&compute, &frozen)
	return
}

func (s *Service) EnsureWallet(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO wallets (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
	return err
}

func (s *Service) Freeze(ctx context.Context, userID int64, amount float64, refType, refID string) error {
	return s.FreezeWithFinalize(ctx, userID, amount, refType, refID, nil)
}

func (s *Service) FreezeWithFinalize(ctx context.Context, userID int64, amount float64, refType, refID string, finalize func(pgx.Tx) error) error {
	if amount <= 0 {
		if finalize == nil {
			return nil
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err = finalize(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance, frozen float64
	err = tx.QueryRow(ctx,
		`SELECT compute_balance, frozen_compute FROM wallets WHERE user_id=$1 FOR UPDATE`, userID,
	).Scan(&balance, &frozen)
	if err != nil {
		return err
	}
	var existingID int64
	var existingAmount float64
	existingErr := tx.QueryRow(ctx,
		`SELECT id, amount FROM balance_freezes
		 WHERE user_id=$1 AND ref_type=$2 AND ref_id=$3 AND status='frozen'
		 FOR UPDATE`, userID, refType, refID,
	).Scan(&existingID, &existingAmount)
	if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return existingErr
	}
	if existingErr == nil {
		delta := amount - existingAmount
		if delta > 0 && balance-frozen < delta {
			return ErrInsufficientBalance
		}
		if delta != 0 {
			if _, err = tx.Exec(ctx,
				`UPDATE wallets SET frozen_compute=GREATEST(frozen_compute+$1,0), updated_at=now() WHERE user_id=$2`,
				delta, userID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE balance_freezes SET amount=$1 WHERE id=$2`, amount, existingID); err != nil {
				return err
			}
		}
		if finalize != nil {
			if err = finalize(tx); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	available := balance - frozen
	if available < amount {
		return ErrInsufficientBalance
	}
	_, err = tx.Exec(ctx,
		`UPDATE wallets SET frozen_compute = frozen_compute + $1, updated_at=now() WHERE user_id=$2`,
		amount, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO balance_freezes (user_id, amount, ref_type, ref_id, status) VALUES ($1,$2,$3,$4,'frozen')`,
		userID, amount, refType, refID)
	if err != nil {
		return err
	}
	if finalize != nil {
		if err = finalize(tx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) Charge(ctx context.Context, userID int64, freezeAmount, actualAmount float64, refType, refID, txType, remark string) error {
	return s.ChargeWithFinalize(ctx, userID, freezeAmount, actualAmount, refType, refID, txType, remark, nil)
}

func (s *Service) ChargeWithFinalize(ctx context.Context, userID int64, freezeAmount, actualAmount float64, refType, refID, txType, remark string, finalize func(pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance, frozen float64
	err = tx.QueryRow(ctx,
		`SELECT compute_balance, frozen_compute FROM wallets WHERE user_id=$1 FOR UPDATE`, userID,
	).Scan(&balance, &frozen)
	if err != nil {
		return err
	}
	lockedAmount, err := sumLockedFreezes(ctx, tx, userID, refType, refID)
	if err != nil {
		return err
	}
	if lockedAmount <= 0 {
		if actualAmount > 0 {
			if finalize != nil {
				return ErrFreezeNotFound
			}
			var alreadyCharged bool
			if err = tx.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM wallet_transactions
				WHERE user_id=$1 AND direction='out' AND ref_type=$2 AND ref_id=$3)`,
				userID, refType, refID).Scan(&alreadyCharged); err != nil {
				return err
			}
			if !alreadyCharged {
				return ErrFreezeNotFound
			}
		}
		if finalize != nil {
			if err = finalize(tx); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		return nil
	}
	_ = freezeAmount // The database lock is authoritative; callers may hold a stale estimate.

	charge := actualAmount
	if charge < 0 {
		charge = 0
	}

	newBalance, newFrozen := settlementWalletValues(balance, frozen, lockedAmount, charge)

	_, err = tx.Exec(ctx,
		`UPDATE wallets SET compute_balance=$1, frozen_compute=$2, updated_at=now() WHERE user_id=$3`,
		newBalance, newFrozen, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE balance_freezes SET status='charged', released_at=now() WHERE user_id=$1 AND ref_type=$2 AND ref_id=$3 AND status='frozen'`,
		userID, refType, refID)
	if err != nil {
		return err
	}

	if charge > 0 {
		_, err = tx.Exec(ctx,
			`INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
			 VALUES ($1,$2,'out',$3,$4,$5,$6,$7)`,
			userID, txType, charge, newBalance, refType, refID, remark)
		if err != nil {
			return err
		}
	}
	if finalize != nil {
		if err = finalize(tx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func settlementWalletValues(balance, frozen, lockedAmount, actualAmount float64) (float64, float64) {
	if actualAmount < 0 {
		actualAmount = 0
	}
	newFrozen := frozen - lockedAmount
	if newFrozen < 0 {
		newFrozen = 0
	}
	return balance - actualAmount, newFrozen
}

func (s *Service) Unfreeze(ctx context.Context, userID int64, amount float64, refType, refID string) error {
	return s.UnfreezeWithFinalize(ctx, userID, amount, refType, refID, nil)
}

func (s *Service) UnfreezeWithFinalize(ctx context.Context, userID int64, amount float64, refType, refID string, finalize func(pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `SELECT 1 FROM wallets WHERE user_id=$1 FOR UPDATE`, userID); err != nil {
		return err
	}
	lockedAmount, err := sumLockedFreezes(ctx, tx, userID, refType, refID)
	if err != nil {
		return err
	}
	if lockedAmount <= 0 {
		if finalize != nil {
			if err = finalize(tx); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		return nil
	}
	_ = amount // Always release the complete active reservation for this reference.

	_, err = tx.Exec(ctx,
		`UPDATE wallets SET frozen_compute = GREATEST(frozen_compute - $1, 0), updated_at=now() WHERE user_id=$2`,
		lockedAmount, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE balance_freezes SET status='released', released_at=now() WHERE user_id=$1 AND ref_type=$2 AND ref_id=$3 AND status='frozen'`,
		userID, refType, refID)
	if err != nil {
		return err
	}
	if finalize != nil {
		if err = finalize(tx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func sumLockedFreezes(ctx context.Context, tx pgx.Tx, userID int64, refType, refID string) (float64, error) {
	rows, err := tx.Query(ctx,
		`SELECT amount FROM balance_freezes
		 WHERE user_id=$1 AND ref_type=$2 AND ref_id=$3 AND status='frozen'
		 FOR UPDATE`,
		userID, refType, refID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	total := 0.0
	for rows.Next() {
		var amount float64
		if err := rows.Scan(&amount); err != nil {
			return 0, err
		}
		total += amount
	}
	return total, rows.Err()
}

func (s *Service) Credit(ctx context.Context, userID int64, amount float64, txType, refType, refID, remark string) error {
	return s.CreditWithFinalize(ctx, userID, amount, txType, refType, refID, remark, nil)
}

func (s *Service) CreditWithFinalize(ctx context.Context, userID int64, amount float64, txType, refType, refID, remark string, finalize func(pgx.Tx) error) error {
	if amount <= 0 {
		return errors.New("credit amount must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance float64
	err = tx.QueryRow(ctx,
		`SELECT compute_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, userID,
	).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `INSERT INTO wallets (user_id, compute_balance) VALUES ($1, $2)`, userID, amount)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx,
				`INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
				 VALUES ($1,$2,'in',$3,$3,$4,$5,$6)`,
				userID, txType, amount, refType, refID, remark)
			if err != nil {
				return err
			}
			if finalize != nil {
				if err = finalize(tx); err != nil {
					return err
				}
			}
			return tx.Commit(ctx)
		}
		return err
	}

	newBalance := balance + amount
	_, err = tx.Exec(ctx,
		`UPDATE wallets SET compute_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
		 VALUES ($1,$2,'in',$3,$4,$5,$6,$7)`,
		userID, txType, amount, newBalance, refType, refID, remark)
	if err != nil {
		return err
	}
	if finalize != nil {
		if err = finalize(tx); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) CreditCash(ctx context.Context, userID int64, amount float64, txType, refType, refID, remark string) error {
	if amount <= 0 {
		return errors.New("cash credit amount must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance float64
	err = tx.QueryRow(ctx, `SELECT cash_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `INSERT INTO wallets (user_id, cash_balance) VALUES ($1, $2)`, userID, amount)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx,
				`INSERT INTO cash_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
				 VALUES ($1,$2,'in',$3,$3,$4,$5,$6)`,
				userID, txType, amount, refType, refID, remark)
			if err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		return err
	}

	newBalance := balance + amount
	_, err = tx.Exec(ctx, `UPDATE wallets SET cash_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO cash_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
		 VALUES ($1,$2,'in',$3,$4,$5,$6,$7)`,
		userID, txType, amount, newBalance, refType, refID, remark)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) DebitCash(ctx context.Context, userID int64, amount float64, txType, refType, refID, remark string) error {
	if amount <= 0 {
		return errors.New("cash debit amount must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance float64
	if err = tx.QueryRow(ctx, `SELECT cash_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return err
	}
	if balance < amount {
		return ErrInsufficientBalance
	}
	newBalance := balance - amount
	if _, err = tx.Exec(ctx, `UPDATE wallets SET cash_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO cash_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
		 VALUES ($1,$2,'out',$3,$4,$5,$6,$7)`,
		userID, txType, amount, newBalance, refType, refID, remark); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AwardReferralOnRecharge(ctx context.Context, referredID int64, paidAmount, creditedAmount float64, triggerType, triggerID string) error {
	var referrerID, levelID int64
	var account, rewardType, rewardTrigger string
	var rewardValue float64
	err := s.db.QueryRow(ctx, `
		SELECT u.referrer_id, ml.id, ml.referral_reward_account, ml.referral_reward_amount,
		       COALESCE(ml.referral_reward_type,'fixed'), COALESCE(ml.referral_reward_trigger,'first_recharge')
		FROM users u
		JOIN users r ON r.id = u.referrer_id
		JOIN member_levels ml ON ml.id = r.member_level_id
		WHERE u.id=$1 AND u.referrer_id IS NOT NULL AND ml.is_enabled=true`,
		referredID).Scan(&referrerID, &levelID, &account, &rewardValue, &rewardType, &rewardTrigger)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if rewardTrigger == "first_recharge" {
		var firstType, firstID string
		if err := s.db.QueryRow(ctx, `
			SELECT ref_type, ref_id FROM wallet_transactions
			WHERE user_id=$1 AND direction='in' AND type IN ('card_recharge','online_recharge')
			ORDER BY created_at ASC, id ASC LIMIT 1`, referredID).Scan(&firstType, &firstID); err != nil {
			return err
		}
		if firstType != triggerType || firstID != triggerID {
			return nil
		}
	}
	if rewardValue <= 0 {
		return nil
	}
	amount := referralRewardAmount(rewardValue, rewardType, account, paidAmount, creditedAmount)
	if amount <= 0 {
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var existing int
	if err = tx.QueryRow(ctx, `SELECT 1 FROM referral_rewards WHERE referred_id=$1 AND trigger_type=$2 AND trigger_id=$3`, referredID, triggerType, triggerID).Scan(&existing); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var balance float64
	if account == "cash" {
		if err = tx.QueryRow(ctx, `SELECT cash_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, referrerID).Scan(&balance); err != nil {
			return err
		}
		newBalance := balance + amount
		if _, err = tx.Exec(ctx, `UPDATE wallets SET cash_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, referrerID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx,
			`INSERT INTO cash_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
			 VALUES ($1,'referral_reward','in',$2,$3,$4,$5,'推荐奖励')`,
			referrerID, amount, newBalance, triggerType, triggerID); err != nil {
			return err
		}
	} else {
		if err = tx.QueryRow(ctx, `SELECT compute_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, referrerID).Scan(&balance); err != nil {
			return err
		}
		newBalance := balance + amount
		if _, err = tx.Exec(ctx, `UPDATE wallets SET compute_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, referrerID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx,
			`INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
			 VALUES ($1,'referral_reward','in',$2,$3,$4,$5,'推荐奖励')`,
			referrerID, amount, newBalance, triggerType, triggerID); err != nil {
			return err
		}
		account = "compute"
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO referral_rewards (referrer_id, referred_id, member_level_id, reward_account, amount, trigger_type, trigger_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		referrerID, referredID, levelID, account, amount, triggerType, triggerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func referralRewardAmount(rewardValue float64, rewardType, account string, paidAmount, creditedAmount float64) float64 {
	if rewardType != "percent" {
		return rewardValue
	}
	base := creditedAmount
	if account == "cash" && paidAmount > 0 {
		base = paidAmount
	}
	return base * rewardValue / 100
}

func (s *Service) ReconcileReferralRewards(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT wt.user_id, COALESCE(o.amount, wt.amount), wt.amount, wt.ref_type, wt.ref_id
		FROM wallet_transactions wt
		JOIN users u ON u.id=wt.user_id AND u.referrer_id IS NOT NULL
		JOIN users r ON r.id=u.referrer_id
		JOIN member_levels ml ON ml.id=r.member_level_id AND ml.is_enabled=true AND ml.referral_reward_amount>0
		LEFT JOIN orders o ON wt.ref_type='order' AND o.order_no=wt.ref_id
		LEFT JOIN referral_rewards rr
		  ON rr.referred_id=wt.user_id AND rr.trigger_type=wt.ref_type AND rr.trigger_id=wt.ref_id
		WHERE wt.direction='in' AND wt.type IN ('card_recharge','online_recharge') AND rr.id IS NULL
		  AND (COALESCE(ml.referral_reward_trigger,'first_recharge')='every_recharge' OR NOT EXISTS (
		    SELECT 1 FROM wallet_transactions prior
		    WHERE prior.user_id=wt.user_id AND prior.direction='in' AND prior.type IN ('card_recharge','online_recharge')
		      AND (prior.created_at < wt.created_at OR (prior.created_at=wt.created_at AND prior.id < wt.id))
		  ))
		ORDER BY wt.created_at ASC, wt.id ASC LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type pendingReward struct {
		userID                 int64
		paid, credited         float64
		triggerType, triggerID string
	}
	pending := make([]pendingReward, 0, limit)
	for rows.Next() {
		var item pendingReward
		if err := rows.Scan(&item.userID, &item.paid, &item.credited, &item.triggerType, &item.triggerID); err != nil {
			return 0, err
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range pending {
		if err := s.AwardReferralOnRecharge(ctx, item.userID, item.paid, item.credited, item.triggerType, item.triggerID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *Service) CountLedgerMismatches(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
		  SELECT w.user_id
		  FROM wallets w
		  JOIN LATERAL (
		    SELECT balance_after FROM wallet_transactions wt
		    WHERE wt.user_id=w.user_id ORDER BY wt.id DESC LIMIT 1
		  ) latest ON true
		  WHERE ABS(w.compute_balance-latest.balance_after) > 0.000001
		  UNION
		  SELECT w.user_id
		  FROM wallets w
		  JOIN LATERAL (
		    SELECT balance_after FROM cash_transactions ct
		    WHERE ct.user_id=w.user_id ORDER BY ct.id DESC LIMIT 1
		  ) latest ON true
		  WHERE ABS(w.cash_balance-latest.balance_after) > 0.01
		) mismatches`).Scan(&count)
	return count, err
}

func (s *Service) AdjustBalance(ctx context.Context, userID int64, amount float64, remark string) error {
	if amount == 0 {
		return errors.New("adjustment amount must not be zero")
	}
	if amount >= 0 {
		return s.Credit(ctx, userID, amount, "admin_adjust", "admin", fmt.Sprintf("%d", userID), remark)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var balance float64
	err = tx.QueryRow(ctx, `SELECT compute_balance FROM wallets WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance)
	if err != nil {
		return err
	}
	deduct := -amount
	newBalance := balance - deduct
	_, err = tx.Exec(ctx, `UPDATE wallets SET compute_balance=$1, updated_at=now() WHERE user_id=$2`, newBalance, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO wallet_transactions (user_id, type, direction, amount, balance_after, ref_type, ref_id, remark)
		 VALUES ($1,'admin_adjust','out',$2,$3,'admin',$4,$5)`,
		userID, deduct, newBalance, fmt.Sprintf("%d", userID), remark)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
