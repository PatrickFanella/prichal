package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

var sessionFixtureRequest = SessionRequest{Key: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", CustomerID: "cus_fixtureSessions"}

func fixtureSessionAdapter(t *testing.T, handler http.HandlerFunc) (*FixtureSessions, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(fixtureSecret(t, "rk_test_")), 0600); err != nil {
		t.Fatal("fixture key creation failed")
	}
	settings := SessionSettings{Billing: Settings{Enabled: true, FixtureURL: server.URL, FixtureKeyFile: path}, PriceID: "price_fixtureSessions", PortalConfigurationID: "bpc_fixtureSessions", ReturnOrigin: "http://127.0.0.1:5173"}
	adapter, err := NewFixtureSessions(settings)
	if err != nil {
		t.Fatal("session fixture setup failed")
	}
	return adapter, server
}

func sessionResponse(origin, kind string) map[string]any {
	if kind == "portal" {
		return map[string]any{"object": "billing_portal.session", "id": "bps_fixtureSessions", "customer": sessionFixtureRequest.CustomerID, "configuration": "bpc_fixtureSessions", "livemode": false, "created": time.Now().Unix(), "return_url": "http://127.0.0.1:5173/billing/return", "url": origin + "/portal/bps_fixtureSessions"}
	}
	return map[string]any{"object": "checkout.session", "id": "cs_test_fixtureSessions", "customer": sessionFixtureRequest.CustomerID, "client_reference_id": sessionFixtureRequest.Key, "livemode": false, "mode": "subscription", "ui_mode": "hosted_page", "status": "open", "expires_at": time.Now().Add(time.Hour).Unix(), "success_url": "http://127.0.0.1:5173/billing/return", "cancel_url": "http://127.0.0.1:5173/billing/cancel", "url": origin + "/checkout/cs_test_fixtureSessions"}
}

func TestFixtureSessionsUseExplicitStableSDKParameters(t *testing.T) {
	var mu sync.Mutex
	var forms []url.Values
	var keys, paths []string
	var origin string
	a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Stripe-Version") != stripe.APIVersion || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer rk_test_") {
			t.Error("incorrect fixture SDK transport")
		}
		if err := r.ParseForm(); err != nil {
			t.Error("form parsing failed")
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		forms = append(forms, r.PostForm)
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		kind := "checkout"
		if r.URL.Path == "/v1/billing_portal/sessions" {
			kind = "portal"
		}
		if err := json.NewEncoder(w).Encode(sessionResponse(origin, kind)); err != nil {
			t.Error("fixture encoding failed")
		}
	})
	origin = server.URL
	for range 2 {
		s, err := a.Checkout(context.Background(), sessionFixtureRequest)
		if err != nil || s.ID != "cs_test_fixtureSessions" || s.ExpiresAt.IsZero() {
			t.Fatal("checkout session was not verified")
		}
	}
	for range 2 {
		s, err := a.Portal(context.Background(), sessionFixtureRequest)
		if err != nil || s.ID != "bps_fixtureSessions" || !s.ExpiresAt.IsZero() {
			t.Fatal("portal session was not verified")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(forms) != 4 || forms[0].Encode() != forms[1].Encode() || forms[2].Encode() != forms[3].Encode() || keys[0] != keys[1] || keys[2] != keys[3] || keys[0] == keys[2] {
		t.Fatal("retry request or namespace changed")
	}
	expected := url.Values{"customer": {sessionFixtureRequest.CustomerID}, "client_reference_id": {sessionFixtureRequest.Key}, "mode": {"subscription"}, "ui_mode": {"hosted_page"}, "line_items[0][price]": {"price_fixtureSessions"}, "line_items[0][quantity]": {"1"}, "success_url": {"http://127.0.0.1:5173/billing/return"}, "cancel_url": {"http://127.0.0.1:5173/billing/cancel"}, "integration_identifier": {integrationLabel(sessionFixtureRequest.Key)}}
	if forms[0].Encode() != expected.Encode() || keys[0] != "pdsaas-checkout-"+sessionFixtureRequest.Key || paths[0] != "/v1/checkout/sessions" {
		t.Fatal("checkout escaped explicit parameters")
	}
	expected = url.Values{"customer": {sessionFixtureRequest.CustomerID}, "configuration": {"bpc_fixtureSessions"}, "return_url": {"http://127.0.0.1:5173/billing/return"}}
	if forms[2].Encode() != expected.Encode() || keys[2] != "pdsaas-portal-"+sessionFixtureRequest.Key || paths[2] != "/v1/billing_portal/sessions" {
		t.Fatal("portal escaped explicit parameters")
	}
	suffix := strings.TrimPrefix(integrationLabel(sessionFixtureRequest.Key), "pdsaas_fixture_")
	if len(suffix) != 8 || strings.Trim(suffix, "abcdefghijklmnopqrstuvwxyz") != "" {
		t.Fatal("integration suffix is not eight letters")
	}
}

func TestFixtureSessionInputRefusalPrecedesNetwork(t *testing.T) {
	var calls atomic.Int32
	a, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, request := range []SessionRequest{{}, {Key: "invalid", CustomerID: sessionFixtureRequest.CustomerID}, {Key: sessionFixtureRequest.Key, CustomerID: "cus_x/injected"}, {Key: sessionFixtureRequest.Key, CustomerID: "email@example.test"}} {
		if _, err := a.Checkout(context.Background(), request); !errors.Is(err, ErrInvalidEvent) {
			t.Fatal("invalid checkout request accepted")
		}
		if _, err := a.Portal(context.Background(), request); !errors.Is(err, ErrInvalidEvent) {
			t.Fatal("invalid portal request accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached billing authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Checkout(ctx, sessionFixtureRequest); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled checkout returned navigation")
	}
	if calls.Load() != 0 {
		t.Fatal("cancelled input reached billing authority")
	}
}

func TestFixtureSessionResponsesFailClosed(t *testing.T) {
	for _, kind := range []string{"checkout", "portal"} {
		mutations := map[string]any{"livemode": true, "id": "foreign", "object": "foreign", "customer": "cus_foreign", "url": "https://attacker.test/private", "return_url": "https://attacker.test", "created": int64(1), "future_created": time.Now().Add(2 * time.Minute).Unix(), "configuration": "bpc_foreign", "missing_configuration": nil}
		if kind == "checkout" {
			mutations = map[string]any{"livemode": true, "id": "cs_live_foreign", "object": "foreign", "customer": "cus_foreign", "client_reference_id": "foreign", "mode": "payment", "ui_mode": "embedded_page", "status": "complete", "expires_at": int64(1), "missing_customer": nil, "too_long_expiry": time.Now().Add(26 * time.Hour).Unix(), "success_url": "https://attacker.test", "cancel_url": "https://attacker.test", "url": "https://attacker.test/private"}
		}
		for field, bad := range mutations {
			t.Run(kind+"/"+field, func(t *testing.T) {
				var origin string
				a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
					value := sessionResponse(origin, kind)
					if field == "missing_configuration" {
						value["configuration"] = nil
					} else if field == "future_created" {
						value["created"] = bad
					} else if field == "missing_customer" {
						value["customer"] = nil
					} else if field == "too_long_expiry" {
						value["expires_at"] = bad
					} else {
						value[field] = bad
					}
					if err := json.NewEncoder(w).Encode(value); err != nil {
						t.Error("fixture encoding failed")
					}
				})
				origin = server.URL
				var err error
				var s HostedSession
				if kind == "checkout" {
					s, err = a.Checkout(context.Background(), sessionFixtureRequest)
				} else {
					s, err = a.Portal(context.Background(), sessionFixtureRequest)
				}
				if !errors.Is(err, ErrUnavailable) || s != (HostedSession{}) {
					t.Fatal("invalid session disclosed a navigation capability")
				}
			})
		}
	}
}

