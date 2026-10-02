package billing

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
	"github.com/riverqueue/river"
)

type checkoutSnapshotAuthority struct{ calls int }

func (a *checkoutSnapshotAuthority) CreateCustomer(_ context.Context, key string) (string, error) {
	a.calls++
	return "cus_snapshot" + strings.ReplaceAll(key, "-", ""), nil
}
func (a *checkoutSnapshotAuthority) Checkout(_ context.Context, r SessionRequest) (HostedSession, error) {
	a.calls++
	return HostedSession{ID: "cs_test_snapshot" + strings.ReplaceAll(r.Key, "-", ""), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type checkoutSnapshotObserver struct{ calls int }

func (o *checkoutSnapshotObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	o.calls++
	return nodeagent.Capacity{}, errors.New("fixture has no host authority")
}
func (o *checkoutSnapshotObserver) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	o.calls++
	return nodeagent.Instance{}, errors.New("fixture has no host authority")
}

// TestPopulatedCheckoutSnapshotRecovery is opt-in to the already-running fixed local
// PostgreSQL. Both databases are allocator-owned fixtures, never source management data.
func TestPopulatedCheckoutSnapshotRecovery(t *testing.T) {
	if os.Getenv("PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT") != "true" {
		t.Skip("requires explicit fixed-local checkout snapshot qualification")
	}
	s, pool, owner, _, calls := fixtureCheckoutIntents(t)
	config := pool.Config().ConnConfig
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
	authority := &checkoutSnapshotAuthority{}
	cases := map[string]CheckoutIntent{}
	for _, state := range []string{"prepared", "queued", "customer_ready", "session_ready", "manual_review"} {
		var org, key string
		if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,$2) RETURNING id,gen_random_uuid()`, owner, "Snapshot "+state).Scan(&org, &key); err != nil {
			t.Fatal(err)
		}
		receipt, err := s.Prepare(ctx, owner, org, key, true)
		if err != nil {
			t.Fatal(err)
		}
		if state != "prepared" {
			if _, err = s.Start(ctx, owner, org, receipt.ID, true); err != nil {
				t.Fatal(err)
			}
			if err = s.reserveDispatch(ctx, receipt.ID); err != nil {
				t.Fatal(err)
			}
			if state == "customer_ready" {
				tx, actor, org, err := s.dispatchScope(ctx, receipt.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.dispatchStep(ctx, tx, receipt.ID, actor, org, authority); err != nil {
					_ = tx.Rollback(ctx)
					t.Fatal(err)
				}
			}
			if state == "session_ready" {
				if err = s.Dispatch(ctx, receipt.ID, authority); err != nil {
					t.Fatal(err)
				}
			}
			if state == "queued" || state == "customer_ready" || state == "manual_review" {
				if _, err = pool.Exec(ctx, `UPDATE billing_checkout_requests SET dispatch_started_at=clock_timestamp()-interval '24 hours' WHERE id=$1`, receipt.ID); err != nil {
					t.Fatal(err)
				}
				if state == "manual_review" {
					if err = s.Dispatch(ctx, receipt.ID, authority); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		receipt, err = s.Get(ctx, owner, org, receipt.ID)
		if err != nil || receipt.State != state {
			t.Fatal("populated checkout fixture state mismatch")
		}
		cases[state] = receipt
	}
	if requests, audits, jobs := checkoutCounts(t, pool); requests != 5 || audits != 17 || jobs != 4 {
		t.Fatal("populated checkout counts differ")
	}
	_ = checkoutHTTPAuth(t, s, owner)
	before := checkoutLedger(t, s)
	snapshot := docker(nil, "exec", "pds-ass-local-db-1", "pg_dump", "-U", "pds_local", "-Fc", config.Database)
	if len(snapshot) == 0 || len(snapshot) > 8*1024*1024 {
		t.Fatal("fixture checkout snapshot outside bound")
	}
	digest := sha256.Sum256(snapshot)
	restored := testdb.NewRecovery(t)
	docker(snapshot, "exec", "-i", "pds-ass-local-db-1", "pg_restore", "-U", "pds_local", "--exit-on-error", "--no-owner", "-d", restored.Config().ConnConfig.Database)
	adapter, err := NewFixtureSessions(s.settings)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := NewCheckoutIntents(true, restored, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if checkoutLedger(t, copy) != before {
		t.Fatal("restored checkout requests/audits/jobs differ")
	}
	var sessions int
	if err := restored.QueryRow(ctx, `SELECT count(*) FROM management_sessions`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatal("fixture session was not included")
	}
	if err := recovery.Hold(ctx, restored, digest[:]); err != nil {
		t.Fatal(err)
	}
	if err := restored.QueryRow(ctx, `SELECT count(*) FROM management_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("held restore resurrected authentication")
	}
	authority.calls = 0
	worker := &CheckoutWorker{Store: copy, Authority: authority}
	for _, receipt := range cases {
		if _, err := copy.Plan(ctx, owner, receipt.OrganizationID); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy disclosed catalog availability")
		}
		if _, err := copy.RecoverSession(ctx, owner, receipt.OrganizationID, receipt.ID, adapter); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy disclosed navigation")
		}
		if _, err := copy.Get(ctx, owner, receipt.OrganizationID, receipt.ID); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy disclosed receipt")
		}
		if _, err := copy.GetByKey(ctx, owner, receipt.OrganizationID, receipt.Key); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy disclosed original key")
		}
		if _, err := copy.Prepare(ctx, owner, receipt.OrganizationID, receipt.Key, true); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy accepted preparation")
		}
		if _, err := copy.Start(ctx, owner, receipt.OrganizationID, receipt.ID, true); !errors.Is(err, recovery.ErrHeld) {
			t.Fatal("held copy accepted dispatch")
		}
		if err := worker.Work(ctx, &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: receipt.ID}}); err == nil {
			t.Fatal("held worker consumed copied checkout")
		}
	}
	if authority.calls != 0 || calls.Load() != 0 || checkoutLedger(t, copy) != before || checkoutLedger(t, s) != before {
		t.Fatal("held restore changed source/copy or called authority")
	}
	observer := &checkoutSnapshotObserver{}
	if err := recovery.Reconcile(ctx, restored, observer); err != nil || observer.calls != 0 {
		t.Fatal("empty checkout fixture acquired host authority")
	}
	for state, receipt := range cases {
		got, err := copy.GetByKey(ctx, owner, receipt.OrganizationID, receipt.Key)
		if err != nil || got != receipt || got.CanPay {
			t.Fatal("restored original receipt lost custody/configuration")
		}
		if state == "session_ready" || state == "manual_review" {
			if err := copy.Dispatch(ctx, receipt.ID, authority); err != nil {
				t.Fatal("restored terminal dispatch changed authority")
			}
		}
		if _, err := copy.Get(ctx, sessionFixtureRequest.Key, receipt.OrganizationID, receipt.ID); !errors.Is(err, ErrCheckoutNotFound) {
			t.Fatal("restored receipt escaped original owner")
		}
	}
	if authority.calls != 0 || checkoutLedger(t, copy) != before || checkoutLedger(t, s) != before {
		t.Fatal("terminal replay changed copied/source ledger")
	}
	for _, state := range []string{"queued", "customer_ready"} {
		receipt := cases[state]
		var priorCustomer *string
		if err := restored.QueryRow(ctx, `SELECT customer_id FROM billing_checkout_requests WHERE id=$1`, receipt.ID).Scan(&priorCustomer); err != nil {
			t.Fatal(err)
		}
		if err := copy.Dispatch(ctx, receipt.ID, authority); err != nil {
			t.Fatal(err)
		}
		got, err := copy.Get(ctx, owner, receipt.OrganizationID, receipt.ID)
		if err != nil || got.State != "manual_review" || got.CanPay {
			t.Fatal("expired copied work recreated resource")
		}
		var same bool
		if err := restored.QueryRow(ctx, `SELECT customer_id IS NOT DISTINCT FROM $2::text AND checkout_id IS NULL FROM billing_checkout_requests WHERE id=$1`, receipt.ID, priorCustomer).Scan(&same); err != nil || !same {
			t.Fatal("expired restored checkpoint discarded original customer")
		}
	}
	if authority.calls != 0 || checkoutLedger(t, s) != before {
		t.Fatal("expired copy called authority or changed fixture source")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(filepath.Join(root, ".local"), "checkout-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	report := fmt.Sprintf("schema=32\nrequests=5\ncheckout_audits=17\ncheckout_jobs=4\nexact_snapshot_preserved=true\nheld_rpc_count=0\ncopied_authentication_revoked=true\nreconciled_without_hosts=true\noriginal_receipts_preserved=true\nterminal_replay_rpc_count=0\nexpired_unknown_rpc_count=0\nconfirmed_customer_retained=true\nsource_fixture_unchanged=true\npaid_entitlement=false\ncompleted_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(work, "report.txt"), []byte(report), 0600); err != nil {
		t.Fatal(err)
	}
}
