package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

func completionFixtureObject() map[string]any {
	return map[string]any{"id": "cs_test_completionFixture", "object": "checkout.session", "livemode": false, "mode": "subscription", "ui_mode": "hosted_page", "status": "complete", "customer": sessionFixtureRequest.CustomerID, "subscription": "sub_completionFixture", "client_reference_id": sessionFixtureRequest.Key, "metadata": map[string]string{"owner_email": "private-marker@test", "organization_id": "foreign-metadata"}, "customer_details": map[string]any{"email": "private-marker@test"}, "url": "https://private-navigation-marker.test"}
}
func TestVerifyCompletionRetainsOnlySignedFirstClassClaims(t *testing.T) {
	secret := fixtureSecret(t, "whsec_")
	for _, kind := range []string{"checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		body := fixturePayload(t, "evt_completionFixture", kind, time.Now().Unix(), completionFixtureObject())
		event, supported, err := VerifyCompletion(body, signature(body, secret, time.Now()), secret)
		if err != nil || !supported || event.Checkout != "cs_test_completionFixture" || event.Customer != sessionFixtureRequest.CustomerID || event.Subscription != "sub_completionFixture" || event.Request != sessionFixtureRequest.Key || event.Type != kind || len(event.Digest) != 32 {
			t.Fatal("completion did not preserve first-class identity")
		}
		retained, err := json.Marshal(event)
		if err != nil || strings.Contains(string(retained), "private-marker") || strings.Contains(string(retained), "foreign-metadata") || strings.Contains(string(retained), "navigation-marker") {
			t.Fatal("completion projection retained private content")
		}
		if _, supported, err := Verify(body, signature(body, secret, time.Now()), secret); err != nil || supported {
			t.Fatal("completion enabled legacy inbox without explicit wiring")
		}
	}
	unsupported := fixturePayload(t, "evt_completionUnsupported", "checkout.session.expired", time.Now().Unix(), completionFixtureObject())
	if _, supported, err := VerifyCompletion(unsupported, signature(unsupported, secret, time.Now()), secret); err != nil || supported {
		t.Fatal("unsupported completion invented evidence")
	}
}
func TestVerifyCompletionRejectsMissingForeignLiveAndMalformedSession(t *testing.T) {
	secret := fixtureSecret(t, "whsec_")
	for _, mode := range []string{"object", "live", "missing_live", "id", "customer", "missing_customer", "live_customer", "subscription", "missing_subscription", "live_subscription", "reference", "metadata_reference", "mode", "ui", "status"} {
		t.Run(mode, func(t *testing.T) {
			object := completionFixtureObject()
			switch mode {
			case "object":
				object["object"] = "subscription"
			case "live":
				object["livemode"] = true
			case "missing_live":
				delete(object, "livemode")
			case "id":
				object["id"] = "cs_live_foreign"
			case "customer":
				object["customer"] = "email@test"
			case "missing_customer":
				object["customer"] = nil
			case "live_customer":
				object["customer"] = map[string]any{"id": sessionFixtureRequest.CustomerID, "livemode": true}
			case "subscription":
				object["subscription"] = "sub_bad/path"
			case "missing_subscription":
				object["subscription"] = nil
			case "live_subscription":
				object["subscription"] = map[string]any{"id": "sub_completionFixture", "livemode": true}
			case "reference":
				object["client_reference_id"] = "foreign"
			case "metadata_reference":
				delete(object, "client_reference_id")
				object["metadata"] = map[string]string{"client_reference_id": sessionFixtureRequest.Key}
			case "mode":
				object["mode"] = "payment"
			case "ui":
				object["ui_mode"] = "embedded_page"
			case "status":
				object["status"] = "open"
			}
			body := fixturePayload(t, "evt_completionInvalid", "checkout.session.completed", time.Now().Unix(), object)
			event, supported, err := VerifyCompletion(body, signature(body, secret, time.Now()), secret)
			if !errors.Is(err, ErrInvalidEvent) || supported || event.ID != "" || len(event.Digest) != 0 {
				t.Fatal("invalid completion retained evidence")
			}
		})
	}
}
func TestCompletionUsesSharedSignatureAndEnvelopeBoundary(t *testing.T) {
	secret := fixtureSecret(t, "whsec_")
	body := fixturePayload(t, "evt_completionSignature", "checkout.session.completed", time.Now().Unix(), completionFixtureObject())
	for _, header := range []string{"", signature(body, fixtureSecret(t, "whsec_"), time.Now()), signature(body, secret, time.Now().Add(-10*time.Minute)), signature(body, secret, time.Now().Add(10*time.Minute))} {
		if _, ok, err := VerifyCompletion(body, header, secret); !errors.Is(err, ErrInvalidEvent) || ok {
			t.Fatal("unauthenticated completion accepted")
		}
	}
	for _, mode := range []string{"live", "missing_live", "account", "context", "version", "future", "invalid_id", "missing_data"} {
		t.Run(mode, func(t *testing.T) {
			var envelope map[string]any
			if json.Unmarshal(body, &envelope) != nil {
				t.Fatal("invalid fixture envelope")
			}
			switch mode {
			case "live":
				envelope["livemode"] = true
			case "missing_live":
				delete(envelope, "livemode")
			case "account":
				envelope["account"] = "acct_other"
			case "context":
				envelope["context"] = "other"
			case "version":
				envelope["api_version"] = "2020-01-01"
			case "future":
				envelope["created"] = time.Now().Add(time.Hour).Unix()
			case "invalid_id":
				envelope["id"] = "foreign"
			case "missing_data":
				envelope["data"] = nil
			}
			raw, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok, err := VerifyCompletion(raw, signature(raw, secret, time.Now()), secret); !errors.Is(err, ErrInvalidEvent) || ok {
				t.Fatal("foreign completion envelope accepted")
			}
		})
	}
}

