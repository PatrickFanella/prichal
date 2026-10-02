// Package billing qualifies signed event delivery and authoritative reconciliation locally.
// Its observations never grant paid hosting entitlement or alter native PDS resources.
package billing

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

var eventID = regexp.MustCompile(`^evt_[A-Za-z0-9]{1,100}$`)
var customerID = regexp.MustCompile(`^cus_[A-Za-z0-9]{1,100}$`)
var subscriptionID = regexp.MustCompile(`^sub_[A-Za-z0-9]{1,100}$`)
var priceID = regexp.MustCompile(`^price_[A-Za-z0-9]{1,100}$`)
var ErrInvalidEvent = errors.New("billing event rejected")
var ErrConflict = errors.New("event identity conflicts with retained payload")
var ErrUnmapped = errors.New("billing identity is not mapped")
var ErrUnavailable = errors.New("billing reconciliation unavailable")

// Event retains identifiers and a digest, not raw invoice/customer/payment content.
type Event struct {
	ID, Type, Customer, Subscription string
	Created                          int64
	Digest                           []byte
}

// Verify authenticates the unmodified raw body before inspecting any identifiers.
// Unsupported and non-subscription invoice events are ignored without side effects.
// verifiedEnvelope authenticates raw delivery and its fixed fixture account/API scope.
func verifiedEnvelope(body []byte, signature, secret string) (stripe.Event, error) {
	if len(body) == 0 || len(body) > 262144 || len(signature) > 4096 || !strings.HasPrefix(secret, "whsec_") || len(secret) < 38 {
		return stripe.Event{}, ErrInvalidEvent
	}
	parsed, err := webhook.ConstructEvent(body, signature, secret)
	if err != nil {
		return stripe.Event{}, ErrInvalidEvent
	}
	var mode struct {
		LiveMode *bool `json:"livemode"`
	}
	if json.Unmarshal(body, &mode) != nil || mode.LiveMode == nil || *mode.LiveMode {
		return stripe.Event{}, ErrInvalidEvent
	}
	// The SDK rejects old timestamps; explicitly reject future timestamps as well.
	timestamps := 0
	for _, part := range strings.Split(signature, ",") {
		if strings.HasPrefix(part, "t=") {
			timestamps++
			seconds, parseErr := strconv.ParseInt(strings.TrimPrefix(part, "t="), 10, 64)
			if parseErr != nil || time.Unix(seconds, 0).After(time.Now().Add(5*time.Minute)) {
				return stripe.Event{}, ErrInvalidEvent
			}
		}
	}
	if timestamps != 1 {
		return stripe.Event{}, ErrInvalidEvent
	}
	if parsed.Livemode || parsed.Account != "" || parsed.Context != "" || parsed.APIVersion != stripe.APIVersion || !eventID.MatchString(parsed.ID) || parsed.Created <= 0 || parsed.Created > time.Now().Add(5*time.Minute).Unix() || parsed.Data == nil {
		return stripe.Event{}, ErrInvalidEvent
	}
	return parsed, nil
}

func Verify(body []byte, signature, secret string) (Event, bool, error) {
	parsed, err := verifiedEnvelope(body, signature, secret)
	if err != nil {
		return Event{}, false, err
	}

	var customer, subscription string
	switch parsed.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.paused", "customer.subscription.resumed":
		var object stripe.Subscription
		if json.Unmarshal(parsed.Data.Raw, &object) != nil || object.Livemode || object.Customer == nil {
			return Event{}, false, ErrInvalidEvent
		}
		customer, subscription = object.Customer.ID, object.ID
	case "invoice.paid", "invoice.payment_failed", "invoice.payment_action_required":
		var object stripe.Invoice
		if json.Unmarshal(parsed.Data.Raw, &object) != nil || object.Livemode || object.Customer == nil {
			return Event{}, false, ErrInvalidEvent
		}
		if object.Parent == nil || object.Parent.Type != stripe.InvoiceParentTypeSubscriptionDetails || object.Parent.SubscriptionDetails == nil || object.Parent.SubscriptionDetails.Subscription == nil {
			return Event{}, false, nil
		}
		customer, subscription = object.Customer.ID, object.Parent.SubscriptionDetails.Subscription.ID
	default:
		return Event{}, false, nil
	}
	if !customerID.MatchString(customer) || !subscriptionID.MatchString(subscription) {
		return Event{}, false, ErrInvalidEvent
	}
	digest := sha256.Sum256(body)
	return Event{ID: parsed.ID, Type: string(parsed.Type), Customer: customer, Subscription: subscription, Created: parsed.Created, Digest: digest[:]}, true, nil
}

func supportedType(kind string) bool {
	switch kind {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.paused", "customer.subscription.resumed", "invoice.paid", "invoice.payment_failed", "invoice.payment_action_required":
		return true
	}
	return false
}
