package billing

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// CompletionArgs contains only the retained notification identifier.
type CompletionArgs struct {
	EventID string `json:"eventId"`
}

func (CompletionArgs) Kind() string { return "billing_checkout_completion" }

// CompletionInbox retains signed claims, without mapping or fulfillment.
type CompletionInbox struct {
	pool   *pgxpool.Pool
	queue  *river.Client[pgx.Tx]
	secret string
}

func NewCompletionInbox(local bool, pool *pgxpool.Pool, adapter *FixtureSessions) (*CompletionInbox, error) {
	if !local || pool == nil || adapter == nil || adapter.client == nil || !adapter.settings.Billing.Enabled || !strings.HasPrefix(adapter.settings.Billing.WebhookSecret, "whsec_") || len(adapter.settings.Billing.WebhookSecret) < 38 {
		return nil, ErrUnavailable
	}
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &CompletionInbox{pool: pool, queue: queue, secret: adapter.settings.Billing.WebhookSecret}, nil
}
func completionType(kind string) bool {
	return kind == "checkout.session.completed" || kind == "checkout.session.async_payment_succeeded" || kind == "checkout.session.async_payment_failed"
}

// billingEventLock serializes delivery identity across the two independently gated inboxes.
func billingEventLock(ctx context.Context, tx pgx.Tx, id, other string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,73647333))`, id); err != nil {
		return err
	}
	if err := billingRecovery(ctx, tx); err != nil {
		return err
	}
	// Table names are fixed by internal callers, never request input.
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+other+` WHERE event_id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	return nil
}

// Receive commits the projection and one job atomically. Unknown request claims remain pending.
func (s *CompletionInbox) Receive(ctx context.Context, e CompletionEvent) (bool, error) {
	if s == nil || s.pool == nil {
		return false, ErrUnavailable
	}
	if !completionType(e.Type) || !eventID.MatchString(e.ID) || !checkoutID.MatchString(e.Checkout) || !customerID.MatchString(e.Customer) || !subscriptionID.MatchString(e.Subscription) || !uuid.MatchString(e.Request) || e.Created <= 0 || len(e.Digest) != 32 {
		return false, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	if err = billingEventLock(ctx, tx, e.ID, "billing_events"); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO billing_checkout_notifications(event_id,event_type,event_created,payload_sha256,checkout_id,customer_id,subscription_id,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_id) DO NOTHING`, e.ID, e.Type, e.Created, e.Digest, e.Checkout, e.Customer, e.Subscription, e.Request)
	if err != nil {
		return false, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		var retained CompletionEvent
		if err = tx.QueryRow(ctx, `SELECT event_type,event_created,checkout_id,customer_id,subscription_id,request_id::text FROM billing_checkout_notifications WHERE event_id=$1`, e.ID).Scan(&retained.Type, &retained.Created, &retained.Checkout, &retained.Customer, &retained.Subscription, &retained.Request); err != nil {
			return false, err
		}
		if retained.Type != e.Type || retained.Created != e.Created || retained.Checkout != e.Checkout || retained.Customer != e.Customer || retained.Subscription != e.Subscription || retained.Request != e.Request {
			return false, ErrConflict
		}
	} else if _, err = s.queue.InsertTx(ctx, tx, CompletionArgs{EventID: e.ID}, &river.InsertOpts{MaxAttempts: 5}); err != nil {
		return false, err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return false, err
	}
	return tag.RowsAffected() == 0, tx.Commit(ctx)
}

// CompletionWorker leaves disabled, held, unmapped and changed-custody work pending.
type CompletionWorker struct {
	river.WorkerDefaults[CompletionArgs]
	Inbox     *CompletionInbox
	Intents   *CheckoutIntents
	Authority *FixtureSessions
}

func (w *CompletionWorker) Work(ctx context.Context, job *river.Job[CompletionArgs]) error {
	err := w.Inbox.Reconcile(ctx, job.Args.EventID, w.Intents, w.Authority)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrUnmapped) || errors.Is(err, ErrConflict) || errors.Is(err, ErrCheckoutNotFound) || errors.Is(err, recovery.ErrHeld) {
		return river.JobSnooze(time.Minute)
	}
	return errors.New("checkout completion could not be confirmed")
}
