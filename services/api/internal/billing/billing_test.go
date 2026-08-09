package billing

import (
	"context"
	"testing"
)

func TestSettlementChargesActualAboveReservation(t *testing.T) {
	balance, frozen := settlementWalletValues(100, 20, 10, 35)
	if balance != 65 {
		t.Fatalf("balance = %v, want 65", balance)
	}
	if frozen != 10 {
		t.Fatalf("frozen = %v, want 10", frozen)
	}
}

func TestSettlementReleasesCompleteDuplicateReservation(t *testing.T) {
	balance, frozen := settlementWalletValues(100, 25, 20, 8)
	if balance != 92 || frozen != 5 {
		t.Fatalf("balance/frozen = %v/%v, want 92/5", balance, frozen)
	}
}

func TestSettlementNeverCreditsNegativeActualCost(t *testing.T) {
	balance, frozen := settlementWalletValues(100, 10, 10, -5)
	if balance != 100 || frozen != 0 {
		t.Fatalf("balance/frozen = %v/%v, want 100/0", balance, frozen)
	}
}

func TestCashReferralPercentUsesPaidCurrencyAmount(t *testing.T) {
	if got := referralRewardAmount(10, "percent", "cash", 20, 2000); got != 2 {
		t.Fatalf("reward = %v, want 2", got)
	}
}

func TestComputeReferralPercentUsesCreditedAmount(t *testing.T) {
	if got := referralRewardAmount(10, "percent", "compute", 20, 2000); got != 200 {
		t.Fatalf("reward = %v, want 200", got)
	}
}

func TestCreditRejectsNonPositiveAmountBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	if err := service.Credit(context.Background(), 1, 0, "test", "test", "1", "test"); err == nil {
		t.Fatal("zero credit was accepted")
	}
	if err := service.Credit(context.Background(), 1, -1, "test", "test", "1", "test"); err == nil {
		t.Fatal("negative credit was accepted")
	}
}
