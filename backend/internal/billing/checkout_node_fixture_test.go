package billing

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestCheckoutNodeAuthorityUsesRealOwnerHTTPRiverSDKAndDurableRegistry(t *testing.T) {
	_, pool, org := fixtureStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var owner string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal("private fixture directory unavailable")
	}
	keyFile := filepath.Join(root, "api.key")
	if err := os.WriteFile(keyFile, []byte(fixtureSecret(t, "rk_test_")), 0600); err != nil {
		t.Fatal("fixture key creation failed")
	}
	controlKey, webhookSecret := fixtureSecret(t, "fixture_control_"), fixtureSecret(t, "whsec_")
	controlFile, webhookFile := filepath.Join(root, "control.key"), filepath.Join(root, "webhook.key")
	for path, value := range map[string]string{controlFile: controlKey, webhookFile: webhookSecret} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal("private completion key creation failed")
		}
	}
	runner, err := filepath.Abs("../../../deploy/local/fixtures/checkout-run.mjs")
	if err != nil {
		t.Fatal(err)
	}
	process := exec.CommandContext(ctx, "node", runner, root, keyFile, "price_fixtureSessions", "http://127.0.0.1:5173", "0", controlFile, webhookFile)
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Start(); err != nil {
		t.Fatal("Node fixture startup failed")
	}
	t.Cleanup(func() {
		if process.Process != nil {
			_ = process.Process.Signal(os.Interrupt)
		}
		_ = process.Wait()
	})
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("fixture did not publish safe loopback origin")
	}
	var announced struct {
		Fixture  bool   `json:"fixture"`
		Origin   string `json:"origin"`
		Payments bool   `json:"payments"`
	}
	if json.Unmarshal(scanner.Bytes(), &announced) != nil || !announced.Fixture || announced.Payments || !sessionOrigin(announced.Origin) {
		t.Fatal("fixture startup crossed billing boundary")
	}
	a, err := NewFixtureSessions(SessionSettings{Billing: Settings{Enabled: true, FixtureURL: announced.Origin, FixtureKeyFile: keyFile, WebhookSecret: webhookSecret}, PriceID: "price_fixtureSessions", PortalConfigurationID: "bpc_fixtureSessions", ReturnOrigin: "http://127.0.0.1:5173"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewCheckoutIntents(true, pool, a)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := NewCompletionInbox(true, pool, a)
	if err != nil {
		t.Fatal(err)
	}
	owners := checkoutHTTPAuth(t, s, owner)
	mux := http.NewServeMux()
	s.RegisterCheckout(owners, a)(mux)
	inbox.Register()(mux)
	base := "/api/organizations/" + org + "/billing/checkout"
	checkoutHTTPRequest(t, mux, "GET", base+"-plan", "", nil, 200)
	var receipt CheckoutIntent
	data := checkoutHTTPRequest(t, mux, "POST", base, `{"idempotencyKey":"`+sessionFixtureRequest.Key+`","confirmed":true}`, nil, 200)
	if json.Unmarshal(data, &receipt) != nil || receipt.State != "prepared" || receipt.CanPay {
		t.Fatal("Node journey preparation invalid")
	}
	if _, err = os.Stat(filepath.Join(root, "authority.json")); !os.IsNotExist(err) {
		t.Fatal("preparation created authority state")
	}
	for range 2 {
		checkoutHTTPRequest(t, mux, "POST", base+"/"+receipt.ID+"/start", `{"confirmed":true}`, nil, 200)
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &CheckoutWorker{Store: s, Authority: a})
	river.AddWorker(workers, &CompletionWorker{Inbox: inbox, Intents: s, Authority: a})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal("disposable checkout worker startup failed")
	}
	defer func() {
		stop, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := client.Stop(stop); err != nil {
			t.Error("disposable checkout worker shutdown failed")
		}
	}()
	for {
		var ready bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_checkout_requests WHERE id=$1 AND state='session_ready') AND EXISTS(SELECT 1 FROM river_job WHERE kind='billing_checkout' AND state='completed')`, receipt.ID).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Node/River checkout did not complete")
		case <-time.After(20 * time.Millisecond):
		}
	}
	before := checkoutLedger(t, s)
	for range 2 {
		data = checkoutHTTPRequest(t, mux, "GET", base+"/"+receipt.ID+"/session", "", nil, 200)
		var session RecoveredCheckout
		if json.Unmarshal(data, &session) != nil || session.Status != "open" || !strings.HasPrefix(session.URL, announced.Origin+"/checkout/cs_test_fixture") {
			t.Fatal("Node authority recovery failed")
		}

		request, err := http.NewRequestWithContext(ctx, "GET", session.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal("hosted fixture navigation failed")
		}
		page, readErr := io.ReadAll(io.LimitReader(response.Body, 8193))
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || len(page) > 8192 || !strings.Contains(string(page), "Disposable checkout review") || !strings.Contains(string(page), "Fixture session: open.") || strings.Contains(string(page), "<script") || strings.Contains(string(page), "<form") {
			t.Fatal("hosted fixture navigation crossed read-only boundary")
		}
	}
	if checkoutLedger(t, s) != before {
		t.Fatal("Node recovered navigation changed management ledger")
	}
	var registry struct {
		Requests map[string]struct {
			Resource struct {
				ID string `json:"id"`
			} `json:"resource"`
		} `json:"requests"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "authority.json"))
	if err != nil || json.Unmarshal(raw, &registry) != nil || len(registry.Requests) != 2 {
		t.Fatal("Node authority created duplicate resources")
	}
	var customer, checkout string
	if err = pool.QueryRow(ctx, `SELECT customer_id,checkout_id FROM billing_checkout_requests WHERE id=$1`, receipt.ID).Scan(&customer, &checkout); err != nil {
		t.Fatal(err)
	}
	if registry.Requests["customer:pdsaas-customer-"+receipt.ID].Resource.ID != customer || registry.Requests["checkout:pdsaas-checkout-"+receipt.ID].Resource.ID != checkout {
		t.Fatal("Node resources lost original SDK identity")
	}
	var unsafe bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_checkout_requests WHERE row_to_json(billing_checkout_requests)::text LIKE '%/checkout/%') OR EXISTS(SELECT 1 FROM billing_customer_bindings) OR EXISTS(SELECT 1 FROM billing_subscription_observations) OR EXISTS(SELECT 1 FROM pilot_entitlement_operations)`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("Node checkout granted billing/hosting authority or retained navigation")
	}
	// Completion uses a separate host-only credential; the owner API never receives it.
	input := map[string]string{"nonce": "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "sessionId": checkout, "type": "checkout.session.completed", "paymentState": "unpaid", "status": "past_due"}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Fixture   bool   `json:"fixture"`
		Payments  bool   `json:"payments"`
		EventID   string `json:"eventId"`
		Body      string `json:"body"`
		Signature string `json:"signature"`
	}
	for range 2 {
		request, err := http.NewRequestWithContext(ctx, "POST", announced.Origin+"/_fixture/checkout/completion", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+controlKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal("private completion control failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 262145))
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || len(raw) > 262144 || json.Unmarshal(raw, &evidence) != nil || !evidence.Fixture || evidence.Payments {
			t.Fatal("completion crossed fixture boundary")
		}
		delivery := httptest.NewRequest("POST", "/api/billing/checkout-webhook", strings.NewReader(evidence.Body))
		delivery.Header.Set("Stripe-Signature", evidence.Signature)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, delivery)
		if recorder.Code != 200 {
			t.Fatal("signed Node completion rejected")
		}
	}
	for {
		var valid bool
		err = pool.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_checkout_notifications WHERE event_id=$1 AND state='applied') AND (SELECT count(*)=1 FROM river_job WHERE kind='billing_checkout_completion' AND state='completed')`, evidence.EventID).Scan(&valid)
		if err != nil {
			t.Fatal(err)
		}
		if valid {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Node completion worker did not complete")
		case <-time.After(20 * time.Millisecond):
		}
	}
	var valid bool
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_customer_bindings WHERE organization_id=$1 AND reason='checkout_verified' AND checkout_request_id=$2 AND checkout_actor_id=$3 AND customer_id=$4 AND completion_event_id=$5) AND (SELECT count(*)=1 FROM billing_checkout_events WHERE request_id=$2 AND action='completion_bound' AND actor_id=$3 AND completion_event_id=$5) AND (SELECT count(*)=1 FROM billing_subscription_observations WHERE organization_id=$1 AND subscription_state='past_due' AND checkout_payment_state='unpaid' AND completion_event_id=$5) AND NOT EXISTS(SELECT 1 FROM pilot_entitlement_operations)`, org, receipt.ID, owner, customer, evidence.EventID).Scan(&valid)
	if err != nil || !valid {
		t.Fatal("Node completion lost custody, duplicated mapping or granted unpaid access")
	}
	raw, err = os.ReadFile(filepath.Join(root, "authority.json"))
	if err != nil || json.Unmarshal(raw, &registry) != nil || len(registry.Requests) != 2 || registry.Requests["customer:pdsaas-customer-"+receipt.ID].Resource.ID != customer || registry.Requests["checkout:pdsaas-checkout-"+receipt.ID].Resource.ID != checkout {
		t.Fatal("Node completion replaced original resources")
	}

}
