package billing

import (
	"context"
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
	if err != nil || object == nil || object.Object != "subscription" || object.Livemode || object.ID != subscription || object.Customer == nil || object.Customer.Livemode || object.Customer.Deleted || object.Customer.ID != request.CustomerID || !subscriptionState(string(object.Status)) || object.Items == nil || object.Items.HasMore || len(object.Items.Data) != 1 {
		return CompletionGraph{}, ErrUnavailable
	}
	item := object.Items.Data[0]
	if item == nil || item.Object != "subscription_item" || item.Subscription != subscription || item.Price == nil || item.Price.Object != "price" || item.Price.Livemode || item.Price.ID != a.settings.PriceID || item.Quantity != 1 || item.CurrentPeriodEnd <= 0 {
		return CompletionGraph{}, ErrUnavailable
	}
	return CompletionGraph{Session: session, Subscription: Snapshot{Customer: object.Customer.ID, Subscription: object.ID, State: string(object.Status), Price: item.Price.ID, Quantity: 1, PeriodEnd: time.Unix(item.CurrentPeriodEnd, 0)}}, nil
}
