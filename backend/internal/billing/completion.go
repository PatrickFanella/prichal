package billing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

// CompletionEvent carries signed first-class claims only. Its reference does not
// select ownership until matched to retained IDs and current authoritative graph.
type CompletionEvent struct {
	ID, Type, Checkout, Customer, Subscription, Request string
	Created                                             int64
	Digest                                              []byte
}

// VerifyCompletion authenticates complete/delayed-payment fixture notifications.
// It retains no email, metadata, payment details, navigation or entitlement.
func VerifyCompletion(body []byte, signature, secret string) (CompletionEvent, bool, error) {
	event, err := verifiedEnvelope(body, signature, secret)
	if err != nil {
		return CompletionEvent{}, false, err
	}
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed":
	default:
		return CompletionEvent{}, false, nil
	}
	var live struct {
		LiveMode *bool `json:"livemode"`
	}
	var session stripe.CheckoutSession
	if json.Unmarshal(event.Data.Raw, &live) != nil || live.LiveMode == nil || *live.LiveMode || json.Unmarshal(event.Data.Raw, &session) != nil || session.Object != "checkout.session" || session.Livemode || !checkoutID.MatchString(session.ID) || session.Customer == nil || session.Customer.Livemode || !customerID.MatchString(session.Customer.ID) || session.Subscription == nil || session.Subscription.Livemode || !subscriptionID.MatchString(session.Subscription.ID) || !uuid.MatchString(session.ClientReferenceID) || session.Mode != "subscription" || session.UIMode != "hosted_page" || session.Status != "complete" {
		return CompletionEvent{}, false, ErrInvalidEvent
	}
	digest := sha256.Sum256(body)
	return CompletionEvent{ID: event.ID, Type: string(event.Type), Checkout: session.ID, Customer: session.Customer.ID, Subscription: session.Subscription.ID, Request: session.ClientReferenceID, Created: event.Created, Digest: digest[:]}, true, nil
}

// CompletedSession is current session identity/payment state, without fulfillment authority.
// Callers must match retained scope and re-fetch the subscription before binding/observing.
type CompletedSession struct{ Checkout, Customer, Subscription, Request, PaymentState string }

// CompletedCheckout re-reads the exact stored session without creating resources or navigation.
// Complete and paid are different states; this reader grants no hosting entitlement.
func (a *FixtureSessions) CompletedCheckout(ctx context.Context, request SessionRequest, id string, expires time.Time) (CompletedSession, error) {
	if a == nil || a.client == nil || !sessionRequestValid(request) || !checkoutID.MatchString(id) || expires.IsZero() {
		return CompletedSession{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	params := &stripe.CheckoutSessionRetrieveParams{}
	params.AddExpand("line_items")
	session, err := a.client.V1CheckoutSessions.Retrieve(ctx, id, params)
	if err != nil || session == nil || session.ID != id || session.Livemode || session.Object != "checkout.session" || session.Customer == nil || session.Customer.Livemode || session.Customer.ID != request.CustomerID || session.Subscription == nil || session.Subscription.Livemode || !subscriptionID.MatchString(session.Subscription.ID) || session.ClientReferenceID != request.Key || session.Mode != "subscription" || session.UIMode != "hosted_page" || session.Status != "complete" || session.SuccessURL != a.settings.ReturnOrigin+"/billing/return" || session.CancelURL != a.settings.ReturnOrigin+"/billing/cancel" || session.ExpiresAt != expires.Unix() || session.LineItems == nil || session.LineItems.HasMore || len(session.LineItems.Data) != 1 {
		return CompletedSession{}, ErrUnavailable
	}
	item := session.LineItems.Data[0]
	if item == nil || item.Price == nil || item.Price.Livemode || item.Price.ID != a.settings.PriceID || item.Quantity != 1 {
		return CompletedSession{}, ErrUnavailable
	}
	switch session.PaymentStatus {
	case "paid", "unpaid", "no_payment_required":
	default:
		return CompletedSession{}, ErrUnavailable
	}
	return CompletedSession{Checkout: session.ID, Customer: session.Customer.ID, Subscription: session.Subscription.ID, Request: session.ClientReferenceID, PaymentState: string(session.PaymentStatus)}, nil
}
