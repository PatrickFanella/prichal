package endpointplan

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type txtFixture struct {
	records  []string
	err      error
	calls    atomic.Int32
	name     string
	deadline bool
}

func (d *txtFixture) LookupTXT(ctx context.Context, name string) ([]string, error) {
	d.calls.Add(1)
	d.name = name
	at, ok := ctx.Deadline()
	d.deadline = ok && time.Until(at) <= 4*time.Second
	return d.records, d.err
}
func publicChoice(t *testing.T) (*Store, Choice) {
	t.Helper()
	s := fixture(t)
	r := selection("public")
	r.Hostname = "pds.creator.net"
	r.ApplicationHosts = []string{"app.other.org"}
	c, err := s.SaveChoice(context.Background(), alice, org, r)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}
func TestBoundedTXTObservationHasNoClaimAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		records     func(string) []string
		err         error
		matched     bool
	}{
		{"exact", "challenge_match", func(p string) []string { return []string{p} }, nil, true},
		{"shared-proof-name", "challenge_match", func(p string) []string {
			return []string{"unrelated=record", "pdsaas-domain-v1=" + strings.Repeat("b", 43), p, p}
		}, nil, true},
		{"other-choice", "pending_dns", func(string) []string { return []string{"pdsaas-domain-v1=" + strings.Repeat("b", 43)} }, nil, false},
		{"extra-byte", "pending_dns", func(p string) []string { return []string{p + " "} }, nil, false},
		{"missing", "pending_dns", func(string) []string { return nil }, &net.DNSError{IsNotFound: true}, false},
		{"private-error", "unavailable", func(string) []string { return nil }, errors.New("PRIVATE_DNS_SERVER_DETAILS"), false},
		{"too-many", "answer_too_large", func(p string) []string { return append([]string{p}, make([]string, 16)...) }, nil, false},
		{"too-large", "answer_too_large", func(p string) []string { return []string{p, strings.Repeat("x", 4097)} }, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := publicChoice(t)
			dns := &txtFixture{records: tc.records(c.ProofValue), err: tc.err}
			r, err := s.Observe(context.Background(), alice, org, c.ID, dns)
			if err != nil || r.State != tc.state || r.Matched != tc.matched || r.CanActivate || r.ClaimReserved || r.ChoiceID != c.ID || r.Hostname != c.Hostname || r.CheckedAt.IsZero() {
				t.Fatalf("observation: %+v %v", r, err)
			}
			if dns.calls.Load() != 1 || dns.name != c.ProofName || !dns.deadline {
				t.Fatal("lookup boundary")
			}
			raw, err := json.Marshal(r)
			if err != nil || strings.Contains(string(raw), c.ProofValue) || strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("private DNS projection")
			}
			current, err := s.GetChoice(context.Background(), alice, org, c.ID)
			if err != nil || current.State != "awaiting_ownership" || current.ProofValue != c.ProofValue {
				t.Fatal("observation persisted claim")
			}
			var audits int
			if err = s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM endpoint_choice_events`).Scan(&audits); err != nil || audits != 1 {
				t.Fatal("read audit mutation")
			}
		})
	}
}
func TestTXTTerminalAndForeignScopeSendNoDNS(t *testing.T) {
	for _, mode := range []string{"foreign", "cancelled", "expired", "local-namespace", "nil-resolver", "production"} {
		t.Run(mode, func(t *testing.T) {
			s, c := publicChoice(t)
			ctx := context.Background()
			dns := &txtFixture{records: []string{c.ProofValue}}
			actor := alice
			var resolver TXTResolver = dns
			want := mode
			switch mode {
			case "foreign":
				actor = bob
			case "cancelled":
				if _, err := s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "local-namespace":
				want = "local_namespace"
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET hostname='pds.creator.test' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "nil-resolver":
				resolver = nil
				want = "unavailable"
			case "production":
				s.Local = false
			}
			r, err := s.Observe(ctx, actor, org, c.ID, resolver)
			if mode == "foreign" || mode == "production" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatal("scope")
				}
			} else if err != nil || r.State != want || r.Matched || r.CanActivate || r.ClaimReserved {
				t.Fatal("terminal authority")
			}
			if dns.calls.Load() != 0 {
				t.Fatal("terminal DNS query")
			}
		})
	}
}

type pausedTXT struct {
	entered, release chan struct{}
	proof            string
}

func (d pausedTXT) LookupTXT(ctx context.Context, _ string) ([]string, error) {
	close(d.entered)
	select {
	case <-d.release:
		return []string{d.proof}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestTXTRechecksCustodyCancellationAndExpiryAfterLookup(t *testing.T) {
	for _, mode := range []string{"cancelled", "expired", "owner", "changed-proof", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s, c := publicChoice(t)
			ctx := context.Background()
			dns := pausedTXT{make(chan struct{}), make(chan struct{}), c.ProofValue}
			done := make(chan struct {
				r   Observation
				err error
			}, 1)
			if mode == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			go func() {
				r, err := s.Observe(ctx, alice, org, c.ID, dns)
				done <- struct {
					r   Observation
					err error
				}{r, err}
			}()
			<-dns.entered
			switch mode {
			case "cancelled":
				if _, err := s.CancelChoice(context.Background(), alice, org, c.ID, "cancel", true); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err := s.Pool.Exec(context.Background(), `UPDATE endpoint_choices SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "owner":
				if _, err := s.Pool.Exec(context.Background(), `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
					t.Fatal(err)
				}
			case "changed-proof":
				if _, err := s.Pool.Exec(context.Background(), `UPDATE endpoint_choices SET proof_value=$2 WHERE id=$1`, c.ID, "pdsaas-domain-v1="+strings.Repeat("b", 43)); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "timeout" {
				close(dns.release)
			}
			var out struct {
				r   Observation
				err error
			}
			select {
			case out = <-done:
			case <-time.After(time.Second):
				t.Fatal("lookup did not stop")
			}
			switch mode {
			case "cancelled", "expired":
				if out.err != nil || out.r.Matched || out.r.State != mode {
					t.Fatal("stale match")
				}
			case "owner":
				if !errors.Is(out.err, ErrNotFound) {
					t.Fatal("stale owner")
				}
			case "changed-proof":
				if !errors.Is(out.err, ErrConflict) {
					t.Fatal("stale challenge")
				}
			case "timeout":
				if out.err == nil || out.r.Matched {
					t.Fatal("expired context accepted")
				}
			}
		})
	}
}
func TestTXTObservationHTTPAndUnchangedChoice(t *testing.T) {
	s, c := publicChoice(t)
	dns := &txtFixture{records: []string{c.ProofValue}}
	a := &sessions{owner: alice}
	mux := http.NewServeMux()
	s.RegisterObservation(a, dns)(mux)
	path := "/api/organizations/" + org + "/endpoint-choices/" + c.ID + "/dns-observation"
	for _, tc := range []struct {
		body, bearer string
		want         int
	}{{`{}`, "", 200}, {`null`, "", 400}, {`{"resolver":"PRIVATE"}`, "", 400}, {`{} {}`, "", 400}, {`{}`, "Bearer private", 403}} {
		r := httptest.NewRequest("POST", path, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		if tc.bearer != "" {
			r.Header.Set("Authorization", tc.bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("HTTP observation boundary")
		}
	}
	if dns.calls.Load() != 1 || !a.calledMutation {
		t.Fatal("unexpected DNS or CSRF boundary")
	}
	s.Local = false
	r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "fixture-csrf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 || dns.calls.Load() != 1 {
		t.Fatal("production lookup")
	}
}
