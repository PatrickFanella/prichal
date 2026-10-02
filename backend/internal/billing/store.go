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

// Args queues only the retained event identifier.
type Args struct {
	EventID string `json:"eventId"`
}

func (Args) Kind() string { return "billing_reconcile" }

// Store owns the signed event inbox. This fixture-only store never changes entitlements.
type Store struct {
	Pool  *pgxpool.Pool
	Queue *river.Client[pgx.Tx]
}

func New(pool *pgxpool.Pool) (*Store, error) {
	q, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{pool, q}, nil
}

// Receive commits notification identity and durable work together, or neither.
func (s *Store) Receive(ctx context.Context, event Event) (bool, error) {
	if !supportedType(event.Type) || !eventID.MatchString(event.ID) || !customerID.MatchString(event.Customer) || !subscriptionID.MatchString(event.Subscription) || len(event.Digest) != 32 || event.Created <= 0 {
		return false, ErrInvalidEvent
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	if err = billingEventLock(ctx, tx, event.ID, "billing_checkout_notifications"); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO billing_events(event_id,payload_sha256,event_type,event_created,customer_id,subscription_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id) DO NOTHING`, event.ID, event.Digest, event.Type, event.Created, event.Customer, event.Subscription)
	if err != nil {
		return false, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		var retained Event
		if err = tx.QueryRow(ctx, `SELECT event_type,event_created,customer_id,subscription_id FROM billing_events WHERE event_id=$1`, event.ID).Scan(&retained.Type, &retained.Created, &retained.Customer, &retained.Subscription); err != nil {
			return false, err
		}
		// Delivery metadata such as pending_webhooks can change. Only immutable
		// reconciliation identity participates in deduplication; raw content is never used to fulfill.
		if retained.Type != event.Type || retained.Created != event.Created || retained.Customer != event.Customer || retained.Subscription != event.Subscription {
			return false, ErrConflict
		}
		if err = billingRecovery(ctx, tx); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{EventID: event.ID}, &river.InsertOpts{MaxAttempts: 5}); err != nil {
		return false, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

// Snapshot contains only current subscription identifiers/status, never payment content.
type Snapshot struct {
	Customer, Subscription, State, Price string
	Quantity                             int
	PeriodEnd                            time.Time
}

// Authority reads current state instead of trusting event delivery order or snapshot status.
type Authority interface {
	Subscription(context.Context, string) (Snapshot, error)
}

// Reconcile re-fetches after failures and serializes authority reads by organization.
func (s *Store) Reconcile(ctx context.Context, id string, authority Authority) error {
	if authority == nil {
		return ErrUnavailable
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
	var event Event
	var state string
	err = tx.QueryRow(ctx, `SELECT customer_id,subscription_id,state FROM billing_events WHERE event_id=$1 FOR UPDATE`, id).Scan(&event.Customer, &event.Subscription, &state)
	if err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if state == "applied" {
		return tx.Commit(ctx)
	}
	var org string
	err = tx.QueryRow(ctx, `SELECT organization_id FROM billing_customer_bindings WHERE customer_id=$1 AND subscription_id=$2`, event.Customer, event.Subscription).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnmapped
	}
	if err != nil {
		return err
	}
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&found); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	// Fetch inside the organization lock: a slower old event cannot overwrite a newer read.
	snapshot, err := authority.Subscription(ctx, event.Subscription)
	if err != nil {
		return ErrUnavailable
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if snapshot.Customer != event.Customer || snapshot.Subscription != event.Subscription || !subscriptionState(snapshot.State) || !priceID.MatchString(snapshot.Price) || snapshot.Quantity != 1 || snapshot.PeriodEnd.IsZero() {
		return ErrInvalidEvent
	}
	_, err = tx.Exec(ctx, `INSERT INTO billing_subscription_observations(organization_id,customer_id,subscription_id,subscription_state,price_id,quantity,period_end,event_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(organization_id) DO UPDATE SET customer_id=excluded.customer_id,subscription_id=excluded.subscription_id,subscription_state=excluded.subscription_state,price_id=excluded.price_id,quantity=excluded.quantity,period_end=excluded.period_end,event_id=excluded.event_id,checked_at=clock_timestamp()`, org, snapshot.Customer, snapshot.Subscription, snapshot.State, snapshot.Price, snapshot.Quantity, snapshot.PeriodEnd, id)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE billing_events SET state='applied',applied_at=clock_timestamp() WHERE event_id=$1`, id); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Worker applies only current authoritative fixture observations. Held/unconfigured work waits.
type Worker struct {
	river.WorkerDefaults[Args]
	Store     *Store
	Authority Authority
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	err := w.Store.Reconcile(ctx, job.Args.EventID, w.Authority)
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrUnmapped) || errors.Is(err, recovery.ErrHeld) {
		return river.JobSnooze(time.Minute)
	}
	if err != nil {
		return errors.New("billing reconciliation could not be confirmed")
	}
	return nil
}
