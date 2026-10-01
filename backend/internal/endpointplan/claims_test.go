package endpointplan

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestClaimSchemaEnforcesExclusiveReservedHostname(t *testing.T) {
	s, c := publicChoice(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO endpoint_claims(choice_id,organization_id,actor_id,hostname,expires_at,proof_sha256,verified_at) VALUES($1,$2,$3,$4,$5,sha256(convert_to($6,'UTF8')),statement_timestamp())`, c.ID, org, alice, c.Hostname, c.ExpiresAt, c.ProofValue); err != nil {
		t.Fatal("exclusive claim storage unavailable", err)
	}
	second := secondClaimChoice(t, s, c.Hostname)
	_, err := s.Pool.Exec(ctx, `INSERT INTO endpoint_claims(choice_id,organization_id,actor_id,hostname,expires_at,proof_sha256,verified_at) VALUES($1,$2,$3,$4,$5,sha256(convert_to($6,'UTF8')),statement_timestamp())`, second.ID, second.OrganizationID, bob, c.Hostname, second.ExpiresAt, second.ProofValue)
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.ConstraintName != "endpoint_claims_reserved_hostname" {
		t.Fatal("database allowed competing hostname reservation", err)
	}
}

func secondClaimChoice(t *testing.T, s *Store, hostname string) Choice {
	t.Helper()
	ctx := context.Background()
	var otherOrg string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'second claim') RETURNING id`, bob).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveChoice(ctx, bob, otherOrg, ChoiceRequest{hostname, []string{"app.other.org"}, "competing", true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClaimReservationIsExclusiveStableAndNeverActivation(t *testing.T) {
	s, c := publicChoice(t)
	ctx := context.Background()
	other := secondClaimChoice(t, s, c.Hostname)
	var wait sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claim, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}})
			if err == nil && (!claim.Reserved || claim.CanActivate || claim.State != "reserved" || !claim.ExpiresAt.Equal(c.ExpiresAt)) {
				err = errors.New("reservation authority inconsistent")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var claims, audits int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_claims),(SELECT count(*) FROM endpoint_claim_events)`).Scan(&claims, &audits); err != nil || claims != 1 || audits != 1 {
		t.Fatal("duplicate claim or audit", claims, audits, err)
	}
	if _, err := s.ReserveClaim(ctx, bob, other.OrganizationID, other.ID, &txtFixture{records: []string{c.ProofValue, other.ProofValue}}); !errors.Is(err, ErrConflict) {
		t.Fatal("competing verified proof displaced reservation", err)
	}
	if _, err := s.GetClaim(ctx, bob, org, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign claim disclosed")
	}
	dns := &txtFixture{err: errors.New("DNS unavailable")}
	if claim, err := s.ReserveClaim(ctx, alice, org, c.ID, dns); err != nil || !claim.Reserved || dns.calls.Load() != 0 {
		t.Fatal("retained transport retry needed DNS", err)
	}
	if _, err := s.CancelChoice(ctx, alice, org, c.ID, "release", true); err != nil {
		t.Fatal(err)
	}
	claim, err := s.GetClaim(ctx, alice, org, c.ID)
	if err != nil || claim.Reserved || claim.State != "released" || claim.CanActivate {
		t.Fatal("cancelled choice retained reservation", err)
	}
	if claim, err = s.ReserveClaim(ctx, alice, org, c.ID, dns); err != nil || claim.Reserved || dns.calls.Load() != 0 {
		t.Fatal("released receipt reopened reservation", err)
	}
	if claim, err = s.ReserveClaim(ctx, bob, other.OrganizationID, other.ID, &txtFixture{records: []string{other.ProofValue}}); err != nil || !claim.Reserved {
		t.Fatal("released hostname unavailable", err)
	}
}

func TestExpiredClaimIsReclaimedWithSystemAudit(t *testing.T) {
	s, c := publicChoice(t)
	ctx := context.Background()
	other := secondClaimChoice(t, s, c.Hostname)
	if _, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_claims SET created_at=statement_timestamp()-interval '1 day',verified_at=statement_timestamp()-interval '1 day',expires_at=clock_timestamp()-interval '1 second' WHERE choice_id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	claim, err := s.GetClaim(ctx, alice, org, c.ID)
	if err != nil || claim.Reserved || claim.State != "expired" {
		t.Fatal("expired claim remained current", err)
	}
	if _, err = s.ReserveClaim(ctx, bob, other.OrganizationID, other.ID, &txtFixture{records: []string{other.ProofValue}}); err != nil {
		t.Fatal(err)
	}
	var expired, active int
	if err = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_claim_events WHERE action='expire' AND actor_id IS NULL),(SELECT count(*) FROM endpoint_claims WHERE state='reserved')`).Scan(&expired, &active); err != nil || expired != 1 || active != 1 {
		t.Fatal("expiry reclamation not audited exclusively", err)
	}
	if claim, err = s.ReserveClaim(ctx, alice, org, c.ID, nil); err != nil || claim.Reserved || claim.State != "expired" {
		t.Fatal("expired ticket reopened claim", err)
	}
}

