package billing

import (
	"context"
	"time"
)

// CheckoutPlan names the fixed fixture catalog, without commercial price claims.
type CheckoutPlan struct {
	PriceID    string `json:"priceId"`
	Fixture    bool   `json:"fixture"`
	CanPrepare bool   `json:"canPrepare"`
}

// Plan reads current original-owner preparation availability without mutation.
func (s *CheckoutIntents) Plan(ctx context.Context, owner, org string) (CheckoutPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, owner, org, "FOR SHARE")
	if err != nil {
		return CheckoutPlan{}, err
	}
	defer tx.Rollback(ctx)
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_checkout_requests WHERE organization_id=$1) OR EXISTS(SELECT 1 FROM billing_customer_bindings WHERE organization_id=$1) OR EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org).Scan(&blocked); err != nil {
		return CheckoutPlan{}, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return CheckoutPlan{}, err
	}
	result := CheckoutPlan{PriceID: s.settings.PriceID, Fixture: true, CanPrepare: !blocked}
	if err = tx.Commit(ctx); err != nil {
		return CheckoutPlan{}, err
	}
	return result, nil
}
