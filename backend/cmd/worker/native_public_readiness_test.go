package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedcheck"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

func publicNativeScope(t *testing.T) (*pgxpool.Pool, string, nativeStagingScope, *nodeagent.Client, nodeagent.Peer) {
	t.Helper()
	if os.Getenv("PDS_QUALIFY_MANAGED_READINESS") != "true" {
		t.Skip("requires designated managed readiness qualification opt-in")
	}
	staging, err := config.LoadStagingProvisioning(os.Getenv("PDS_MODE"))
	if err != nil {
		t.Fatal(err)
	}
	managed, err := config.LoadStagingManagedRuntime(os.Getenv("PDS_MODE"), staging)
	if err != nil {
		t.Fatal(err)
	}
	public, err := config.LoadStagingManagedPublicChecks(os.Getenv("PDS_MODE"), staging, managed)
	if err != nil || !public {
		t.Fatal("explicit public worker opt-in required", err)
	}
	pool, root := nativeStagingPool(t)
	var scope nativeStagingScope
	if err = json.Unmarshal(nativeRead(t, filepath.Join(root, "scope.json")), &scope); err != nil {
		t.Fatal(err)
	}
	if scope.Deployment != "2395105c-9958-47b5-ae8b-5b0424cbfc4f" || scope.Node != "60684f10-c6b2-4833-a0b8-fb9f96a7869b" {
		t.Fatal("designated second fixture required")
	}
	client, err := nodeagent.NewClient(filepath.Join(root, "pki/ca.crt"), filepath.Join(root, "pki/worker.crt"), filepath.Join(root, "pki/worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	peer := nodeagent.Peer{NodeID: scope.Node}
	if err = pool.QueryRow(context.Background(), `SELECT agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1 AND state='active'`, scope.Node).Scan(&peer.Endpoint, &peer.PublicKeyHash); err != nil {
		t.Fatal(err)
	}
	return pool, filepath.Join(root, "managed-readiness"), scope, client, peer
}

func nativePublicCheckpoint(t *testing.T, pool *pgxpool.Pool, id string) (string, int, int, int, bool) {
	t.Helper()
	var state string
	var cycle, attempts, checks int
	var completed bool
	err := pool.QueryRow(context.Background(), `SELECT d.state,coalesce(c.cycle,0),coalesce(c.attempts,0),(SELECT count(*) FROM managed_public_checks WHERE deployment_id=d.id),NOT EXISTS(SELECT 1 FROM river_job WHERE args->>'deploymentId'=d.id::text AND state NOT IN ('completed','cancelled','discarded')) FROM deployments d LEFT JOIN managed_public_readiness c ON c.deployment_id=d.id WHERE d.id=$1`, id).Scan(&state, &cycle, &attempts, &checks, &completed)
	if err != nil {
		t.Fatal(err)
	}
	return state, cycle, attempts, checks, completed
}

func startPublicWorker(t *testing.T, pool *pgxpool.Pool, client *nodeagent.Client, checker managedcheck.Checker) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runStagingWorkerWithPublic(ctx, pool, client, true, checker) }()
	return cancel, done
}
func stopPublicWorker(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("qualification worker did not stop")
	}
}

func TestNativeManagedPublicBaseline(t *testing.T) {
	pool, root, scope, client, peer := publicNativeScope(t)
	if os.Getenv("SSL_CERT_FILE") != "" || os.Getenv("SSL_CERT_DIR") != "" {
		t.Fatal("baseline requires unchanged system trust")
	}
	state, cycle, attempts, checks, _ := nativePublicCheckpoint(t, pool, scope.Deployment)
	if state != "checking" || cycle != 0 || attempts != 0 || checks != 0 {
		t.Fatal("fresh unqualified public checkpoint required")
	}
	native, err := client.ManagedStatus(context.Background(), peer, scope.Deployment)
	if err != nil || !native.Healthy {
		t.Fatal("native fixture unhealthy", err)
	}
	result := managedcheck.New().Check(context.Background(), scope.Deployment)
	if result.Code != "ready" || result.Validate(scope.Deployment, time.Now()) != nil {
		t.Fatal("real public probe failed", result.Code)
	}
	nativeConversionWrite(t, filepath.Join(root, "baseline.json"), map[string]any{"deploymentId": scope.Deployment, "nativeHealthy": true, "publicObservation": result, "readinessPersisted": false})
}

