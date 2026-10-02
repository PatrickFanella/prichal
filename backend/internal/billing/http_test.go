package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

func postEvent(t *testing.T, server *httptest.Server, raw []byte, header, origin string, want int) string {
	t.Helper()
	req, _ := http.NewRequest("POST", server.URL+"/api/billing/webhook", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set("Stripe-Signature", header)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("fixture HTTP request failed")
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("billing status %d, want %d", response.StatusCode, want)
	}
	return string(data)
}
func TestWebhookAcknowledgesOnlyDurableVerifiedDelivery(t *testing.T) {
	store, pool, _ := fixtureStore(t)
	secret := fixtureSecret(t, "whsec_")
	mux := http.NewServeMux()
	store.Register(Settings{Enabled: true, WebhookSecret: secret})(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	raw := fixturePayload(t, "evt_fixtureHTTP", "customer.subscription.updated", time.Now().Unix(), fixtureSubscription())
	header := signature(raw, secret, time.Now())
	for _, tc := range []struct{ name, signature, origin string }{{"missing", "", ""}, {"wrong", signature(raw, fixtureSecret(t, "whsec_"), time.Now()), ""}, {"browser", header, "http://attacker.test"}} {
		t.Run(tc.name, func(t *testing.T) {
			body := postEvent(t, server, raw, tc.signature, tc.origin, 400)
			if strings.Contains(body, secret) || strings.Contains(body, "private_marker") {
				t.Fatal("private webhook detail leaked")
			}
		})
	}
	body := postEvent(t, server, raw, header, "", 200)
	var receipt map[string]bool
	if json.Unmarshal([]byte(body), &receipt) != nil || !receipt["accepted"] || receipt["duplicate"] {
		t.Fatal("valid notification not accepted")
	}
	body = postEvent(t, server, raw, header, "", 200)
	if json.Unmarshal([]byte(body), &receipt) != nil || !receipt["duplicate"] {
		t.Fatal("replayed notification not acknowledged")
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		t.Fatal("invalid fixture envelope")
	}
	envelope["pending_webhooks"] = 0
	changedDelivery, _ := json.Marshal(envelope)
	body = postEvent(t, server, changedDelivery, signature(changedDelivery, secret, time.Now()), "", 200)
	if json.Unmarshal([]byte(body), &receipt) != nil || !receipt["duplicate"] {
		t.Fatal("changed delivery metadata rejected stable event identity")
	}
	var jobs int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind='billing_reconcile'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatal("HTTP replay created duplicate work")
	}
	oversized := bytes.Repeat([]byte("x"), 262145)
	postEvent(t, server, oversized, signature(oversized, secret, time.Now()), "", 400)
	if _, err := pool.Exec(context.Background(), `ALTER TABLE river_job ADD CONSTRAINT reject_webhook CHECK(kind<>'billing_reconcile') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	raw = fixturePayload(t, "evt_fixtureHTTPRollback", "customer.subscription.updated", time.Now().Unix(), fixtureSubscription())
	postEvent(t, server, raw, signature(raw, secret, time.Now()), "", 503)
	var events int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM billing_events`).Scan(&events); err != nil || events != 1 {
		t.Fatal("HTTP acknowledged work without durable insert")
	}
	disabled := http.NewServeMux()
	(&Store{}).Register(Settings{})(disabled)
	closed := httptest.NewServer(disabled)
	defer closed.Close()
	postEvent(t, closed, raw, "", "", 503)
}
func TestAuthorityUsesInstanceClientAndRejectsRedirects(t *testing.T) {
	key := fixtureSecret(t, "rk_test_")
	file := filepath.Join(t.TempDir(), "fixture.key")
	if err := os.WriteFile(file, []byte(key), 0600); err != nil {
		t.Fatal(err)
	}
	redirect := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("Stripe-Version") != stripe.APIVersion || r.URL.Path != "/v1/subscriptions/sub_fixtureAlice" {
			t.Error("wrong fixture client transport")
		}
		if redirect {
			http.Redirect(w, r, "http://127.0.0.1:1/must-not-follow", 307)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "sub_fixtureAlice", "object": "subscription", "customer": "cus_fixtureAlice", "livemode": false, "status": "past_due", "items": map[string]any{"object": "list", "has_more": false, "data": []any{map[string]any{"id": "si_fixtureAlice", "object": "subscription_item", "subscription": "sub_fixtureAlice", "quantity": 1, "current_period_end": time.Now().Add(time.Hour).Unix(), "price": map[string]any{"id": "price_fixtureSingle", "object": "price", "livemode": false}}}}})
	}))
	defer server.Close()
	authority, err := NewAuthority(Settings{Enabled: true, FixtureURL: server.URL, FixtureKeyFile: file})
	if err != nil {
		t.Fatal("fixture authority setup failed")
	}
	result, err := authority.Subscription(context.Background(), "sub_fixtureAlice")
	if err != nil || result.State != "past_due" || result.Customer != "cus_fixtureAlice" {
		t.Fatal("current subscription fetch failed")
	}
	redirect = true
	if _, err = authority.Subscription(context.Background(), "sub_fixtureAlice"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("fixture redirect accepted")
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = NewAuthority(Settings{Enabled: true, FixtureURL: server.URL, FixtureKeyFile: file}); err == nil {
		t.Fatal("public API key file accepted")
	}
}

func TestSettingsKeepLiveBillingClosed(t *testing.T) {
	t.Setenv("PDS_BILLING_MODE", "fixture")
	t.Setenv("PDS_BILLING_FIXTURE_URL", "https://api.stripe.com")
	if _, err := LoadSettings(true); err == nil {
		t.Fatal("fixture reached external billing")
	}
	t.Setenv("PDS_BILLING_FIXTURE_URL", "http://127.0.0.1:5560")
	if _, err := LoadSettings(false); err == nil {
		t.Fatal("fixture enabled in production")
	}
	t.Setenv("PDS_BILLING_MODE", "stripe_live")
	if _, err := LoadSettings(true); err == nil {
		t.Fatal("live payment mode opened")
	}
	t.Setenv("PDS_BILLING_MODE", "disabled")
	settings, err := LoadSettings(false)
	if err != nil || settings.Enabled {
		t.Fatal("disabled billing required credentials")
	}
}