func TestFixtureSessionURLsRejectRedirectAndUntrustedShapes(t *testing.T) {
	for _, kind := range []string{"checkout", "portal"} {
		for _, shape := range []string{"credentials", "query", "empty_query", "fragment", "encoded", "other_path", "redirect", "malformed", "provider_error"} {
			t.Run(kind+"/"+shape, func(t *testing.T) {
				var origin string
				a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
					if shape == "redirect" {
						http.Redirect(w, r, "http://127.0.0.1:1/private", 307)
						return
					}
					if shape == "provider_error" {
						w.WriteHeader(503)
						_, err := w.Write([]byte(`{"error":{"message":"private_provider_marker"}}`))
						if err != nil {
							t.Error("fixture write failed")
						}
						return
					}
					if shape == "malformed" {
						_, err := w.Write([]byte("{"))
						if err != nil {
							t.Error("fixture write failed")
						}
						return
					}
					v := sessionResponse(origin, kind)
					raw := v["url"].(string)
					switch shape {
					case "credentials":
						raw = strings.Replace(raw, "http://", "http://private:password@", 1)
					case "query":
						raw += "?private=marker"
					case "empty_query":
						raw += "?"
					case "fragment":
						raw += "#marker"
					case "encoded":
						raw = strings.Replace(raw, "/checkout/", "/checkout%2F", 1)
						if kind == "portal" {
							raw = strings.Replace(raw, "/portal/", "/portal%2F", 1)
						}
					case "other_path":
						raw = origin + "/unexpected"
					}
					v["url"] = raw
					if err := json.NewEncoder(w).Encode(v); err != nil {
						t.Error("fixture encoding failed")
					}
				})
				origin = server.URL
				var s HostedSession
				var err error
				if kind == "checkout" {
					s, err = a.Checkout(context.Background(), sessionFixtureRequest)
				} else {
					s, err = a.Portal(context.Background(), sessionFixtureRequest)
				}
				if err != ErrUnavailable || s != (HostedSession{}) {
					t.Fatal("unconfirmed provider response escaped generic failure")
				}
			})
		}
	}
}

func TestSessionAdapterRequiresCompleteLocalConfiguration(t *testing.T) {
	a, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	for _, change := range []func(*SessionSettings){
		func(s *SessionSettings) { s.Billing.Enabled = false }, func(s *SessionSettings) { s.Billing.FixtureURL = "https://api.stripe.com" },
		func(s *SessionSettings) { s.ReturnOrigin = "https://customer.test" }, func(s *SessionSettings) { s.ReturnOrigin = "http://127.0.0.1:5173/path" },
		func(s *SessionSettings) { s.ReturnOrigin = "http://private@127.0.0.1:5173" }, func(s *SessionSettings) { s.ReturnOrigin = "http://127.0.0.1:5173?" },
		func(s *SessionSettings) { s.Billing.FixtureURL += "?" },
		func(s *SessionSettings) { s.PriceID = "" },
		func(s *SessionSettings) { s.PortalConfigurationID = "" }, func(s *SessionSettings) { s.Billing.FixtureKeyFile = "missing" },
	} {
		s := a.settings
		change(&s)
		if value, err := NewFixtureSessions(s); err != ErrUnavailable || value != nil {
			t.Fatal("incomplete or external session configuration accepted")
		}
	}
}
