package billing

import (
	"context"
	"crypto/sha256"
	"net/url"
	"regexp"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

// SessionSettings fixes the local catalog and return origin outside browser input.
type SessionSettings struct {
	Billing                                      Settings
	PriceID, PortalConfigurationID, ReturnOrigin string
}

// SessionRequest is transport input from a future durable, owner-authorized request.
// CustomerID must come from the retained customer mapping, never browser metadata.
type SessionRequest struct{ Key, CustomerID string }

// HostedSession contains an ephemeral navigation capability, not payment entitlement.
// Its URL must not be logged or retained in an audit.
type HostedSession struct {
	ID, URL   string
	ExpiresAt time.Time
}

// FixtureSessions is a loopback-only Checkout and Portal adapter. It exposes no HTTP route.
// Durable ownership, customer creation and fulfillment remain separate work.
type FixtureSessions struct {
	client    *stripe.Client
	settings  SessionSettings
	keyDigest [32]byte
}

var checkoutID = regexp.MustCompile(`^cs_test_[A-Za-z0-9]{1,100}$`)
var portalID = regexp.MustCompile(`^bps_[A-Za-z0-9]{1,100}$`)
var portalConfigurationID = regexp.MustCompile(`^bpc_[A-Za-z0-9]{1,100}$`)

// NewFixtureSessions refuses external billing and absent catalog configuration.
func NewFixtureSessions(settings SessionSettings) (*FixtureSessions, error) {
	if !settings.Billing.Enabled || !sessionOrigin(settings.Billing.FixtureURL) || !sessionOrigin(settings.ReturnOrigin) || !priceID.MatchString(settings.PriceID) || !portalConfigurationID.MatchString(settings.PortalConfigurationID) {
		return nil, ErrUnavailable
	}
	authority, err := NewAuthority(settings.Billing)
	if err != nil {
		return nil, ErrUnavailable
	}
	a, ok := authority.(*StripeAuthority)
	if !ok || a.Client == nil {
		return nil, ErrUnavailable
	}
	return &FixtureSessions{client: a.Client, settings: settings, keyDigest: a.keyDigest}, nil
}

func sessionOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && fixtureOrigin(raw) && !parsed.ForceQuery && raw == parsed.Scheme+"://"+parsed.Host
}

func sessionRequestValid(request SessionRequest) bool {
	return uuid.MatchString(request.Key) && customerID.MatchString(request.CustomerID)
}

// integrationLabel preserves the same request body on transport retry. The request
// key is random at creation; its digest supplies the eight-letter label suffix.
func integrationLabel(key string) string {
	digest := sha256.Sum256([]byte(key))
	suffix := make([]byte, 8)
	for i := range suffix {
		suffix[i] = 'a' + digest[i]%26
	}
	return "pdsaas_fixture_" + string(suffix)
}

func (a *FixtureSessions) navigation(raw, path string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.ForceQuery || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != path {
		return false
	}
	return parsed.Scheme+"://"+parsed.Host == a.settings.Billing.FixtureURL
}

// Checkout creates only a hosted subscription session for the configured fixture Price.
// Success-page navigation never grants entitlement or establishes customer ownership.
func (a *FixtureSessions) Checkout(ctx context.Context, request SessionRequest) (HostedSession, error) {
	if a == nil || a.client == nil || !sessionRequestValid(request) {
		return HostedSession{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	p := &stripe.CheckoutSessionCreateParams{
		Customer: stripe.String(request.CustomerID), ClientReferenceID: stripe.String(request.Key),
		Mode: stripe.String("subscription"), UIMode: stripe.String("hosted_page"),
		LineItems:             []*stripe.CheckoutSessionCreateLineItemParams{{Price: stripe.String(a.settings.PriceID), Quantity: stripe.Int64(1)}},
		SuccessURL:            stripe.String(a.settings.ReturnOrigin + "/billing/return"),
		CancelURL:             stripe.String(a.settings.ReturnOrigin + "/billing/cancel"),
		IntegrationIdentifier: stripe.String(integrationLabel(request.Key)),
	}
	p.SetIdempotencyKey("pdsaas-checkout-" + request.Key)
	session, err := a.client.V1CheckoutSessions.Create(ctx, p)
	now := time.Now()
	if err != nil || session == nil || session.Livemode || session.Object != "checkout.session" || !checkoutID.MatchString(session.ID) || session.Customer == nil || session.Customer.ID != request.CustomerID || session.ClientReferenceID != request.Key || session.Mode != "subscription" || session.UIMode != "hosted_page" || session.Status != "open" || session.SuccessURL != *p.SuccessURL || session.CancelURL != *p.CancelURL || session.ExpiresAt <= now.Unix() || session.ExpiresAt > now.Add(24*time.Hour+time.Minute).Unix() || !a.navigation(session.URL, "/checkout/"+session.ID) {
		return HostedSession{}, ErrUnavailable
	}
	return HostedSession{session.ID, session.URL, time.Unix(session.ExpiresAt, 0)}, nil
}

// Portal creates a short-lived session using an explicit fixture portal configuration.
// It cannot cancel hosting or change a retained subscription observation itself.
func (a *FixtureSessions) Portal(ctx context.Context, request SessionRequest) (HostedSession, error) {
	if a == nil || a.client == nil || !sessionRequestValid(request) {
		return HostedSession{}, ErrInvalidEvent
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	p := &stripe.BillingPortalSessionCreateParams{Customer: stripe.String(request.CustomerID), Configuration: stripe.String(a.settings.PortalConfigurationID), ReturnURL: stripe.String(a.settings.ReturnOrigin + "/billing/return")}
	p.SetIdempotencyKey("pdsaas-portal-" + request.Key)
	session, err := a.client.V1BillingPortalSessions.Create(ctx, p)
	now := time.Now()
	if err != nil || session == nil || session.Livemode || session.Object != "billing_portal.session" || !portalID.MatchString(session.ID) || session.Customer != request.CustomerID || session.Configuration == nil || session.Configuration.ID != a.settings.PortalConfigurationID || session.ReturnURL != *p.ReturnURL || session.Created <= 0 || session.Created > now.Add(time.Minute).Unix() || session.Created < now.Add(-5*time.Minute).Unix() || !a.navigation(session.URL, "/portal/"+session.ID) {
		return HostedSession{}, ErrUnavailable
	}
	return HostedSession{ID: session.ID, URL: session.URL}, nil
}
