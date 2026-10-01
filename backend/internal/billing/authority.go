package billing

import (
	"context"
	"net/http"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

type quietLogger struct{}

func (quietLogger) Debugf(string, ...interface{}) {}
func (quietLogger) Errorf(string, ...interface{}) {}
func (quietLogger) Infof(string, ...interface{})  {}
func (quietLogger) Warnf(string, ...interface{})  {}

// StripeAuthority uses an instance client and a separately protected local fixture key.
// It is not a qualified Stripe sandbox/live connection.
type StripeAuthority struct{ Client *stripe.Client }

func NewAuthority(settings Settings) (Authority, error) {
	if !settings.Enabled {
		return nil, nil
	}
	if !fixtureOrigin(settings.FixtureURL) {
		return nil, ErrUnavailable
	}
	key, err := privateKey(settings.FixtureKeyFile, "rk_test_")
	if err != nil {
		return nil, err
	}
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(settings.FixtureURL), HTTPClient: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, MaxNetworkRetries: stripe.Int64(0), LeveledLogger: quietLogger{}})
	return &StripeAuthority{Client: stripe.NewClient(key, stripe.WithBackends(&stripe.Backends{API: backend}))}, nil
}
func (a *StripeAuthority) Subscription(ctx context.Context, id string) (Snapshot, error) {
	if !subscriptionID.MatchString(id) {
		return Snapshot{}, ErrInvalidEvent
	}
	object, err := a.Client.V1Subscriptions.Retrieve(ctx, id, nil)
	if err != nil || object == nil || object.Livemode || object.ID != id || object.Customer == nil || object.Items == nil || object.Items.HasMore || len(object.Items.Data) != 1 {
		return Snapshot{}, ErrUnavailable
	}
	item := object.Items.Data[0]
	if item == nil || item.Price == nil || item.Price.Livemode || item.Quantity != 1 || item.CurrentPeriodEnd <= 0 {
		return Snapshot{}, ErrUnavailable
	}
	return Snapshot{Customer: object.Customer.ID, Subscription: object.ID, State: string(object.Status), Price: item.Price.ID, Quantity: 1, PeriodEnd: time.Unix(item.CurrentPeriodEnd, 0)}, nil
}
