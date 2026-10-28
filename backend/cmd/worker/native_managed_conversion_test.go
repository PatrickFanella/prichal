package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

func managedNativeScope(t *testing.T) (*pgxpool.Pool, string, nativeStagingScope, *nodeagent.Client, nodeagent.Peer) {
	t.Helper()
	if os.Getenv("PDS_QUALIFY_MANAGED_CONVERSION") != "true" {
		t.Skip("requires designated empty staging-02 managed conversion opt-in")
	}
	pool, root := nativeStagingPool(t)
	var scope nativeStagingScope
	if err := json.Unmarshal(nativeRead(t, filepath.Join(root, "scope.json")), &scope); err != nil {
		t.Fatal(err)
	}
	if scope.Node != "60684f10-c6b2-4833-a0b8-fb9f96a7869b" || scope.Deployment != "2395105c-9958-47b5-ae8b-5b0424cbfc4f" {
		t.Fatal("fixed second staging node/deployment required")
	}
	client, err := nodeagent.NewClient(filepath.Join(root, "pki/ca.crt"), filepath.Join(root, "pki/worker.crt"), filepath.Join(root, "pki/worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	peer := nodeagent.Peer{NodeID: scope.Node}
	if err := pool.QueryRow(context.Background(), `SELECT agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1 AND state='active'`, scope.Node).Scan(&peer.Endpoint, &peer.PublicKeyHash); err != nil {
		t.Fatal(err)
	}
	return pool, root, scope, client, peer
}

func nativeConversionWrite(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeManagedConversionPrivateBaseline(t *testing.T) {
	pool, root, scope, client, peer := managedNativeScope(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	phase := os.Getenv("PDS_MANAGED_CONVERSION_PHASE")
	if phase == "" {
		phase = "baseline"
	}
	if phase != "baseline" && phase != "after-abort" {
		t.Fatal("known private verification phase required")
	}
	var state, hostname string
	var confirmed bool
	if err := pool.QueryRow(ctx, `SELECT state,managed_hostname,EXISTS(SELECT 1 FROM managed_native_conversions WHERE deployment_id=$1) FROM deployments WHERE id=$1`, scope.Deployment).Scan(&state, &hostname, &confirmed); err != nil {
		t.Fatal(err)
	}
	if state != "ready" || hostname != scope.Deployment+".localhost" || confirmed {
		t.Fatal("unconfirmed retained private management fixture required")
	}
	status, err := client.Status(ctx, peer, scope.Deployment)
	if err != nil || !status.Healthy || status.Endpoint != nodeagent.LocalEndpoint(scope.Deployment) {
		t.Fatal("private identity is not healthy", err)
	}
	usage, err := client.Usage(ctx, peer, scope.Deployment)
	if err != nil || usage.Accounts != 0 {
		t.Fatal("private fixture is not verifiably empty", err)
	}
	certificate, err := client.IngressCertificate(ctx, peer, scope.Deployment)
	if err != nil || certificate.Endpoint != status.Endpoint {
		t.Fatal("private certificate identity unavailable", err)
	}
	capacity, err := client.Capacity(ctx, peer)
	if err != nil || capacity.MaximumSlots != 1 || capacity.CPUs != 3 || len(capacity.InstanceIDs) != 1 || capacity.InstanceIDs[0] != scope.Deployment {
		t.Fatal("designated node capacity changed", err)
	}
	nativeConversionWrite(t, filepath.Join(root, "managed-conversion", "private-"+phase+".json"), map[string]any{"completedAt": time.Now().UTC(), "deploymentId": scope.Deployment, "nodeId": scope.Node, "endpoint": status.Endpoint, "certificateSha256": certificate.SHA256, "accounts": usage.Accounts, "dataBytes": usage.DataBytes, "blobBytes": usage.BlobBytes, "nativeNamespaces": capacity.InstanceIDs, "publicReady": false})
}

func TestNativeManagedConversionAbortReadback(t *testing.T) {
	pool, root, scope, client, peer := managedNativeScope(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var confirmed bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM managed_native_conversions WHERE deployment_id=$1)`, scope.Deployment).Scan(&confirmed); err != nil || confirmed {
		t.Fatal("pre-create abort requires unconfirmed management identity", err)
	}
	status, err := client.Status(ctx, peer, scope.Deployment)
	if err != nil || status.State != "starting" || status.Healthy || status.Endpoint != nodeagent.LocalEndpoint(scope.Deployment) {
		t.Fatal("pre-create abort did not restore stopped private policy", err)
	}
	nativeConversionWrite(t, filepath.Join(root, "managed-conversion", "precreate-abort.json"), map[string]any{"deploymentId": scope.Deployment, "endpoint": status.Endpoint, "state": status.State, "healthy": false, "privatePolicyRestored": true, "nativeRestarted": false, "publicReady": false})
}

func TestNativeManagedConversionAgentActivation(t *testing.T) {
	pool, root, scope, client, peer := managedNativeScope(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var state, hostname string
	var confirmed bool
	if err := pool.QueryRow(ctx, `SELECT state,managed_hostname,EXISTS(SELECT 1 FROM managed_native_conversions WHERE deployment_id=$1) FROM deployments WHERE id=$1`, scope.Deployment).Scan(&state, &hostname, &confirmed); err != nil {
		t.Fatal(err)
	}
	if state != "ready" || hostname != scope.Deployment+".localhost" || confirmed {
		t.Fatal("activation requires the unconfirmed retained private management checkpoint")
	}
	result, err := client.EnsureManaged(ctx, peer, scope.Deployment)
	if err != nil {
		t.Fatal("restricted managed reconciliation failed", err)
	}
	host, _ := managednamespace.Endpoint(scope.Deployment)
	if result.Endpoint != "https://"+host {
		t.Fatal("managed endpoint mismatch")
	}
	for {
		result, err = client.ManagedStatus(ctx, peer, scope.Deployment)
		if err == nil && result.Healthy && result.State == "running" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("managed native health unavailable", err)
		case <-time.After(200 * time.Millisecond):
		}
	}
	usage, err := client.Usage(ctx, peer, scope.Deployment)
	if err != nil || usage.Accounts != 0 {
		t.Fatal("empty managed native usage unavailable", err)
	}
	certificate, err := client.IngressCertificate(ctx, peer, scope.Deployment)
	if err != nil || certificate.Endpoint != result.Endpoint {
		t.Fatal("managed certificate readback failed", err)
	}
	var oldCertificate []byte
	if err := pool.QueryRow(ctx, `SELECT certificate_sha256 FROM managed_edge_observations WHERE deployment_id=$1 ORDER BY sequence DESC LIMIT 1`, scope.Deployment).Scan(&oldCertificate); err != nil {
		t.Fatal(err)
	}
	if certificate.SHA256 != hex.EncodeToString(oldCertificate) {
		t.Fatal("conversion replaced ingress identity")
	}
	capacity, err := client.Capacity(ctx, peer)
	if err != nil || capacity.MaximumSlots != 1 || capacity.CPUs != 3 || len(capacity.InstanceIDs) != 1 || capacity.InstanceIDs[0] != scope.Deployment {
		t.Fatal("conversion changed authorized capacity/namespace", err)
	}
	path := filepath.Join(root, "managed-conversion", "agent-activation.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("retained activation report cannot be represented as a fresh run")
	}
	nativeConversionWrite(t, path, map[string]any{"deploymentId": scope.Deployment, "nodeId": scope.Node, "endpoint": result.Endpoint, "certificateSha256": certificate.SHA256, "accounts": usage.Accounts, "namespaces": capacity.InstanceIDs, "publicReady": false})
}

type interruptedManagedConversionAgent struct {
	*nodeagent.Client
	success chan nodeagent.Instance
}

func (a interruptedManagedConversionAgent) EnsureManaged(ctx context.Context, peer nodeagent.Peer, id string) (nodeagent.Instance, error) {
	result, err := a.Client.EnsureManaged(ctx, peer, id)
	if err != nil {
		return result, err
	}
	select {
	case a.success <- result:
	default:
	}
	<-ctx.Done()
	return nodeagent.Instance{}, ctx.Err()
}

func TestNativeManagedConversionWorkerReplay(t *testing.T) {
	pool, root, scope, client, peer := managedNativeScope(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	store, err := deployment.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	store.Staging = true
	before, err := store.Get(ctx, scope.Owner, scope.Deployment)
	if err != nil || before.State != "configuring" || before.Endpoint != "" {
		t.Fatal("fresh managed replay requires the confirmed configuring checkpoint", err)
	}
	path := filepath.Join(root, "managed-conversion", "worker-replay.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("retained replay cannot be represented as another fresh interruption")
	}
	interrupted := interruptedManagedConversionAgent{Client: client, success: make(chan nodeagent.Instance, 1)}
	firstCtx, stopFirst := context.WithCancel(ctx)
	firstDone := make(chan error, 1)
	go func() { firstDone <- runStagingWorker(firstCtx, pool, interrupted, true) }()
	defer stopFirst()
	select {
	case <-interrupted.success:
	case err := <-firstDone:
		t.Fatal("initial managed worker exited", err)
	case <-ctx.Done():
		t.Fatal("managed reconciliation was not observed")
	}
	stopFirst()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("interrupted managed worker did not stop")
	}
	checkpoint, err := store.Get(ctx, scope.Owner, scope.Deployment)
	if err != nil || checkpoint.State != "configuring" || checkpoint.Endpoint != "" || len(checkpoint.Events) != len(before.Events) {
		t.Fatal("uncertain native completion advanced checkpoint", err)
	}
	secondCtx, stopSecond := context.WithCancel(ctx)
	secondDone := make(chan error, 1)
	go func() { secondDone <- runStagingWorker(secondCtx, pool, client, true) }()
	defer func() {
		stopSecond()
		if err := <-secondDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	for {
		op, err := store.Get(ctx, scope.Owner, scope.Deployment)
		if err != nil {
			t.Fatal(err)
		}
		var pending bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM river_job WHERE queue=$1 AND args->>'deploymentId'=$2 AND state='scheduled' AND scheduled_at>now()+interval '20 seconds')`, deployment.ManagedIntentQueue, scope.Deployment).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if op.State == "checking" && pending {
			if op.Endpoint != "" || len(op.Events) != len(before.Events)+1 {
				t.Fatal("managed worker advertised public readiness or duplicated events")
			}
			break
		}
		if op.State == "failed" || op.State == "ready" {
			t.Fatal("managed worker left its pending public boundary", op.State)
		}
		select {
		case <-ctx.Done():
			t.Fatal("managed worker reconstruction did not reach pending checking")
		case <-time.After(200 * time.Millisecond):
		}
	}
	capacity, err := client.Capacity(ctx, peer)
	if err != nil || len(capacity.InstanceIDs) != 1 || capacity.InstanceIDs[0] != scope.Deployment {
		t.Fatal("managed reconstruction duplicated namespace", err)
	}
	var placements int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE deployment_id=$1 AND node_id=$2`, scope.Deployment, scope.Node).Scan(&placements); err != nil || placements != 1 {
		t.Fatal("managed reconstruction changed placement", err)
	}
	nativeConversionWrite(t, path, map[string]any{"deploymentId": scope.Deployment, "nodeId": scope.Node, "uncertainCompletionInjected": true, "workerReconstructed": true, "state": "checking", "servingUrl": "", "namespaces": capacity.InstanceIDs, "placementCount": placements, "publicReady": false, "nativeCreation": "restricted agent under operator conversion"})
}
