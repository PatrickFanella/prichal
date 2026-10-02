package billing

import (
	"context"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

// RecoveredCheckout contains a transient navigation capability and no entitlement.
// URL is empty for complete/expired sessions and must never be logged or persisted.
type RecoveredCheckout struct {
	Status    string    `json:"status"`
	URL       string    `json:"url,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// RetrieveCheckout reads only an exact checkpointed session through the pinned SDK.
// It never creates, renews or expires a remote resource.
func (a *FixtureSessions) RetrieveCheckout(ctx context.Context, request SessionRequest, id string, expires time.Time) (RecoveredCheckout, error) {
	if a == nil || a.client == nil || !sessionRequestValid(request) || !checkoutID.MatchString(id) || expires.IsZero() {
		return RecoveredCheckout{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	p := &stripe.CheckoutSessionRetrieveParams{}
	p.AddExpand("line_items")
	session, err := a.client.V1CheckoutSessions.Retrieve(ctx, id, p)
	if err != nil || session == nil || session.ID != id || session.Livemode || session.Object != "checkout.session" || session.Customer == nil || session.Customer.ID != request.CustomerID || session.ClientReferenceID != request.Key || session.Mode != "subscription" || session.UIMode != "hosted_page" || session.SuccessURL != a.settings.ReturnOrigin+"/billing/return" || session.CancelURL != a.settings.ReturnOrigin+"/billing/cancel" || session.ExpiresAt != expires.Unix() || session.LineItems == nil || session.LineItems.HasMore || len(session.LineItems.Data) != 1 {
		return RecoveredCheckout{}, ErrUnavailable
	}
	item := session.LineItems.Data[0]
	if item == nil || item.Price == nil || item.Price.ID != a.settings.PriceID || item.Quantity != 1 {
		return RecoveredCheckout{}, ErrUnavailable
	}
	result := RecoveredCheckout{Status: string(session.Status), ExpiresAt: expires}
	switch session.Status {
	case "open":
		if expires.After(time.Now()) {
			if !a.navigation(session.URL, "/checkout/"+id) {
				return RecoveredCheckout{}, ErrUnavailable
			}
			result.URL = session.URL
		} else {
			result.Status = "expired"
		}
	case "complete", "expired":
		// A closed session conveys no navigation or payment authority.
	default:
		return RecoveredCheckout{}, ErrUnavailable
	}
	return result, nil
}

// RecoverSession reads the original owner's confirmed hosted checkout under custody
// and recovery locks. Unknown/unfinished work cannot create replacement resources.
func (s *CheckoutIntents) RecoverSession(ctx context.Context, owner, org, id string, adapter *FixtureSessions) (RecoveredCheckout, error) {
	if !uuid.MatchString(id) {
		return RecoveredCheckout{}, ErrCheckoutNotFound
	}
	if s == nil || adapter == nil || adapter.client == nil || adapter.settings.Billing.FixtureURL != s.settings.Billing.FixtureURL || adapter.keyDigest != s.keyDigest || adapter.settings.PriceID != s.settings.PriceID || adapter.settings.ReturnOrigin != s.settings.ReturnOrigin {
		return RecoveredCheckout{}, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, owner, org, "FOR SHARE")
	if err != nil {
		return RecoveredCheckout{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	var customer, session *string
	var expires *time.Time
	err = tx.QueryRow(ctx, `SELECT state,customer_id,checkout_id,session_expires_at FROM billing_checkout_requests WHERE id=$1 AND organization_id=$2 AND actor_id=$3`, id, org, owner).Scan(&state, &customer, &session, &expires)
	if err != nil {
		return RecoveredCheckout{}, ErrCheckoutNotFound
	}
	if err = s.configuration(ctx, tx, id, owner); err != nil {
		return RecoveredCheckout{}, err
	}
	if state != "session_ready" || customer == nil || session == nil || expires == nil {
		return RecoveredCheckout{}, ErrConflict
	}
	if err = checkoutPendingAuthority(ctx, tx, org); err != nil {
		return RecoveredCheckout{}, err
	}
	if err = checkoutRecovery(ctx, tx); err != nil {
		return RecoveredCheckout{}, err
	}
	result := RecoveredCheckout{Status: "expired", ExpiresAt: *expires}
	if expires.After(time.Now()) {
		result, err = adapter.RetrieveCheckout(ctx, SessionRequest{Key: id, CustomerID: *customer}, *session, *expires)
		if err != nil {
			return RecoveredCheckout{}, err
		}
	}
	if err = checkoutRecovery(ctx, tx); err != nil {
		return RecoveredCheckout{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RecoveredCheckout{}, err
	}
	return result, nil
}
