package billing

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// ErrCheckoutNotFound deliberately conceals unknown or foreign checkout scope.
var ErrCheckoutNotFound = errors.New("checkout preparation unavailable")

// CheckoutIntents retains reviewed fixture preparation, without remote payment dispatch.
type CheckoutIntents struct {
	pool      *pgxpool.Pool
	settings  SessionSettings
	keyDigest [32]byte
	queue     *river.Client[pgx.Tx]
}

// CheckoutIntent is a safe owner receipt. It carries no navigation or payment capability.
type CheckoutIntent struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Key            string    `json:"idempotencyKey"`
	PriceID        string    `json:"priceId"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"createdAt"`
	CanPay         bool      `json:"canPay"`
}

// NewCheckoutIntents enables preparation only with a constructed local fixture adapter.
func NewCheckoutIntents(local bool, pool *pgxpool.Pool, adapter *FixtureSessions) (*CheckoutIntents, error) {
	if !local || pool == nil || adapter == nil || adapter.client == nil {
		return nil, ErrUnavailable
	}
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &CheckoutIntents{pool: pool, settings: adapter.settings, keyDigest: adapter.keyDigest, queue: queue}, nil
}

func checkoutRecovery(ctx context.Context, tx pgx.Tx) error {
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return ErrUnavailable
	}
	if held {
		return recovery.ErrHeld
	}
	return nil
}

func (s *CheckoutIntents) begin(ctx context.Context, owner, org, lock string) (pgx.Tx, error) {
	if s == nil || s.pool == nil || !uuid.MatchString(owner) || !uuid.MatchString(org) {
		return nil, ErrCheckoutNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var found string
	// Lock mode is fixed by the caller, never request input.
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2 `+lock, org, owner).Scan(&found)
	if err == nil {
		err = checkoutRecovery(ctx, tx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCheckoutNotFound
		}
		return nil, err
	}
	return tx, nil
}

const checkoutIntentColumns = `id,organization_id,idempotency_key,price_id,state,created_at`

func scanCheckoutIntent(row pgx.Row) (CheckoutIntent, error) {
	var result CheckoutIntent
	err := row.Scan(&result.ID, &result.OrganizationID, &result.Key, &result.PriceID, &result.State, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrCheckoutNotFound
	}
	return result, err
}

// Prepare records a reviewed configured plan and its audit together. It queues no work.
// An exact retry preserves original custody and configuration; a new key cannot replace it.
func (s *CheckoutIntents) Prepare(ctx context.Context, owner, org, key string, confirmed bool) (CheckoutIntent, error) {
	if !confirmed || !uuid.MatchString(key) {
		return CheckoutIntent{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, owner, org, "FOR UPDATE")
	if err != nil {
		return CheckoutIntent{}, err
	}
	defer tx.Rollback(ctx)
	var actor, price, origin string
	prior, err := scanCheckoutIntent(tx.QueryRow(ctx, `SELECT `+checkoutIntentColumns+` FROM billing_checkout_requests WHERE organization_id=$1 AND idempotency_key=$2`, org, key))
	if err == nil {
		if err = tx.QueryRow(ctx, `SELECT actor_id,price_id,return_origin FROM billing_checkout_requests WHERE id=$1`, prior.ID).Scan(&actor, &price, &origin); err != nil {
			return CheckoutIntent{}, err
		}
		if actor != owner {
			return CheckoutIntent{}, ErrCheckoutNotFound
		}
		if price != s.settings.PriceID || origin != s.settings.ReturnOrigin {
			return CheckoutIntent{}, ErrConflict
		}
		if err = checkoutRecovery(ctx, tx); err != nil {
			return CheckoutIntent{}, err
		}
		return prior, tx.Commit(ctx)
	}
	if !errors.Is(err, ErrCheckoutNotFound) {
		return CheckoutIntent{}, err
	}
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_checkout_requests WHERE organization_id=$1) OR EXISTS(SELECT 1 FROM billing_customer_bindings WHERE organization_id=$1) OR EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org).Scan(&blocked)
	if err != nil {
		return CheckoutIntent{}, err
	}
	if blocked {
		return CheckoutIntent{}, ErrConflict
	}
	result, err := scanCheckoutIntent(tx.QueryRow(ctx, `INSERT INTO billing_checkout_requests(organization_id,actor_id,idempotency_key,price_id,return_origin) VALUES($1,$2,$3,$4,$5) RETURNING `+checkoutIntentColumns, org, owner, key, s.settings.PriceID, s.settings.ReturnOrigin))
	if err != nil {
		return CheckoutIntent{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO billing_checkout_events(request_id,organization_id,actor_id,action) VALUES($1,$2,$3,'prepare')`, result.ID, org, owner); err != nil {
		return CheckoutIntent{}, err
	}
	if err = checkoutRecovery(ctx, tx); err != nil {
		return CheckoutIntent{}, err
	}
	return result, tx.Commit(ctx)
}

// Get reads only the original current owner's exact preparation under custody/recovery locks.
func (s *CheckoutIntents) Get(ctx context.Context, owner, org, id string) (CheckoutIntent, error) {
	return s.get(ctx, owner, org, id, false)
}

// GetByKey recovers an original preparation after a lost response without mutation.
func (s *CheckoutIntents) GetByKey(ctx context.Context, owner, org, key string) (CheckoutIntent, error) {
	return s.get(ctx, owner, org, key, true)
}

func (s *CheckoutIntents) get(ctx context.Context, owner, org, id string, byKey bool) (CheckoutIntent, error) {
	if !uuid.MatchString(id) {
		return CheckoutIntent{}, ErrCheckoutNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, owner, org, "FOR SHARE")
	if err != nil {
		return CheckoutIntent{}, err
	}
	defer tx.Rollback(ctx)
	column := "id"
	if byKey {
		column = "idempotency_key"
	}
	result, err := scanCheckoutIntent(tx.QueryRow(ctx, `SELECT `+checkoutIntentColumns+` FROM billing_checkout_requests WHERE `+column+`=$1 AND organization_id=$2 AND actor_id=$3`, id, org, owner))
	if err != nil {
		return CheckoutIntent{}, err
	}
	if err = checkoutRecovery(ctx, tx); err != nil {
		return CheckoutIntent{}, err
	}
	return result, tx.Commit(ctx)
}
