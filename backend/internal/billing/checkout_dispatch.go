package billing

import (
	"bytes"
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// CheckoutArgs queues only the retained preparation UUID.
type CheckoutArgs struct {
	RequestID string `json:"requestId"`
}

func (CheckoutArgs) Kind() string { return "billing_checkout" }

// CheckoutAuthority is bounded fixture transport, without host or payment fulfillment authority.
type CheckoutAuthority interface {
	CreateCustomer(context.Context, string) (string, error)
	Checkout(context.Context, SessionRequest) (HostedSession, error)
}

func checkoutAudit(ctx context.Context, tx pgx.Tx, id, org, owner, action, executor string) error {
	_, err := tx.Exec(ctx, `INSERT INTO billing_checkout_events(request_id,organization_id,actor_id,action,executor_kind) VALUES($1,$2,$3,$4,$5)`, id, org, owner, action, executor)
	return err
}

func (s *CheckoutIntents) configuration(ctx context.Context, tx pgx.Tx, id, owner string) error {
	var actor, price, origin, state string
	var authorityOrigin *string
	var digest []byte
	if err := tx.QueryRow(ctx, `SELECT actor_id,price_id,return_origin,state,authority_origin,authority_key_sha256 FROM billing_checkout_requests WHERE id=$1 FOR UPDATE`, id).Scan(&actor, &price, &origin, &state, &authorityOrigin, &digest); err != nil {
		return err
	}
	if actor != owner {
		return ErrCheckoutNotFound
	}
	if price != s.settings.PriceID || origin != s.settings.ReturnOrigin || state != "prepared" && (authorityOrigin == nil || *authorityOrigin != s.settings.Billing.FixtureURL || !bytes.Equal(digest, s.keyDigest[:])) {
		return ErrConflict
	}
	return nil
}

func checkoutPendingAuthority(ctx context.Context, tx pgx.Tx, org string) error {
	var blocked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE organization_id=$1) OR EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return ErrConflict
	}
	return nil
}

