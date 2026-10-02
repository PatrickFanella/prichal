package billing

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Reconcile maps only original/current-owner checkout identity and current graph.
// Binding, original-owner worker audit, observation and delivery receipt commit together.
// No payment status or subscription state grants hosting entitlement here.
func (s *CompletionInbox) Reconcile(ctx context.Context, id string, intents *CheckoutIntents, adapter *FixtureSessions) error {
	if s == nil || intents == nil || adapter == nil || s.pool != intents.pool || adapter.client == nil || adapter.settings != intents.settings || s.secret != adapter.settings.Billing.WebhookSecret || !bytes.Equal(adapter.keyDigest[:], intents.keyDigest[:]) {
		return ErrUnavailable
	}
	if !eventID.MatchString(id) {
		return ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	var event CompletionEvent
	var state string
	if err = tx.QueryRow(ctx, `SELECT request_id::text,checkout_id,customer_id,subscription_id,state FROM billing_checkout_notifications WHERE event_id=$1 FOR UPDATE`, id).Scan(&event.Request, &event.Checkout, &event.Customer, &event.Subscription, &state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnmapped
		}
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if state == "applied" {
		return tx.Commit(ctx)
	}
	var org, original, current string
	if err = tx.QueryRow(ctx, `SELECT organization_id,actor_id FROM billing_checkout_requests WHERE id=$1`, event.Request).Scan(&org, &original); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnmapped
		}
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&current); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if current != original {
		return ErrCheckoutNotFound
	}
	if err = intents.configuration(ctx, tx, event.Request, original); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	var requestState string
	var customer, checkout *string
	var expires *time.Time
	if err = tx.QueryRow(ctx, `SELECT state,customer_id,checkout_id,session_expires_at FROM billing_checkout_requests WHERE id=$1`, event.Request).Scan(&requestState, &customer, &checkout, &expires); err != nil {
		return err
	}
	if requestState != "session_ready" || customer == nil || checkout == nil || expires == nil || *customer != event.Customer || *checkout != event.Checkout {
		return ErrUnmapped
	}
	var transfer bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org).Scan(&transfer); err != nil {
		return err
	}
	if transfer {
		return ErrConflict
	}
	var existingCustomer, existingSubscription, reason string
	var existingRequest, existingOwner *string
	err = tx.QueryRow(ctx, `SELECT customer_id,subscription_id,reason,checkout_request_id::text,checkout_actor_id::text FROM billing_customer_bindings WHERE organization_id=$1`, org).Scan(&existingCustomer, &existingSubscription, &reason, &existingRequest, &existingOwner)
	bound := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if bound && (reason != "checkout_verified" || existingRequest == nil || *existingRequest != event.Request || existingOwner == nil || *existingOwner != original || existingCustomer != event.Customer || existingSubscription != event.Subscription) {
		return ErrConflict
	}
	if !bound {
		var collision bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE customer_id=$1 OR subscription_id=$2)`, event.Customer, event.Subscription).Scan(&collision); err != nil {
			return err
		}
		if collision {
			return ErrConflict
		}
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	graph, err := adapter.CompletedGraph(ctx, SessionRequest{Key: event.Request, CustomerID: event.Customer}, event.Checkout, event.Subscription, *expires)
	if err != nil {
		return ErrUnavailable
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	if !bound {
		if _, err = tx.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason,checkout_request_id,checkout_actor_id,completion_event_id) VALUES($1,$2,$3,$4,'billing_completion_worker','checkout_verified',$4,$5,$6)`, org, event.Customer, event.Subscription, event.Request, original, id); err != nil {
			var constraint *pgconn.PgError
			if errors.As(err, &constraint) && constraint.Code == "23505" {
				return ErrConflict
			}
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO billing_checkout_events(request_id,organization_id,actor_id,action,executor_kind,completion_event_id) VALUES($1,$2,$3,'completion_bound','worker',$4)`, event.Request, org, original, id); err != nil {
			return err
		}
	}
	snapshot := graph.Subscription
	if _, err = tx.Exec(ctx, `INSERT INTO billing_subscription_observations(organization_id,customer_id,subscription_id,subscription_state,price_id,quantity,period_end,event_id,completion_event_id,checkout_payment_state) VALUES($1,$2,$3,$4,$5,$6,$7,NULL,$8,$9) ON CONFLICT(organization_id) DO UPDATE SET customer_id=excluded.customer_id,subscription_id=excluded.subscription_id,subscription_state=excluded.subscription_state,price_id=excluded.price_id,quantity=excluded.quantity,period_end=excluded.period_end,event_id=NULL,completion_event_id=excluded.completion_event_id,checkout_payment_state=excluded.checkout_payment_state,checked_at=clock_timestamp()`, org, snapshot.Customer, snapshot.Subscription, snapshot.State, snapshot.Price, snapshot.Quantity, snapshot.PeriodEnd, id, graph.Session.PaymentState); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE billing_checkout_notifications SET state='applied',applied_at=clock_timestamp() WHERE event_id=$1`, id); err != nil {
		return err
	}
	if err = billingRecovery(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