func TestCompletedCheckoutReadsCurrentIdentityWithoutNavigationOrEntitlement(t *testing.T) {
	for _, payment := range []string{"paid", "unpaid", "no_payment_required"} {
		t.Run(payment, func(t *testing.T) {
			var origin string
			expires := time.Now().Add(-time.Hour).Truncate(time.Second)
			var calls atomic.Int32
			adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/v1/checkout/sessions/cs_test_fixtureSessions" || r.URL.Query().Get("expand[0]") != "line_items" || len(r.URL.Query()) != 1 || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("Stripe-Version") != stripe.APIVersion {
					t.Error("completion escaped exact read-only SDK request")
				}
				body := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
				body["status"], body["subscription"], body["payment_status"], body["url"] = "complete", "sub_completionFixture", payment, "https://private-navigation-marker.test"
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error("fixture encoding failed")
				}
			})
			origin = server.URL
			got, err := adapter.CompletedCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", expires)
			if err != nil || got.Subscription != "sub_completionFixture" || got.Request != sessionFixtureRequest.Key || got.PaymentState != payment || calls.Load() != 1 {
				t.Fatal("completion identity was not re-read")
			}
			raw, err := json.Marshal(got)
			if err != nil || strings.Contains(string(raw), "navigation-marker") {
				t.Fatal("completion disclosed navigation")
			}
		})
	}
}

func TestCompletedCheckoutRefusesUnconfirmedProviderIdentity(t *testing.T) {
	for _, mode := range []string{"id", "live", "object", "customer", "customer_nil", "subscription", "subscription_nil", "subscription_live", "reference", "mode", "ui", "status", "success", "cancel", "expiry", "items_nil", "items_more", "item_nil", "price", "price_live", "quantity", "payment", "redirect", "error", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			var origin string
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			var calls atomic.Int32
			adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
				body["status"], body["subscription"], body["payment_status"] = "complete", "sub_completionFixture", "paid"
				switch mode {
				case "id":
					body["id"] = "cs_test_other"
				case "live":
					body["livemode"] = true
				case "object":
					body["object"] = "foreign"
				case "customer":
					body["customer"] = "cus_other"
				case "customer_nil":
					body["customer"] = nil
				case "subscription":
					body["subscription"] = "sub_other/path"
				case "subscription_nil":
					body["subscription"] = nil
				case "subscription_live":
					body["subscription"] = map[string]any{"id": "sub_completionFixture", "livemode": true}
				case "reference":
					body["client_reference_id"] = "foreign"
				case "mode":
					body["mode"] = "payment"
				case "ui":
					body["ui_mode"] = "embedded_page"
				case "status":
					body["status"] = "open"
				case "success":
					body["success_url"] = "https://foreign.test"
				case "cancel":
					body["cancel_url"] = "https://foreign.test"
				case "expiry":
					body["expires_at"] = expires.Add(time.Minute).Unix()
				case "items_nil":
					body["line_items"] = nil
				case "items_more":
					body["line_items"].(map[string]any)["has_more"] = true
				case "item_nil":
					body["line_items"].(map[string]any)["data"] = []any{nil}
				case "price":
					body["line_items"].(map[string]any)["data"].([]any)[0].(map[string]any)["price"] = nil
				case "price_live":
					body["line_items"].(map[string]any)["data"].([]any)[0].(map[string]any)["price"].(map[string]any)["livemode"] = true
				case "quantity":
					body["line_items"].(map[string]any)["data"].([]any)[0].(map[string]any)["quantity"] = 2
				case "payment":
					body["payment_status"] = "unknown"
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1:1/private", 307)
					return
				case "error":
					w.WriteHeader(500)
					return
				case "malformed":
					_, _ = w.Write([]byte("{invalid"))
					return
				}
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error("fixture encoding failed")
				}
			})
			origin = server.URL
			got, err := adapter.CompletedCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", expires)
			if !errors.Is(err, ErrUnavailable) || got != (CompletedSession{}) || calls.Load() != 1 {
				t.Fatal("unconfirmed provider response returned completion")
			}
		})
	}
}
func TestCompletedCheckoutInputAndCancellationBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, input := range []struct {
		request SessionRequest
		id      string
		expires time.Time
	}{{SessionRequest{}, "cs_test_fixtureSessions", time.Now()}, {sessionFixtureRequest, "cs_test_x/path", time.Now()}, {sessionFixtureRequest, "cs_test_fixtureSessions", time.Time{}}} {
		if _, err := adapter.CompletedCheckout(context.Background(), input.request, input.id, input.expires); !errors.Is(err, ErrInvalidEvent) {
			t.Fatal("invalid completion input accepted")
		}
	}
	var missing *FixtureSessions
	if _, err := missing.CompletedCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", time.Now()); !errors.Is(err, ErrInvalidEvent) {
		t.Fatal("missing adapter accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.CompletedCheckout(ctx, sessionFixtureRequest, "cs_test_fixtureSessions", time.Now()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled completion returned evidence")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid/cancelled completion called provider")
	}
}
