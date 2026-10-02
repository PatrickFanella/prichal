package billing

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Binding is an audited fixture association by first-class customer/subscription IDs.
type Binding struct {
	OrganizationID string `json:"organizationId"`
	CustomerID     string `json:"customerId"`
	SubscriptionID string `json:"subscriptionId"`
	Key            string `json:"idempotencyKey"`
	Actor          string `json:"actor"`
}

// Bind creates an immutable local test association. Owner metadata/email never selects ownership.
func (s *Store) Bind(ctx context.Context, b Binding) error {
	if !uuid.MatchString(b.OrganizationID) || !uuid.MatchString(b.Key) || !customerID.MatchString(b.CustomerID) || !subscriptionID.MatchString(b.SubscriptionID) || b.Actor == "" || len(b.Actor) > 100 {
		return ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, b.OrganizationID).Scan(&found); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	var existing Binding
	err = tx.QueryRow(ctx, `SELECT organization_id,customer_id,subscription_id,idempotency_key,actor FROM billing_customer_bindings WHERE organization_id=$1`, b.OrganizationID).Scan(&existing.OrganizationID, &existing.CustomerID, &existing.SubscriptionID, &existing.Key, &existing.Actor)
	if err == nil {
		if existing != b {
			return ErrConflict
		}
		if err = billingRecovery(ctx, tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason) VALUES($1,$2,$3,$4,$5,'qualification')`, b.OrganizationID, b.CustomerID, b.SubscriptionID, b.Key, b.Actor); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
