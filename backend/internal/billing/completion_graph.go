package billing

import (
	"context"
	stripe "github.com/stripe/stripe-go/v87"
	"time"
)

// CompletionGraph contains current first-class session/subscription evidence only.
// It is not an ownership binding, paid plan decision or hosting allowance.
type CompletionGraph struct {
	Session      CompletedSession
	Subscription Snapshot
}

func subscriptionState(state string) bool {
	switch state {
	case "incomplete", "incomplete_expired", "trialing", "active", "past_due", "canceled", "unpaid", "paused":
		return true
	default:
		return false
	}
}

// CompletedGraph verifies both reads through the same configured client and credential.
// The expected subscription is a signed claim that must match current session identity.
func (a *FixtureSessions) CompletedGraph(ctx context.Context, request SessionRequest, id, subscription string, expires time.Time) (CompletionGraph, error) {
	if a == nil || a.client == nil || !subscriptionID.MatchString(subscription) {
		return CompletionGraph{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	session, err := a.CompletedCheckout(ctx, request, id, expires)
	if err != nil {
		return CompletionGraph{}, err
	}
	if session.Subscription != subscription {
		return CompletionGraph{}, ErrUnavailable
	}
	object, err := a.client.V1Subscriptions.Retrieve(ctx, subscription, nil)
	if err != nil {
		return CompletionGraph{}, ErrUnavailable
	}
	snapshot, err := subscriptionSnapshot(object, subscription)
	if err != nil || snapshot.Customer != request.CustomerID || snapshot.Price != a.settings.PriceID {
		return CompletionGraph{}, ErrUnavailable
	}
	return CompletionGraph{Session: session, Subscription: snapshot}, nil
}

// subscriptionSnapshot validates the first-class current subscription/item graph for both inboxes.
func subscriptionSnapshot(object *stripe.Subscription, id string) (Snapshot, error) {
	if object == nil || object.Object != "subscription" || object.Livemode || object.ID != id || object.Customer == nil || object.Customer.Livemode || object.Customer.Deleted || !customerID.MatchString(object.Customer.ID) || !subscriptionState(string(object.Status)) || object.Items == nil || object.Items.HasMore || len(object.Items.Data) != 1 {
		return Snapshot{}, ErrUnavailable
	}
	item := object.Items.Data[0]
	if item == nil || item.Object != "subscription_item" || item.Subscription != id || item.Price == nil || item.Price.Object != "price" || item.Price.Livemode || !priceID.MatchString(item.Price.ID) || item.Quantity != 1 || item.CurrentPeriodEnd <= 0 {
		return Snapshot{}, ErrUnavailable
	}
	return Snapshot{Customer: object.Customer.ID, Subscription: object.ID, State: string(object.Status), Price: item.Price.ID, Quantity: 1, PeriodEnd: time.Unix(item.CurrentPeriodEnd, 0)}, nil
}
