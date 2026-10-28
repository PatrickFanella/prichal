package deployment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedcheck"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

type publicAgent struct{ *managedRemote }

func (a publicAgent) ManagedStatus(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	host, _ := managednamespace.Endpoint(id)
	return nodeagent.Instance{ID: id, Endpoint: "https://" + host, State: "running", Healthy: true}, nil
}

type publicChecker struct {
	code  string
	calls atomic.Int64
	after func()
}

func (c *publicChecker) Check(_ context.Context, id string) managedcheck.Result {
	c.calls.Add(1)
	if c.after != nil {
		c.after()
	}
	host, _ := managednamespace.Endpoint(id)
	unknown, _ := managednamespace.Endpoint("aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
	expires := time.Now().Add(time.Hour)
	return managedcheck.Result{Hostname: host, Code: c.code, ObservedAt: time.Now(), Addresses: []string{"1.1.1.1"}, CertificateSHA256: strings.Repeat("a", 64), CertificateExpiresAt: &expires, Version: "0.4.5036", UnknownHost: unknown}
}
func publicFixture(t *testing.T) (*Store, string, string, AgentProvisioner, *publicChecker) {
	t.Helper()
	s, owner, _, op, a := nativeIntentFixture(t)
	a.healthy = true
	p := AgentProvisioner{Pool: s.Pool, Client: publicAgent{a}, ManagedRuntime: true}
	ctx := context.Background()
	for range 2 {
		if err := p.Step(ctx, op.ID); err != nil {
			t.Fatal(err)
		}
	}
	checker := &publicChecker{code: "ready"}
	p.PublicCheck = checker
	return s, owner, op.ID, p, checker
}
func expirePublicDelay(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `UPDATE managed_public_readiness SET next_check_at=now()-interval '1 second' WHERE deployment_id=$1`, id); err != nil {
		t.Fatal(err)
	}
}
func publicFailure(t *testing.T, s *Store, id string, p AgentProvisioner, checker *publicChecker) {
	t.Helper()
	checker.code = "tls"
	for i := 0; i < 3; i++ {
		err := p.Step(context.Background(), id)
		if i < 2 && !errors.Is(err, ErrPublicCheckPending) {
			t.Fatal("missing bounded pending result", err)
		}
		if i == 2 && err != nil {
			t.Fatal(err)
		}
		expirePublicDelay(t, s, id)
	}
	if _, err := s.Pool.Exec(context.Background(), `UPDATE river_job SET state='completed',finalized_at=now() WHERE args->>'deploymentId'=$1`, id); err != nil {
		t.Fatal(err)
	}
}
func TestPublicReadinessFailureReconstructionRetryAndAtomicReady(t *testing.T) {
	s, owner, id, p, checker := publicFixture(t)
	ctx := context.Background()
	publicFailure(t, s, id, p, checker)
	op, err := s.Get(ctx, owner, id)
	if err != nil || op.State != "failed" || op.Endpoint != "" || !strings.Contains(op.Message, "certificate") {
		t.Fatal("public failure advertised readiness", op, err)
	}
	var attempts, checks int
	if err = s.Pool.QueryRow(ctx, `SELECT attempts,(SELECT count(*) FROM managed_public_checks) FROM managed_public_readiness WHERE deployment_id=$1`, id).Scan(&attempts, &checks); err != nil || attempts != 3 || checks != 3 {
		t.Fatal("failure checkpoint lost", err)
	}
	var node string
	if err = s.Pool.QueryRow(ctx, `SELECT node_id::text FROM instance_placements WHERE deployment_id=$1`, id).Scan(&node); err != nil {
		t.Fatal(err)
	}
	key := "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"
	var wg sync.WaitGroup
	results := make(chan PublicRetry, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.RetryManagedPublic(ctx, "operator", id, node, key)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first PublicRetry
	for r := range results {
		if first.Cycle != 0 && first != r {
			t.Fatal("retry replay changed")
		}
		first = r
	}
	if first.Cycle != 2 {
		t.Fatal("retry did not create one cycle")
	}
	checker.code = "ready"
	p = AgentProvisioner{Pool: s.Pool, Client: p.Client, ManagedRuntime: true, PublicCheck: checker}
	if err = p.Step(ctx, id); err != nil {
		t.Fatal(err)
	}
	op, err = s.Get(ctx, owner, id)
	if err != nil || op.State != "ready" || op.Endpoint != "https://"+op.ManagedHostname {
		t.Fatal("complete public proof did not activate URL", op, err)
	}
	if checker.calls.Load() != 4 {
		t.Fatal("duplicate or missing check")
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_public_checks`).Scan(&checks); err != nil || checks != 4 {
		t.Fatal("success receipt missing", err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE args->>'deploymentId'=$1`, id).Scan(&checks); err != nil || checks != 2 {
		t.Fatal("retry duplicated job", err)
	}
	if _, err = s.RetryManagedPublic(ctx, "operator", id, node, key); err != nil {
		t.Fatal("completed same-key replay failed", err)
	}
	if _, err = s.RetryManagedPublic(ctx, "other", id, node, key); !errors.Is(err, ErrConflict) {
		t.Fatal("retry actor remapping accepted", err)
	}
}
func TestPublicChecksSerializeAndHonorDelay(t *testing.T) {
	s, _, id, p, checker := publicFixture(t)
	ctx := context.Background()
	checker.code = "dns"
	for range 2 {
		if err := p.checkPublic(ctx, id); !errors.Is(err, ErrPublicCheckPending) {
			t.Fatal(err)
		}
	}
	if checker.calls.Load() != 1 {
		t.Fatal("pending delay made another network check")
	}
	expirePublicDelay(t, s, id)
	checker.code = "ready"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- p.checkPublic(ctx, id) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if checker.calls.Load() != 2 {
		t.Fatal("concurrent readiness repeated transport")
	}
}
func TestPublicLateHoldAndCancellationDoNotCheckpoint(t *testing.T) {
	for _, fault := range []string{"hold", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			s, owner, id, p, checker := publicFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checker.after = func() {
				if fault == "cancel" {
					cancel()
					return
				}
				if _, err := s.Pool.Exec(context.Background(), `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
					t.Error(err)
				}
			}
			err := p.checkPublic(ctx, id)
			if fault == "hold" && !errors.Is(err, recovery.ErrHeld) {
				t.Fatal("late hold lost", err)
			}
			if fault == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("uncertain check checkpointed", err)
			}
			op, err := s.Get(context.Background(), owner, id)
			if err != nil || op.State != "checking" || op.Endpoint != "" {
				t.Fatal("uncertain public check activated", err)
			}
			var count int
			if err = s.Pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM managed_public_checks)+(SELECT count(*) FROM managed_public_readiness)`).Scan(&count); err != nil || count != 0 {
				t.Fatal("uncertain receipt consumed attempt", err)
			}
		})
	}
}
func TestPublicRetryPreservesFailureOnChangedPinQueueOrHold(t *testing.T) {
	for _, fault := range []string{"node-key", "queue", "late-hold", "unfinished"} {
		t.Run(fault, func(t *testing.T) {
			s, owner, id, p, checker := publicFixture(t)
			ctx := context.Background()
			publicFailure(t, s, id, p, checker)
			var node string
			if err := s.Pool.QueryRow(ctx, `SELECT node_id::text FROM instance_placements WHERE deployment_id=$1`, id).Scan(&node); err != nil {
				t.Fatal(err)
			}
			var sql string
			switch fault {
			case "node-key":
				sql = `UPDATE hosting_nodes SET public_key_hash=decode(repeat('ff',32),'hex')`
			case "queue":
				sql = `ALTER TABLE river_job ADD CONSTRAINT reject_public_retry CHECK(queue<>'staging_managed_deployments') NOT VALID`
			case "late-hold":
				sql = `CREATE FUNCTION public_retry_hold() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held'); RETURN NEW; END $$; CREATE TRIGGER public_retry_hold BEFORE INSERT ON river_job FOR EACH ROW EXECUTE FUNCTION public_retry_hold()`
			case "unfinished":
				sql = `UPDATE river_job SET state='available',finalized_at=NULL`
			}
			if _, err := s.Pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RetryManagedPublic(ctx, "operator", id, node, "cccccccc-cccc-4ccc-accc-cccccccccccc"); err == nil {
				t.Fatal("unsafe retry accepted")
			}
			op, err := s.Get(ctx, owner, id)
			if err != nil || op.State != "failed" || op.Endpoint != "" {
				t.Fatal("retry refusal lost checkpoint", err)
			}
			var cycle, count int
			if err = s.Pool.QueryRow(ctx, `SELECT cycle,(SELECT count(*) FROM managed_public_retries) FROM managed_public_readiness WHERE deployment_id=$1`, id).Scan(&cycle, &count); err != nil || cycle != 1 || count != 0 {
				t.Fatal("failed retry retained partial cycle", err)
			}
		})
	}
}
