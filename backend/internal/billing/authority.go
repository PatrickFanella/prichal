package billing

import (
	"context"
	"crypto/sha256"
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
type StripeAuthority struct {
	Client    *stripe.Client
	keyDigest [32]byte
	origin    string
}

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
	return &StripeAuthority{Client: stripe.NewClient(key, stripe.WithBackends(&stripe.Backends{API: backend})), keyDigest: sha256.Sum256([]byte(key)), origin: settings.FixtureURL}, nil
}
func (a *StripeAuthority) Subscription(ctx context.Context, id string) (Snapshot, error) {
	if a == nil || a.Client == nil {
		return Snapshot{}, ErrUnavailable
	}
	if !subscriptionID.MatchString(id) {
		return Snapshot{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	object, err := a.Client.V1Subscriptions.Retrieve(ctx, id, nil)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	return subscriptionSnapshot(object, id)
}