// Start queues an explicit original-owner dispatch and its audit atomically.
// Queued and terminal retries preserve the original job; preparation alone never starts it.
func (s *CheckoutIntents) Start(ctx context.Context, owner, org, id string, confirmed bool) (CheckoutIntent, error) {
	if !confirmed || !uuid.MatchString(id) {
		return CheckoutIntent{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, owner, org, "FOR UPDATE")
	if err != nil {
		return CheckoutIntent{}, err
	}
	defer tx.Rollback(ctx)
	result, err := scanCheckoutIntent(tx.QueryRow(ctx, `SELECT `+checkoutIntentColumns+` FROM billing_checkout_requests WHERE id=$1 AND organization_id=$2 AND actor_id=$3`, id, org, owner))
	if err != nil {
		return CheckoutIntent{}, err
	}
	if err = s.configuration(ctx, tx, id, owner); err != nil {
		return CheckoutIntent{}, err
	}
	if result.State != "prepared" {
		if err = billingRecovery(ctx, tx); err != nil {
			return CheckoutIntent{}, err
		}
		return result, tx.Commit(ctx)
	}
	if err = checkoutPendingAuthority(ctx, tx, org); err != nil {
		return CheckoutIntent{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE billing_checkout_requests SET state='queued',authority_origin=$2,authority_key_sha256=$3 WHERE id=$1`, id, s.settings.Billing.FixtureURL, s.keyDigest[:]); err != nil {
		return CheckoutIntent{}, err
	}
	if err = checkoutAudit(ctx, tx, id, org, owner, "queue", "owner"); err != nil {
		return CheckoutIntent{}, err
	}
	if _, err = s.queue.InsertTx(ctx, tx, CheckoutArgs{RequestID: id}, &river.InsertOpts{MaxAttempts: 5}); err != nil {
		return CheckoutIntent{}, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return CheckoutIntent{}, err
	}
	result.State = "queued"
	return result, tx.Commit(ctx)
}

func (s *CheckoutIntents) dispatchScope(ctx context.Context, id string) (pgx.Tx, string, string, error) {
	if s == nil || s.pool == nil || !uuid.MatchString(id) {
		return nil, "", "", ErrUnavailable
	}
	var owner, org string
	if err := s.pool.QueryRow(ctx, `SELECT actor_id,organization_id FROM billing_checkout_requests WHERE id=$1`, id).Scan(&owner, &org); err != nil {
		return nil, "", "", ErrCheckoutNotFound
	}
	tx, err := s.begin(ctx, owner, org, "FOR UPDATE")
	if err != nil {
		return nil, "", "", err
	}
	if err = s.configuration(ctx, tx, id, owner); err != nil {
		_ = tx.Rollback(ctx)
		return nil, "", "", err
	}
	return tx, owner, org, nil
}

// reserveDispatch commits the original remote retry identity before any RPC.
func (s *CheckoutIntents) reserveDispatch(ctx context.Context, id string) error {
	tx, owner, org, err := s.dispatchScope(ctx, id)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	var started *time.Time
	if err = tx.QueryRow(ctx, `SELECT state,dispatch_started_at FROM billing_checkout_requests WHERE id=$1`, id).Scan(&state, &started); err != nil {
		return err
	}
	if state == "prepared" {
		return ErrConflict
	}
	if state == "session_ready" || state == "manual_review" || started != nil {
		if err = billingRecovery(ctx, tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE billing_checkout_requests SET dispatch_started_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		return err
	}
	if err = checkoutAudit(ctx, tx, id, org, owner, "dispatch_reserve", "worker"); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Dispatch checkpoints first-class customer/session IDs separately. Unknown outcomes
// reuse only the original key within 23 hours; expiry requires manual review.
func (s *CheckoutIntents) Dispatch(ctx context.Context, id string, authority CheckoutAuthority) error {
	if authority == nil {
		return ErrUnavailable
	}
	if a, ok := authority.(*FixtureSessions); ok && (s == nil || a == nil || a.settings.Billing.FixtureURL != s.settings.Billing.FixtureURL || a.keyDigest != s.keyDigest || a.settings.PriceID != s.settings.PriceID || a.settings.ReturnOrigin != s.settings.ReturnOrigin) {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.reserveDispatch(ctx, id); err != nil {
		return err
	}
	for range 2 {
		tx, owner, org, err := s.dispatchScope(ctx, id)
		if err != nil {
			return err
		}
		err = s.dispatchStep(ctx, tx, id, owner, org, authority)
		_ = tx.Rollback(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *CheckoutIntents) dispatchStep(ctx context.Context, tx pgx.Tx, id, owner, org string, authority CheckoutAuthority) error {
	var state string
	var started time.Time
	var customer *string
	if err := tx.QueryRow(ctx, `SELECT state,dispatch_started_at,customer_id FROM billing_checkout_requests WHERE id=$1`, id).Scan(&state, &started, &customer); err != nil {
		return err
	}
	if state == "session_ready" || state == "manual_review" {
		if err := billingRecovery(ctx, tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if state != "queued" && state != "customer_ready" {
		return ErrConflict
	}
	if err := checkoutPendingAuthority(ctx, tx, org); err != nil {
		return err
	}
	if err := billingRecovery(ctx, tx); err != nil {
		return err
	}
	if started.After(time.Now().Add(time.Minute)) {
		return ErrUnavailable
	}
	if time.Now().After(started.Add(23 * time.Hour)) {
		if _, err := tx.Exec(ctx, `UPDATE billing_checkout_requests SET state='manual_review' WHERE id=$1`, id); err != nil {
			return err
		}
		if err := checkoutAudit(ctx, tx, id, org, owner, "manual_review", "worker"); err != nil {
			return err
		}
		if err := billingRecovery(ctx, tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if customer == nil {
		value, err := authority.CreateCustomer(ctx, id)
		if err != nil || !customerID.MatchString(value) {
			return ErrUnavailable
		}
		if err = billingRecovery(ctx, tx); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE billing_checkout_requests SET customer_id=$2,state='customer_ready' WHERE id=$1`, id, value); err != nil {
			return err
		}
		if err = checkoutAudit(ctx, tx, id, org, owner, "customer_confirmed", "worker"); err != nil {
			return err
		}
	} else {
		session, err := authority.Checkout(ctx, SessionRequest{Key: id, CustomerID: *customer})
		if err != nil || !checkoutID.MatchString(session.ID) || session.ExpiresAt.Before(time.Now()) {
			return ErrUnavailable
		}
		if err = billingRecovery(ctx, tx); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE billing_checkout_requests SET checkout_id=$2,session_expires_at=$3,state='session_ready' WHERE id=$1`, id, session.ID, session.ExpiresAt); err != nil {
			return err
		}
		if err = checkoutAudit(ctx, tx, id, org, owner, "session_confirmed", "worker"); err != nil {
			return err
		}
	}
	if err := billingRecovery(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CheckoutWorker leaves disabled, held, changed-custody and uncertain fixture work pending.
type CheckoutWorker struct {
	river.WorkerDefaults[CheckoutArgs]
	Store     *CheckoutIntents
	Authority CheckoutAuthority
}

func (w *CheckoutWorker) Work(ctx context.Context, job *river.Job[CheckoutArgs]) error {
	if w.Store == nil || w.Authority == nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.Store.Dispatch(ctx, job.Args.RequestID, w.Authority)
	if err == nil {
		return nil
	}
	if errors.Is(err, recovery.ErrHeld) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrConflict) || errors.Is(err, ErrCheckoutNotFound) {
		return river.JobSnooze(time.Minute)
	}
	return errors.New("checkout dispatch could not be confirmed")
}
