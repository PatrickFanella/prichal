package billing

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type registryReplayAuthority struct {
	adapter *FixtureSessions
	calls   atomic.Int32
}

func (a *registryReplayAuthority) CreateCustomer(ctx context.Context, key string) (string, error) {
	a.calls.Add(1)
	return a.adapter.CreateCustomer(ctx, key)
}
func (a *registryReplayAuthority) Checkout(ctx context.Context, r SessionRequest) (HostedSession, error) {
	a.calls.Add(1)
	return a.adapter.Checkout(ctx, r)
}

// This opt-in restores only allocator-owned fixtures and never calls the native agent.
func TestCheckoutRegistrySnapshotRestartRecoversLostSessionCheckpoint(t *testing.T) {
	if os.Getenv("PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT") != "true" {
		t.Skip("requires explicit fixed-local registry snapshot qualification")
	}
	_, pool, org := fixtureStore(t)
	config := pool.Config().ConnConfig
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
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(root, "api.key")
	if err := os.WriteFile(keyFile, []byte(fixtureSecret(t, "rk_test_")), 0600); err != nil {
		t.Fatal(err)
	}
	runner, err := filepath.Abs("../../../deploy/local/fixtures/checkout-run.mjs")
	if err != nil {
		t.Fatal(err)
	}
	var process *exec.Cmd
	stop := func() {
		t.Helper()
		if process != nil {
			_ = process.Process.Signal(os.Interrupt)
			if err := process.Wait(); err != nil {
				t.Error("fixture authority shutdown failed")
			}
			process = nil
		}
	}
	defer stop()
	start := func(port string) string {
		t.Helper()
		process = exec.CommandContext(ctx, "node", runner, root, keyFile, "price_fixtureSessions", "http://127.0.0.1:5173", port)
		stdout, err := process.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err = process.Start(); err != nil {
			t.Fatal("fixture startup failed")
		}
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			t.Fatal("fixture startup missing safe origin")
		}
		var announce struct {
			Fixture  bool   `json:"fixture"`
			Origin   string `json:"origin"`
			Payments bool   `json:"payments"`
		}
		if json.Unmarshal(scanner.Bytes(), &announce) != nil || !announce.Fixture || announce.Payments || !sessionOrigin(announce.Origin) {
			t.Fatal("fixture crossed authority boundary")
		}
		return announce.Origin
	}
	origin := start("0")
	settings := SessionSettings{Billing: Settings{Enabled: true, FixtureURL: origin, FixtureKeyFile: keyFile}, PriceID: "price_fixtureSessions", PortalConfigurationID: "bpc_fixtureSessions", ReturnOrigin: "http://127.0.0.1:5173"}
	adapter, err := NewFixtureSessions(settings)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewCheckoutIntents(true, pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	var owner string
	if err = pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, owner, org, receipt.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.reserveDispatch(ctx, receipt.ID); err != nil {
		t.Fatal(err)
	}
	tx, actor, scope, err := s.dispatchScope(ctx, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.dispatchStep(ctx, tx, receipt.ID, actor, scope, adapter); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	// The actual SDK creates the session, then SQL checkpoint failure rolls back its receipt.
	if _, err = pool.Exec(ctx, `ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_registry_checkpoint CHECK(action<>'session_confirmed') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	tx, actor, scope, err = s.dispatchScope(ctx, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = s.dispatchStep(ctx, tx, receipt.ID, actor, scope, adapter)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("fixture did not lose session checkpoint")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE billing_checkout_events DROP CONSTRAINT reject_registry_checkpoint`); err != nil {
		t.Fatal(err)
	}
	var partial bool
	if err = pool.QueryRow(ctx, `SELECT state='customer_ready' AND customer_id IS NOT NULL AND checkout_id IS NULL AND session_expires_at IS NULL FROM billing_checkout_requests WHERE id=$1`, receipt.ID).Scan(&partial); err != nil || !partial {
		t.Fatal("failed checkpoint changed retained state")
	}
	registryFile := filepath.Join(root, "authority.json")
	retained, err := os.ReadFile(registryFile)
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Requests map[string]struct {
			Resource struct {
				ID      string `json:"id"`
				Expires int64  `json:"expires_at"`
			} `json:"resource"`
		} `json:"requests"`
	}
	if json.Unmarshal(retained, &registry) != nil || len(registry.Requests) != 2 {
		t.Fatal("lost checkpoint did not retain exact authority resources")
	}
	remote := registry.Requests["checkout:pdsaas-checkout-"+receipt.ID].Resource
	if !checkoutID.MatchString(remote.ID) || remote.Expires <= time.Now().Unix() {
		t.Fatal("retained original session invalid")
	}
	_ = checkoutHTTPAuth(t, s, owner)
	before := checkoutLedger(t, s)
	snapshot := docker(nil, "exec", "pds-ass-local-db-1", "pg_dump", "-U", "pds_local", "-Fc", config.Database)
	if len(snapshot) == 0 || len(snapshot) > 8*1024*1024 {
		t.Fatal("snapshot outside fixture bound")
	}
	restored := testdb.NewRecovery(t)
	docker(snapshot, "exec", "-i", "pds-ass-local-db-1", "pg_restore", "-U", "pds_local", "--exit-on-error", "--no-owner", "-d", restored.Config().ConnConfig.Database)
	stop()
	parsed, _ := url.Parse(origin)
	if next := start(parsed.Port()); next != origin {
		t.Fatal("restart changed retained authority scope")
	}
	fresh, err := NewFixtureSessions(settings)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := NewCheckoutIntents(true, restored, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if checkoutLedger(t, copy) != before {
		t.Fatal("restored original ledger differs")
	}
	digest := sha256.Sum256(snapshot)
	if err = recovery.Hold(ctx, restored, digest[:]); err != nil {
		t.Fatal(err)
	}
	counted := &registryReplayAuthority{adapter: fresh}
	worker := &CheckoutWorker{Store: copy, Authority: counted}
	if err = worker.Work(ctx, &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: receipt.ID}}); err == nil || counted.calls.Load() != 0 {
		t.Fatal("held worker acknowledged or called authority")
	}
	if _, err = copy.RecoverSession(ctx, owner, org, receipt.ID, fresh); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held copy exposed session")
	}
	var revoked bool
	if err = restored.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM management_sessions)`).Scan(&revoked); err != nil || !revoked {
		t.Fatal("copied authentication retained")
	}
	if checkoutLedger(t, copy) != before {
		t.Fatal("held copy consumed original work")
	}
	observer := &checkoutSnapshotObserver{}
	if err = recovery.Reconcile(ctx, restored, observer); err != nil || observer.calls != 0 {
		t.Fatal("fixture acquired host authority")
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
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
			t.Error("copied River worker shutdown failed")
		}
	}()
	for {
		var ready bool
		if err = restored.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_checkout_requests WHERE id=$1 AND state='session_ready') AND EXISTS(SELECT 1 FROM river_job WHERE kind='billing_checkout' AND state='completed')`, receipt.ID).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("copied River work did not complete")
		case <-time.After(20 * time.Millisecond):
		}
	}
	var customer, session string
	var expiry time.Time
	if err = restored.QueryRow(ctx, `SELECT customer_id,checkout_id,session_expires_at FROM billing_checkout_requests WHERE id=$1`, receipt.ID).Scan(&customer, &session, &expiry); err != nil || session != remote.ID || expiry.Unix() != remote.Expires || customer != registry.Requests["customer:pdsaas-customer-"+receipt.ID].Resource.ID {
		t.Fatal("replay replaced original resource or expiry")
	}
	completed := checkoutLedger(t, copy)
	if err = copy.Dispatch(ctx, receipt.ID, counted); err != nil || counted.calls.Load() != 1 || checkoutLedger(t, copy) != completed {
		t.Fatal("terminal replay repeated authority")
	}
	recovered, err := copy.RecoverSession(ctx, owner, org, receipt.ID, fresh)
	if err != nil || recovered.Status != "open" || !strings.Contains(recovered.URL, session) {
		t.Fatal("restored original navigation unavailable")
	}
	after, err := os.ReadFile(registryFile)
	if err != nil || !bytes.Equal(after, retained) || checkoutLedger(t, s) != before || checkoutLedger(t, copy) != completed {
		t.Fatal("replay/read altered registry or source")
	}
	var safe bool
	if err = restored.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM billing_customer_bindings) AND NOT EXISTS(SELECT 1 FROM billing_subscription_observations) AND NOT EXISTS(SELECT 1 FROM pilot_entitlement_operations)`).Scan(&safe); err != nil || !safe {
		t.Fatal("replay granted authority")
	}
	project, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	reportDir, err := os.MkdirTemp(filepath.Join(project, ".local"), "checkout-registry-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	report := fmt.Sprintf("schema=%d\nexact_snapshot_preserved=true\ncopied_authentication_revoked=true\nheld_rpc_count=0\nreconciled_host_calls=0\noriginal_authority_restarted=true\noriginal_resources=2\nreplay_rpc_count=1\noriginal_ids_and_expiry_preserved=true\nregistry_bytes_unchanged=true\nsource_fixture_unchanged=true\nterminal_replay_rpc_count=0\npaid_entitlement=false\ncompleted_at=%s\n", migrate.Version, time.Now().UTC().Format(time.RFC3339))
	if err = os.WriteFile(filepath.Join(reportDir, "report.txt"), []byte(report), 0600); err != nil {
		t.Fatal(err)
	}
}
