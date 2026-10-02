package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestCheckoutDispatchUsesPinnedSDKAndRetainedAuthorityIdentity(t *testing.T) {
	_, pool, org := fixtureStore(t)
	ctx := context.Background()
	var owner string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var keys []string
	adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error("fixture form invalid")
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		var body map[string]any
		switch r.URL.Path {
		case "/v1/customers":
			body = map[string]any{"object": "customer", "id": "cus_sdkDispatch", "livemode": false}
		case "/v1/checkout/sessions":
			if r.PostForm.Get("customer") != "cus_sdkDispatch" || r.PostForm.Get("line_items[0][price]") != "price_fixtureSessions" || r.PostForm.Get("line_items[0][quantity]") != "1" {
				t.Error("SDK checkout escaped mapped customer/catalog")
			}
			body = map[string]any{"object": "checkout.session", "id": "cs_test_sdkDispatch", "customer": "cus_sdkDispatch", "client_reference_id": r.PostForm.Get("client_reference_id"), "livemode": false, "mode": "subscription", "ui_mode": "hosted_page", "status": "open", "success_url": r.PostForm.Get("success_url"), "cancel_url": r.PostForm.Get("cancel_url"), "expires_at": time.Now().Add(time.Hour).Unix(), "url": "http://" + r.Host + "/checkout/cs_test_sdkDispatch"}
		default:
			t.Error("unexpected SDK dispatch path")
			w.WriteHeader(400)
			return
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error("fixture encoding failed")
		}
	})
	s, err := NewCheckoutIntents(true, pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, owner, org, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.Dispatch(ctx, p.ID, adapter); err != nil {
		t.Fatal("retained SDK dispatch failed")
	}
	var authority string
	var digest []byte
	if err = pool.QueryRow(ctx, `SELECT authority_origin,authority_key_sha256 FROM billing_checkout_requests WHERE id=$1`, p.ID).Scan(&authority, &digest); err != nil || authority != server.URL || string(digest) != string(adapter.keyDigest[:]) {
		t.Fatal("dispatch authority did not match actual client")
	}
	if err = s.Dispatch(ctx, p.ID, adapter); err != nil {
		t.Fatal("confirmed SDK receipt retry failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != "pdsaas-customer-"+p.ID || keys[1] != "pdsaas-checkout-"+p.ID {
		t.Fatal("SDK repeated confirmed work or changed dispatch namespace")
	}
}