func TestNativeManagedPublicTLSFailure(t *testing.T) {
	pool, root, scope, client, peer := publicNativeScope(t)
	if os.Getenv("SSL_CERT_FILE") != filepath.Join(root, "untrusted-ca.crt") || os.Getenv("SSL_CERT_DIR") != filepath.Join(root, "empty-ca-directory") {
		t.Fatal("only the private process-local trust failure is allowed")
	}
	if state, cycle, _, _, _ := nativePublicCheckpoint(t, pool, scope.Deployment); state != "checking" || cycle != 0 {
		t.Fatal("fresh public failure checkpoint required")
	}
	native, err := client.ManagedStatus(context.Background(), peer, scope.Deployment)
	if err != nil || !native.Healthy {
		t.Fatal("fault must preserve authenticated native trust", err)
	}
	result := managedcheck.New().Check(context.Background(), scope.Deployment)
	if result.Code != "tls" {
		t.Fatal("process-local public trust failure not confirmed", result.Code)
	}
	cancel, done := startPublicWorker(t, pool, client, managedcheck.New())
	stopped := false
	defer func() {
		if !stopped {
			stopPublicWorker(t, cancel, done)
		}
	}()
	deadline := time.Now().Add(2 * time.Minute)
	reconstructed := false
	for time.Now().Before(deadline) {
		state, cycle, attempts, checks, completed := nativePublicCheckpoint(t, pool, scope.Deployment)
		if state == "ready" {
			t.Fatal("untrusted public certificate activated endpoint")
		}
		if !reconstructed && cycle == 1 && attempts == 1 && checks == 1 {
			stopPublicWorker(t, cancel, done)
			stopped = true
			cancel, done = startPublicWorker(t, pool, client, managedcheck.New())
			stopped = false
			reconstructed = true
		}
		if state == "failed" && completed {
			if !reconstructed || cycle != 1 || attempts != 3 || checks != 3 {
				t.Fatal("bounded failure/reconstruction lost checkpoint")
			}
			stopPublicWorker(t, cancel, done)
			stopped = true
			var tlsChecks int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM managed_public_checks WHERE deployment_id=$1 AND code='tls'`, scope.Deployment).Scan(&tlsChecks); err != nil || tlsChecks != 3 {
				t.Fatal("failure codes changed", err)
			}
			var empty bool
			if err := pool.QueryRow(context.Background(), `SELECT endpoint_url IS NULL FROM deployments WHERE id=$1`, scope.Deployment).Scan(&empty); err != nil || !empty {
				t.Fatal("failure advertised URL", err)
			}
			nativeConversionWrite(t, filepath.Join(root, "tls-failure.json"), map[string]any{"deploymentId": scope.Deployment, "state": "failed", "cycle": cycle, "attempts": attempts, "tlsChecks": tlsChecks, "workerReconstructed": true, "servingURL": "", "fault": "process-local public trust only", "routeChanged": false})
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("live public failure did not reach its bounded checkpoint")
}

type interruptedPublicProbe struct {
	checker managedcheck.Checker
	success chan managedcheck.Result
}

func (p interruptedPublicProbe) Check(ctx context.Context, id string) managedcheck.Result {
	result := p.checker.Check(ctx, id)
	if result.Code != "ready" {
		return result
	}
	select {
	case p.success <- result:
	default:
	}
	<-ctx.Done()
	return result
}

func TestNativeManagedPublicReadyReplay(t *testing.T) {
	pool, root, scope, client, peer := publicNativeScope(t)
	if os.Getenv("SSL_CERT_FILE") != "" || os.Getenv("SSL_CERT_DIR") != "" {
		t.Fatal("readiness requires restored system trust")
	}
	state, cycle, attempts, checks, _ := nativePublicCheckpoint(t, pool, scope.Deployment)
	if state != "checking" || cycle != 2 || attempts != 0 || checks != 3 {
		t.Fatal("explicit retry of the retained failed cycle required")
	}
	interrupted := interruptedPublicProbe{checker: managedcheck.New(), success: make(chan managedcheck.Result, 1)}
	cancel, done := startPublicWorker(t, pool, client, interrupted)
	select {
	case <-interrupted.success:
	case err := <-done:
		cancel()
		t.Fatal("worker stopped before public proof", err)
	case <-time.After(time.Minute):
		stopPublicWorker(t, cancel, done)
		t.Fatal("real public success not observed")
	}
	stopPublicWorker(t, cancel, done)
	state, cycle, attempts, checks, _ = nativePublicCheckpoint(t, pool, scope.Deployment)
	if state != "checking" || cycle != 2 || attempts != 0 || checks != 3 {
		t.Fatal("uncertain public success checkpointed")
	}
	cancel, done = startPublicWorker(t, pool, client, managedcheck.New())
	defer stopPublicWorker(t, cancel, done)
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		state, cycle, attempts, checks, completed := nativePublicCheckpoint(t, pool, scope.Deployment)
		if state == "failed" {
			t.Fatal("restored public trust failed readiness")
		}
		if state == "ready" && completed {
			if cycle != 2 || attempts != 1 || checks != 4 {
				t.Fatal("reconstruction duplicated public proof")
			}
			store, err := deployment.New(pool)
			if err != nil {
				t.Fatal(err)
			}
			op, err := store.Get(context.Background(), scope.Owner, scope.Deployment)
			if err != nil || op.Endpoint != "https://"+op.ManagedHostname {
				t.Fatal("ready endpoint not retained", err)
			}
			capacity, err := client.Capacity(context.Background(), peer)
			if err != nil || len(capacity.InstanceIDs) != 1 || capacity.InstanceIDs[0] != scope.Deployment {
				t.Fatal("public retry changed native namespace", err)
			}
			usage, err := client.Usage(context.Background(), peer, scope.Deployment)
			if err != nil || usage.Accounts != 0 {
				t.Fatal("public readiness enabled accounts", err)
			}
			nativeConversionWrite(t, filepath.Join(root, "ready-replay.json"), map[string]any{"deploymentId": scope.Deployment, "nodeId": scope.Node, "state": "ready", "endpoint": op.Endpoint, "cycle": cycle, "attempts": attempts, "retainedChecks": checks, "workerReconstructedAfterUncertainSuccess": true, "nativeNamespaces": capacity.InstanceIDs, "accounts": usage.Accounts, "publicReadinessPersisted": true})
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("public readiness did not complete after reconstruction")
}
