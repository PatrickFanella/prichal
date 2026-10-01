package endpointplan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This optional qualification uses the already-running, fixed local PostgreSQL.
// Both databases come from testdb and are disposable; no source management data
// or host execution authority is copied into the fixture.
func TestPopulatedClaimSnapshotRecovery(t *testing.T) {
	if os.Getenv("PDS_TEST_LOCAL_CLAIM_SNAPSHOT") != "true" {
		t.Skip("requires explicit fixed-local snapshot qualification")
	}
	s, released := publicChoice(t)
	config := s.Pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.Port != 35433 || config.User != "pds_local" || !strings.HasPrefix(config.Database, "pds_smoke_") {
		t.Fatal("fixed disposable local database required")
	}
	ctx := context.Background()
	docker := func(input []byte, args ...string) []byte {
		t.Helper()
		deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		command := exec.CommandContext(deadline, "docker", args...)
		command.Stdin = bytes.NewReader(input)
		// Never print subprocess output: a dump or database error can contain proof data.
		output, err := command.Output()
		if err != nil {
			t.Fatal("fixed-local snapshot subprocess failed")
		}
		return output
	}
	if !strings.HasPrefix(strings.TrimSpace(string(docker(nil, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}"))), "unix:///") {
		t.Fatal("local Docker socket required")
	}
	labels := strings.TrimSpace(string(docker(nil, "inspect", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}/{{index .Config.Labels \"com.docker.compose.service\"}}", "pds-ass-local-db-1")))
	if labels != "pds-ass-local/db" {
		t.Fatal("fixed project PostgreSQL required")
	}
	reserve := func(actor string, c Choice) {
		t.Helper()
		if receipt, err := s.ReserveClaim(ctx, actor, c.OrganizationID, c.ID, &txtFixture{records: []string{c.ProofValue}}); err != nil || !receipt.Reserved || receipt.CanActivate {
			t.Fatal("fixture reservation failed", err)
		}
	}
	reserve(alice, released)
	if _, err := s.CancelChoice(ctx, alice, org, released.ID, "snapshot-release", true); err != nil {
		t.Fatal(err)
	}
	active := secondClaimChoice(t, s, released.Hostname)
	reserve(bob, active)
	expired := secondClaimChoice(t, s, "expired.creator.net")
	reserve(bob, expired)
	if _, err := s.Pool.Exec(ctx, `UPDATE endpoint_claims SET created_at=statement_timestamp()-interval '1 day',verified_at=statement_timestamp()-interval '1 day',expires_at=statement_timestamp()-interval '1 second' WHERE choice_id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	competing, err := s.SaveChoice(ctx, alice, org, ChoiceRequest{active.Hostname, []string{"app.other.org"}, "snapshot-competing", true})
	if err != nil {
		t.Fatal(err)
	}
	var claims, audits int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_claims),(SELECT count(*) FROM endpoint_claim_events)`).Scan(&claims, &audits); err != nil || claims != 3 || audits != 4 {
		t.Fatal("populated fixture ledger incomplete", err)
	}
	before := claimRecoveryInventory(t, s.Pool)
	snapshot := docker(nil, "exec", "pds-ass-local-db-1", "pg_dump", "-U", "pds_local", "-Fc", config.Database)
	if len(snapshot) == 0 || len(snapshot) > 8*1024*1024 {
		t.Fatal("fixture snapshot outside qualification bound")
	}
	digest := sha256.Sum256(snapshot)
	restored := testdb.NewRecovery(t)
	docker(snapshot, "exec", "-i", "pds-ass-local-db-1", "pg_restore", "-U", "pds_local", "--exit-on-error", "--no-owner", "-d", restored.Config().ConnConfig.Database)
	if claimRecoveryInventory(t, restored) != before {
		t.Fatal("populated claim snapshot differs")
	}
	if err := recovery.Hold(ctx, restored, digest[:]); err != nil {
		t.Fatal(err)
	}
	copy := &Store{restored, true, "localhost"}
	dns := &txtFixture{records: []string{active.ProofValue}}
	for _, c := range []Choice{released, active, expired} {
		actor := bob
		if c.ID == released.ID {
			actor = alice
		}
		if _, err := copy.GetClaim(ctx, actor, c.OrganizationID, c.ID); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held restore disclosed claim authority", err)
		}
		if _, err := copy.ReserveClaim(ctx, actor, c.OrganizationID, c.ID, dns); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held restore accepted reservation retry", err)
		}
	}
	if _, err := copy.CancelChoice(ctx, bob, active.OrganizationID, active.ID, "held-release", true); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held restore released reservation", err)
	}
	if dns.calls.Load() != 0 || claimRecoveryInventory(t, restored) != before {
		t.Fatal("held attempts changed ledger or queried DNS")
	}
	observer := claimRecoveryObserver{}
	if err := recovery.Reconcile(ctx, restored, &observer); err != nil {
		t.Fatal(err)
	}
	if observer.calls != 0 {
		t.Fatal("empty fixture unexpectedly acquired host authority")
	}
	for _, tc := range []struct {
		actor    string
		choice   Choice
		state    string
		reserved bool
	}{
		{alice, released, "released", false}, {bob, active, "reserved", true}, {bob, expired, "expired", false},
	} {
		for i := 0; i < 2; i++ {
			receipt, err := copy.ReserveClaim(ctx, tc.actor, tc.choice.OrganizationID, tc.choice.ID, dns)
			if err != nil || receipt.State != tc.state || receipt.Reserved != tc.reserved || receipt.CanActivate {
				t.Fatal("restored exact receipt changed authority", err)
			}
		}
	}
	if _, err := copy.GetClaim(ctx, alice, active.OrganizationID, active.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("restored receipt disclosed across owners", err)
	}
	if dns.calls.Load() != 0 || claimRecoveryInventory(t, restored) != before || claimRecoveryInventory(t, s.Pool) != before {
		t.Fatal("receipt replay reopened authority or changed source")
	}
	if _, err := copy.ReserveClaim(ctx, alice, org, competing.ID, &txtFixture{records: []string{competing.ProofValue}}); !errors.Is(err, ErrConflict) {
		t.Fatal("restored active reservation lost exclusivity", err)
	}
	if claimRecoveryInventory(t, restored) != before || claimRecoveryInventory(t, s.Pool) != before {
		t.Fatal("competing restored claim changed retained ledgers")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(filepath.Join(root, ".local"), "claim-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	// Retain only a safe report; the fixture databases are removed by testdb.
	var version int
	if err := restored.QueryRow(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	report := fmt.Sprintf("schema=%d\nclaims=%d\nclaim_audits=%d\nrestored_held=true\nreconciled_without_hosts=true\nexact_receipts_preserved=true\nexclusive_hostname_preserved=true\nsource_fixture_unchanged=true\nactivation_authorized=false\ncompleted_at=%s\n", version, claims, audits, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(work, "report.txt"), []byte(report), 0600); err != nil {
		t.Fatal(err)
	}
}

func claimRecoveryInventory(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var digest string
	if err := pool.QueryRow(context.Background(), `SELECT encode(sha256(convert_to(string_agg(payload,E'\n' ORDER BY payload),'UTF8')),'hex') FROM (
SELECT 'choice:'||row_to_json(c)::text AS payload FROM endpoint_choices c UNION ALL
SELECT 'choice_event:'||row_to_json(e)::text FROM endpoint_choice_events e UNION ALL
SELECT 'claim:'||row_to_json(c)::text FROM endpoint_claims c UNION ALL
SELECT 'claim_event:'||row_to_json(e)::text FROM endpoint_claim_events e) receipts`).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}

type claimRecoveryObserver struct{ calls int }

func (o *claimRecoveryObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	o.calls++
	return nodeagent.Capacity{}, errors.New("fixture has no hosts")
}
func (o *claimRecoveryObserver) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	o.calls++
	return nodeagent.Instance{}, errors.New("fixture has no hosts")
}