func TestUnconfirmedProofAndRecoveryNeverReserve(t *testing.T) {
	for _, mode := range []string{"missing", "local", "cancelled", "expired", "foreign", "held", "missing-ledger", "production", "nil"} {
		t.Run(mode, func(t *testing.T) {
			s, c := publicChoice(t)
			ctx := context.Background()
			actor := alice
			dns := &txtFixture{records: []string{c.ProofValue}}
			var resolver TXTResolver = dns
			switch mode {
			case "missing":
				dns.records = nil
			case "local":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET hostname='pds.creator.test' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				if _, err := s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				actor = bob
			case "held":
				if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
					t.Fatal(err)
				}
			case "missing-ledger":
				if _, err := s.Pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
					t.Fatal(err)
				}
			case "production":
				s.Local = false
			case "nil":
				resolver = nil
			}
			if _, err := s.ReserveClaim(ctx, actor, org, c.ID, resolver); err == nil {
				t.Fatal("unconfirmed proof reserved")
			}
			var count int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM endpoint_claims`).Scan(&count); err != nil || count != 0 {
				t.Fatal("refusal left claim", err)
			}
			if mode != "missing" && dns.calls.Load() != 0 {
				t.Fatal("terminal claim sent DNS", mode)
			}
		})
	}
}

func TestClaimAuditFailureRollsBackReservationAndRelease(t *testing.T) {
	s, c := publicChoice(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_claim_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_claim_audit BEFORE INSERT ON endpoint_claim_events FOR EACH ROW EXECUTE FUNCTION reject_claim_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}}); err == nil {
		t.Fatal("audit failure accepted reservation")
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM endpoint_claims`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed audit left reservation")
	}
	if _, err := s.Pool.Exec(ctx, `DROP TRIGGER reject_claim_audit ON endpoint_claim_events`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `CREATE TRIGGER reject_claim_audit BEFORE INSERT ON endpoint_claim_events FOR EACH ROW EXECUTE FUNCTION reject_claim_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err == nil {
		t.Fatal("audit failure accepted release")
	}
	choice, err := s.GetChoice(ctx, alice, org, c.ID)
	if err != nil || choice.State != "awaiting_ownership" {
		t.Fatal("release failure cancelled choice")
	}
	claim, err := s.GetClaim(ctx, alice, org, c.ID)
	if err != nil || !claim.Reserved {
		t.Fatal("release failure released reservation")
	}
}

func TestClaimHTTPRefusesBodyAuthorityAndProduction(t *testing.T) {
	s, c := publicChoice(t)
	a := &sessions{owner: alice}
	dns := &txtFixture{records: []string{c.ProofValue}}
	mux := http.NewServeMux()
	s.RegisterClaims(a, dns)(mux)
	path := "/api/organizations/" + org + "/endpoint-choices/" + c.ID + "/claim"
	for _, tc := range []struct {
		body, bearer string
		status       int
	}{{"null", "", 400}, {`{"resolver":"PRIVATE"}`, "", 400}, {"{} {}", "", 400}, {"{}", "Bearer private", 403}, {"{}", "", 200}} {
		r := httptest.NewRequest("POST", path, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		if tc.bearer != "" {
			r.Header.Set("Authorization", tc.bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), c.ProofValue) {
			t.Fatal("claim HTTP boundary", w.Code, tc.status)
		}
	}
	if dns.calls.Load() != 1 || !a.calledMutation {
		t.Fatal("claim lookup or mutation boundary")
	}
	s.Local = false
	r := httptest.NewRequest("POST", path, strings.NewReader("{}"))
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "fixture-csrf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 || dns.calls.Load() != 1 {
		t.Fatal("production claim opened")
	}
}

func TestClaimRechecksScopeProofAndCancellationDuringDNS(t *testing.T) {
	for _, mode := range []string{"cancelled", "expired", "owner", "proof", "held", "native", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s, c := publicChoice(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dns := pausedTXT{make(chan struct{}), make(chan struct{}), c.ProofValue}
			done := make(chan error, 1)
			go func() { _, err := s.ReserveClaim(ctx, alice, org, c.ID, dns); done <- err }()
			select {
			case <-dns.entered:
			case <-ctx.Done():
				t.Fatal("DNS did not start")
			}
			switch mode {
			case "cancelled":
				if _, err := s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
					t.Fatal(err)
				}
			case "owner":
				if _, err := s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
					t.Fatal(err)
				}
			case "proof":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET proof_value=$2 WHERE id=$1`, c.ID, "pdsaas-domain-v1="+strings.Repeat("b", 43)); err != nil {
					t.Fatal(err)
				}
			case "held":
				if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
					t.Fatal(err)
				}
			case "native":
				if _, err := s.Pool.Exec(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,'retained','local','retained','agent','queued','fixture.localhost','https://fixture.localhost')`, org); err != nil {
					t.Fatal(err)
				}
			case "timeout":
				cancel()
			}
			close(dns.release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("stale DNS match reserved claim", mode)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("claim lookup did not finish")
			}
			var count int
			if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM endpoint_claims`).Scan(&count); err != nil || count != 0 {
				t.Fatal("stale match persisted claim", err)
			}
		})
	}
}

