package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	stripe "github.com/stripe/stripe-go/v87"
)

func TestCustomerFixtureUsesOriginalKeyWithoutIdentityMetadata(t *testing.T) {
	var mu sync.Mutex
	var bodies, keys []string
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error("invalid customer request")
			w.WriteHeader(400)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/v1/customers" || r.Header.Get("Stripe-Version") != stripe.APIVersion {
			t.Error("customer transport escaped SDK contract")
		}
		mu.Lock()
		bodies = append(bodies, r.PostForm.Encode())
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		if err := json.NewEncoder(w).Encode(map[string]any{"object": "customer", "id": "cus_createdFixture", "livemode": false}); err != nil {
			t.Error("fixture encoding failed")
		}
	})
	for range 2 {
		id, err := adapter.CreateCustomer(context.Background(), sessionFixtureRequest.Key)
		if err != nil || id != "cus_createdFixture" {
			t.Fatal("customer ID was not verified")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 || bodies[0] != "description=Managed+PDS+hosting+fixture" || bodies[0] != bodies[1] || keys[0] != "pdsaas-customer-"+sessionFixtureRequest.Key || keys[0] != keys[1] {
		t.Fatal("customer retry parameters changed or included private identity")
	}
}

func TestCustomerFixtureRefusesUnconfirmedResponsesAndInput(t *testing.T) {
	for field, value := range map[string]any{"livemode": true, "deleted": true, "object": "subscription", "id": "invalid"} {
		t.Run(field, func(t *testing.T) {
			adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{"object": "customer", "id": "cus_createdFixture", "livemode": false}
				body[field] = value
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error("fixture encoding failed")
				}
			})
			if id, err := adapter.CreateCustomer(context.Background(), sessionFixtureRequest.Key); err != ErrUnavailable || id != "" {
				t.Fatal("unconfirmed customer identity escaped")
			}
		})
	}
	var calls atomic.Int32
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	if id, err := adapter.CreateCustomer(context.Background(), "invalid"); err != ErrInvalidEvent || id != "" || calls.Load() != 0 {
		t.Fatal("invalid customer key reached authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if id, err := adapter.CreateCustomer(ctx, sessionFixtureRequest.Key); err != ErrUnavailable || id != "" || calls.Load() != 0 {
		t.Fatal("cancelled customer request reached authority")
	}
	if id, err := adapter.CreateCustomer(context.Background(), sessionFixtureRequest.Key); err != ErrUnavailable || id != "" || calls.Load() != 1 {
		t.Fatal("unknown customer result retried or disclosed identity")
	}
}
