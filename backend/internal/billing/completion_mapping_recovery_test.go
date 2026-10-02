package billing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// The explicit local qualification copies only allocator-owned fixture data.
func TestPopulatedCompletionMappingSnapshotAndHeldRiverReplay(t *testing.T) {
	if os.Getenv("PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT") != "true" {
		t.Skip("requires explicit fixed-local mapped checkout snapshot qualification")
	}
	f := mappingFixture(t)
	config := f.inbox.pool.Config().ConnConfig
	if config.Host != "127.0.0.1" || config.Port != 35433 || config.User != "pds_local" || !strings.HasPrefix(config.Database, "pds_smoke_") {
		t.Fatal("fixed disposable local database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	docker := func(input []byte, args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(ctx, "docker", args...)
		command.Stdin = bytes.NewReader(input)
		output, err := command.Output()
		if err != nil {
			t.Fatal("fixed-local snapshot subprocess failed")
		}
		return output
	}
	if !strings.HasPrefix(strings.TrimSpace(string(docker(nil, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}"))), "unix:///") || strings.TrimSpace(string(docker(nil, "inspect", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}/{{index .Config.Labels \"com.docker.compose.service\"}}", "pds-ass-local-db-1"))) != "pds-ass-local/db" {
		t.Fatal("fixed project PostgreSQL and local socket required")
	}
	one := f.event(t, "evt_snapshotMapped", "checkout.session.completed")
	two := f.event(t, "evt_snapshotPending", "checkout.session.async_payment_failed")
	if _, err := f.inbox.Receive(ctx, one); err != nil {
		t.Fatal(err)
	}
	if err := f.inbox.Reconcile(ctx, one.ID, f.intents, f.adapter); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inbox.Receive(ctx, two); err != nil {
		t.Fatal(err)
	}
	_ = checkoutHTTPAuth(t, f.intents, f.owner)
	sourceBefore := f.ledger(t)
	calls := f.calls.Load()
	snapshot := docker(nil, "exec", "pds-ass-local-db-1", "pg_dump", "-U", "pds_local", "-Fc", config.Database)
	if len(snapshot) == 0 || len(snapshot) > 8*1024*1024 {
		t.Fatal("mapped snapshot outside fixture bound")
	}
	restored := testdb.NewRecovery(t)
	docker(snapshot, "exec", "-i", "pds-ass-local-db-1", "pg_restore", "-U", "pds_local", "--exit-on-error", "--no-owner", "-d", restored.Config().ConnConfig.Database)
	adapter, err := NewFixtureSessions(f.adapter.settings)
	if err != nil {
		t.Fatal(err)
	}
	intents, err := NewCheckoutIntents(true, restored, adapter)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := NewCompletionInbox(true, restored, adapter)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := New(restored)
	if err != nil {
		t.Fatal(err)
	}
	copy := &completionMappingFixture{inbox: inbox, intents: intents, adapter: adapter, legacy: legacy}
	if copy.ledger(t) != sourceBefore {
		t.Fatal("mapped restore lost exact binding/observation/delivery/audit/job data")
	}
	var included bool
	if err = restored.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_customer_bindings WHERE reason='checkout_verified') AND (SELECT count(*)=1 FROM billing_checkout_events WHERE action='completion_bound') AND (SELECT count(*)=1 FROM billing_subscription_observations WHERE checkout_payment_state='unpaid' AND event_id IS NULL AND completion_event_id=$1) AND (SELECT count(*)=1 FROM billing_checkout_notifications WHERE state='applied') AND (SELECT count(*)=1 FROM billing_checkout_notifications WHERE state='pending') AND (SELECT count(*)=3 FROM river_job) AND (SELECT count(*)=1 FROM management_sessions)`, one.ID).Scan(&included); err != nil || !included {
		t.Fatal("mapped snapshot inventory differs")
	}
	digest := sha256.Sum256(snapshot)
	if err = recovery.Hold(ctx, restored, digest[:]); err != nil {
		t.Fatal(err)
	}
	worker := &CompletionWorker{Inbox: inbox, Intents: intents, Authority: adapter}
	for _, e := range []CompletionEvent{one, two} {
		if err = worker.Work(ctx, &river.Job[CompletionArgs]{Args: CompletionArgs{EventID: e.ID}}); err == nil {
			t.Fatal("held mapped delivery consumed work")
		}
	}
	if _, err = inbox.Receive(ctx, two); err == nil {
		t.Fatal("held duplicate acknowledged")
	}
	var revoked bool
	if err = restored.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM management_sessions)`).Scan(&revoked); err != nil || !revoked {
		t.Fatal("mapped restore resurrected authentication")
	}
	if f.calls.Load() != calls || copy.ledger(t) != sourceBefore || f.ledger(t) != sourceBefore {
		t.Fatal("held mapping changed source/copy or called authority")
	}
	observer := &checkoutSnapshotObserver{}
	if err = recovery.Reconcile(ctx, restored, observer); err != nil || observer.calls != 0 {
		t.Fatal("mapped fixture acquired host authority")
	}
	if err = inbox.Reconcile(ctx, one.ID, intents, adapter); err != nil || f.calls.Load() != calls || copy.ledger(t) != sourceBefore {
		t.Fatal("applied mapped replay repeated or rewrote evidence")
	}
	f.state.Store("past_due")
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	river.AddWorker(workers, &CheckoutWorker{Store: intents, Authority: adapter})
	client, err := river.NewClient(riverpgxv5.New(restored), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		deadline, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := client.Stop(deadline); err != nil {
			t.Error("mapped copied worker shutdown failed")
		}
	}()
	for {
		var ready bool
		if err = restored.QueryRow(ctx, `SELECT (SELECT count(*)=3 FROM river_job WHERE state='completed') AND (SELECT count(*)=2 FROM billing_checkout_notifications WHERE state='applied')`).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("mapped copied jobs did not complete")
		case <-time.After(20 * time.Millisecond):
		}
	}
	var valid bool
	if err = restored.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_customer_bindings WHERE checkout_request_id=$1 AND checkout_actor_id=$2 AND completion_event_id=$3) AND (SELECT count(*)=1 FROM billing_checkout_events WHERE request_id=$1 AND action='completion_bound' AND actor_id=$2 AND executor_kind='worker' AND completion_event_id=$3) AND (SELECT count(*)=1 FROM billing_subscription_observations WHERE organization_id=$4 AND subscription_state='past_due' AND checkout_payment_state='unpaid' AND completion_event_id=$5 AND event_id IS NULL) AND NOT EXISTS(SELECT 1 FROM pilot_entitlement_operations)`, f.receipt.ID, f.owner, one.ID, f.receipt.OrganizationID, two.ID).Scan(&valid); err != nil || !valid || f.calls.Load() != calls+2 {
		t.Fatal("mapped replay replaced custody/audit or granted paid access")
	}
	completed := copy.ledger(t)
	if err = inbox.Reconcile(ctx, two.ID, intents, adapter); err != nil || copy.ledger(t) != completed || f.calls.Load() != calls+2 || f.ledger(t) != sourceBefore {
		t.Fatal("terminal replay changed copied/source evidence")
	}
	var safe bool
	if err = restored.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE row_to_json(billing_customer_bindings)::text LIKE '%private_marker%') AND NOT EXISTS(SELECT 1 FROM billing_subscription_observations WHERE row_to_json(billing_subscription_observations)::text LIKE '%owner-from-email%')`).Scan(&safe); err != nil || !safe {
		t.Fatal("mapped snapshot retained raw personal content")
	}
	project, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	reportDir, err := os.MkdirTemp(filepath.Join(project, ".local"), "checkout-mapping-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	report := fmt.Sprintf("schema=%d\nrequests=1\nverified_bindings=1\nbinding_audits=1\nobservations=1\nnotifications=2\ncopied_jobs=3\nexact_snapshot_preserved=true\ncopied_authentication_revoked=true\nheld_rpc_count=0\nreconciled_host_calls=0\napplied_replay_rpc_count=0\npending_replay_rpc_count=2\noriginal_binding_and_audit_preserved=true\ncurrent_unpaid_adverse_observation_preserved=true\nsource_fixture_unchanged=true\nterminal_replay_rpc_count=0\npaid_entitlement=false\ncompleted_at=%s\n", migrate.Version, time.Now().UTC().Format(time.RFC3339))
	if err = os.WriteFile(filepath.Join(reportDir, "report.txt"), []byte(report), 0600); err != nil {
		t.Fatal(err)
	}
}