func TestClaimReceiptRequiresOriginalProofAndCurrentOwner(t *testing.T) {
	for _, mode := range []string{"proof", "owner", "held"} {
		t.Run(mode, func(t *testing.T) {
			s, c := publicChoice(t)
			ctx := context.Background()
			if _, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}}); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "proof":
				if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_choices SET proof_value=$2 WHERE id=$1`, c.ID, "pdsaas-domain-v1="+strings.Repeat("b", 43)); err != nil {
					t.Fatal(err)
				}
			case "owner":
				if _, err := s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
					t.Fatal(err)
				}
			case "held":
				if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.GetClaim(ctx, alice, org, c.ID); err == nil {
				t.Fatal("stale claim read disclosed authority")
			}
			if _, err := s.ReserveClaim(ctx, alice, org, c.ID, nil); err == nil {
				t.Fatal("stale claim replay disclosed authority")
			}
			if mode == "owner" {
				if _, err := s.GetClaim(ctx, bob, org, c.ID); !errors.Is(err, ErrNotFound) {
					t.Fatal("new owner read original claim")
				}
			}
		})
	}
}

func TestCompetingVerifiedClaimantsCannotBothWin(t *testing.T) {
	s, c := publicChoice(t)
	other := secondClaimChoice(t, s, c.Hostname)
	ctx := context.Background()
	results := make(chan error, 2)
	for _, entry := range []struct {
		actor  string
		choice Choice
	}{{alice, c}, {bob, other}} {
		go func(actor string, choice Choice) {
			_, err := s.ReserveClaim(ctx, actor, choice.OrganizationID, choice.ID, &txtFixture{records: []string{c.ProofValue, other.ProofValue}})
			results <- err
		}(entry.actor, entry.choice)
	}
	accepted, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatal("competing claim outcomes", accepted, conflicts)
	}
	var claims, audits int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_claims WHERE state='reserved'),(SELECT count(*) FROM endpoint_claim_events)`).Scan(&claims, &audits); err != nil || claims != 1 || audits != 1 {
		t.Fatal("competing claim committed more than one reservation", err)
	}
}

func TestClaimRechecksRecoveryAfterWaitingForHostnameLock(t *testing.T) {
	s, c := publicChoice(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lock, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,73647302))`, c.Hostname); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.ReserveClaim(ctx, alice, org, c.ID, &txtFixture{records: []string{c.ProofValue}})
		done <- err
	}()
	for {
		var waiting bool
		if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%hashtextextended%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("claim did not wait for hostname lock")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("claim ignored recovery hold introduced during lock wait")
	}
	var count int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM endpoint_claims`).Scan(&count); err != nil || count != 0 {
		t.Fatal("held claim persisted", err)
	}
}
