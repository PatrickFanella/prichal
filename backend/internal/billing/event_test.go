package billing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

func fixtureSecret(t *testing.T, prefix string) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
func fixturePayload(t *testing.T, id, kind string, created int64, object any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"id": id, "object": "event", "api_version": stripe.APIVersion, "livemode": false, "type": kind, "created": created, "data": map[string]any{"object": object}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func fixtureSubscription() map[string]any {
	return map[string]any{"id": "sub_fixtureAlice", "object": "subscription", "customer": "cus_fixtureAlice", "livemode": false, "status": "active", "metadata": map[string]string{"private_marker": "not-retained"}}
}
func signature(raw []byte, secret string, timestamp time.Time) string {
	return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: raw, Secret: secret, Timestamp: timestamp}).Header
}
func TestVerifyRawSignatureAndEnvironment(t *testing.T) {
	secret := fixtureSecret(t, "whsec_")
	raw := fixturePayload(t, "evt_fixtureAlice", "customer.subscription.updated", time.Now().Unix(), fixtureSubscription())
	for _, tc := range []struct {
		name   string
		body   []byte
		header string
	}{{"missing", raw, ""}, {"wrong key", raw, signature(raw, fixtureSecret(t, "whsec_"), time.Now())}, {"tampered", append(append([]byte{}, raw...), byte(' ')), signature(raw, secret, time.Now())}, {"expired", raw, signature(raw, secret, time.Now().Add(-10*time.Minute))}, {"future", raw, signature(raw, secret, time.Now().Add(10*time.Minute))}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Verify(tc.body, tc.header, secret); err == nil {
				t.Fatal("unauthenticated event accepted")
			}
		})
	}
	event, supported, err := Verify(raw, signature(raw, secret, time.Now()), secret)
	if err != nil || !supported || event.Customer != "cus_fixtureAlice" || event.Subscription != "sub_fixtureAlice" || len(event.Digest) != 32 {
		t.Fatal("valid event rejected")
	}
	for _, field := range []string{"livemode", "account", "context", "api_version"} {
		t.Run(field, func(t *testing.T) {
			var body map[string]any
			json.Unmarshal(raw, &body)
			switch field {
			case "livemode":
				body[field] = true
			case "account":
				body[field] = "acct_fixture"
			case "context":
				body[field] = "acct_fixture"
			case "api_version":
				body[field] = "2026-08-26.dahlia"
			}
			payload, _ := json.Marshal(body)
			if _, _, err := Verify(payload, signature(payload, secret, time.Now()), secret); err == nil {
				t.Fatal("unsafe event mode/version accepted")
			}
		})
	}
	if _, _, err = Verify(raw, signature(raw, secret, time.Now())+",t="+"1790856000", secret); err == nil {
		t.Fatal("duplicate timestamp accepted")
	}
	if strings.Contains(event.Customer, "private_marker") {
		t.Fatal("private content retained")
	}
}
func TestInvoiceGraphAndUnsupportedEvents(t *testing.T) {
	secret := fixtureSecret(t, "whsec_")
	invoice := map[string]any{"id": "in_fixtureAlice", "object": "invoice", "livemode": false, "customer": "cus_fixtureAlice", "parent": map[string]any{"type": "subscription_details", "subscription_details": map[string]string{"subscription": "sub_fixtureAlice"}}}
	raw := fixturePayload(t, "evt_fixtureInvoice", "invoice.payment_failed", time.Now().Unix(), invoice)
	event, supported, err := Verify(raw, signature(raw, secret, time.Now()), secret)
	if err != nil || !supported || event.Subscription != "sub_fixtureAlice" {
		t.Fatal("invoice graph was not resolved")
	}
	delete(invoice, "parent")
	raw = fixturePayload(t, "evt_fixtureOneOff", "invoice.paid", time.Now().Unix(), invoice)
	if _, supported, err = Verify(raw, signature(raw, secret, time.Now()), secret); err != nil || supported {
		t.Fatal("one-off invoice triggered subscription reconciliation")
	}
	raw = fixturePayload(t, "evt_fixtureIgnored", "payment_intent.succeeded", time.Now().Unix(), map[string]string{"id": "pi_fixture"})
	if _, supported, err = Verify(raw, signature(raw, secret, time.Now()), secret); err != nil || supported {
		t.Fatal("unsupported event triggered work")
	}
}
