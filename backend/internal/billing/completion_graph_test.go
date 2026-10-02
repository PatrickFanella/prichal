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

func completionSubscriptionObject(expires time.Time) map[string]any {
	return map[string]any{"id": "sub_completionFixture", "object": "subscription", "livemode": false, "customer": sessionFixtureRequest.CustomerID, "status": "active", "metadata": map[string]string{"private_marker": "owner-from-email@test"}, "items": map[string]any{"object": "list", "has_more": false, "data": []any{map[string]any{"object": "subscription_item", "subscription": "sub_completionFixture", "quantity": 1, "current_period_end": expires.Unix(), "price": map[string]any{"id": "price_fixtureSessions", "livemode": false, "object": "price"}}}}}
}
func TestCompletedGraphSameClientExactCustomerSubscriptionAndPrice(t *testing.T) {
	for _, state := range []string{"incomplete", "incomplete_expired", "trialing", "active", "past_due", "canceled", "unpaid", "paused"} {
		t.Run(state, func(t *testing.T) {
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			var origin string
			var calls atomic.Int32
			a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if r.Method != "GET" || r.Header.Get("Stripe-Version") != stripe.APIVersion || r.Header.Get("Idempotency-Key") != "" {
					t.Error("graph changed SDK read scope")
				}
				var object map[string]any
				if call == 1 {
					if r.URL.Path != "/v1/checkout/sessions/cs_test_fixtureSessions" || r.URL.Query().Get("expand[0]") != "line_items" {
						t.Error("graph read foreign session")
					}
					object = recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
					object["status"], object["subscription"], object["payment_status"] = "complete", "sub_completionFixture", "unpaid"
				} else {
					if call != 2 || r.URL.Path != "/v1/subscriptions/sub_completionFixture" || len(r.URL.Query()) != 0 {
						t.Error("graph read foreign subscription")
					}
					object = completionSubscriptionObject(expires)
					object["status"] = state
				}
				_ = json.NewEncoder(w).Encode(object)
			})
			origin = server.URL
			graph, err := a.CompletedGraph(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", "sub_completionFixture", expires)
			if err != nil || calls.Load() != 2 || graph.Subscription.Customer != sessionFixtureRequest.CustomerID || graph.Subscription.Subscription != "sub_completionFixture" || graph.Subscription.Price != "price_fixtureSessions" || graph.Subscription.State != state || graph.Subscription.Quantity != 1 || graph.Subscription.PeriodEnd.Unix() != expires.Unix() || graph.Session.PaymentState != "unpaid" {
				t.Fatal("graph lost exact current evidence")
			}
			raw, _ := json.Marshal(graph)
			if strings.Contains(string(raw), "private_marker") || strings.Contains(string(raw), "owner-from-email") || strings.Contains(string(raw), "/checkout/") {
				t.Fatal("graph retained metadata/navigation")
			}
		})
	}
}
func TestCompletedGraphRejectsUnconfirmedCurrentSubscription(t *testing.T) {
	for _, mode := range []string{"claim", "id", "object", "live", "customer", "customer_nil", "customer_live", "customer_deleted", "state", "items_nil", "items_more", "items_many", "item_nil", "item_object", "item_subscription", "price_object", "price_nil", "price", "price_live", "quantity", "period", "redirect", "error", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			var origin string
			var calls atomic.Int32
			a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					object := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
					object["status"], object["subscription"], object["payment_status"] = "complete", "sub_completionFixture", "paid"
					_ = json.NewEncoder(w).Encode(object)
					return
				}
				object := completionSubscriptionObject(expires)
				item := object["items"].(map[string]any)["data"].([]any)[0].(map[string]any)
				switch mode {
				case "id":
					object["id"] = "sub_other"
				case "object":
					object["object"] = "foreign"
				case "live":
					object["livemode"] = true
				case "customer":
					object["customer"] = "cus_other"
				case "customer_nil":
					object["customer"] = nil
				case "customer_live":
					object["customer"] = map[string]any{"id": sessionFixtureRequest.CustomerID, "livemode": true}
				case "customer_deleted":
					object["customer"] = map[string]any{"id": sessionFixtureRequest.CustomerID, "deleted": true}
				case "state":
					object["status"] = "unknown"
				case "items_nil":
					object["items"] = nil
				case "items_more":
					object["items"].(map[string]any)["has_more"] = true
				case "items_many":
					object["items"].(map[string]any)["data"] = []any{item, item}
				case "item_nil":
					object["items"].(map[string]any)["data"] = []any{nil}
				case "item_object":
					item["object"] = "foreign"
				case "item_subscription":
					item["subscription"] = "sub_other"
				case "price_object":
					item["price"].(map[string]any)["object"] = "foreign"
				case "price_nil":
					item["price"] = nil
				case "price":
					item["price"].(map[string]any)["id"] = "price_other"
				case "price_live":
					item["price"].(map[string]any)["livemode"] = true
				case "quantity":
					item["quantity"] = 2
				case "period":
					item["current_period_end"] = 0
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
				_ = json.NewEncoder(w).Encode(object)
			})
			origin = server.URL
			claim := "sub_completionFixture"
			want := int32(2)
			if mode == "claim" {
				claim = "sub_other"
				want = 1
			}
			graph, err := a.CompletedGraph(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", claim, expires)
			if !errors.Is(err, ErrUnavailable) || graph != (CompletionGraph{}) || calls.Load() != want {
				t.Fatal("unconfirmed graph returned authority")
			}
		})
	}
}
func TestCompletedGraphInvalidInputAndCancellationMakeNoRequest(t *testing.T) {
	var calls atomic.Int32
	a, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		a            *FixtureSessions
		ctx          context.Context
		request      SessionRequest
		subscription string
	}{{nil, context.Background(), sessionFixtureRequest, "sub_completionFixture"}, {a, context.Background(), sessionFixtureRequest, "bad/path"}, {a, context.Background(), SessionRequest{}, "sub_completionFixture"}, {a, ctx, sessionFixtureRequest, "sub_completionFixture"}} {
		graph, err := tc.a.CompletedGraph(tc.ctx, tc.request, "cs_test_fixtureSessions", tc.subscription, time.Now())
		if err == nil || graph != (CompletionGraph{}) {
			t.Fatal("invalid graph input accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid/cancelled graph made request")
	}
}
