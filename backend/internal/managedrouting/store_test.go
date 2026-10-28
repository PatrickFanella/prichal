package managedrouting

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const requestOne = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const requestTwo = "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"

type inspectorFixture struct {
	value    nodeagent.IngressCertificate
	calls    atomic.Int64
	accounts int64
	entered  chan struct{}
	release  chan struct{}
}

func (i *inspectorFixture) IngressCertificate(ctx context.Context, p nodeagent.Peer, id string) (nodeagent.IngressCertificate, error) {
	i.calls.Add(1)
	if i.entered != nil {
		close(i.entered)
		select {
		case <-i.release:
		case <-ctx.Done():
			return nodeagent.IngressCertificate{}, ctx.Err()
		}
	}
	return i.value, nil
}
func (i *inspectorFixture) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: i.value.InstanceID, Endpoint: i.value.Endpoint, State: "running", Healthy: true}, nil
}
func (i *inspectorFixture) Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error) {
	return nodeagent.Usage{InstanceID: i.value.InstanceID, Accounts: i.accounts, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}, nil
}

type storeFixture struct {
	Store
	id, node  string
	inspector *inspectorFixture
}

func routingFixture(t *testing.T) storeFixture {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('fixture','routing','Routing') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Routing',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) SELECT id,$1,'routing','local','routing','agent','queued',id::text||'.localhost','https://'||id::text||'.localhost:44444' FROM (SELECT gen_random_uuid() id) identity RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',1,1,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE deployments SET endpoint_url=$2 WHERE id=$1`, id, nodeagent.LocalEndpoint(id)); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{id + ".localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	sum := sha256.Sum256(data)
	return storeFixture{Store: Store{Pool: pool}, id: id, node: node, inspector: &inspectorFixture{value: nodeagent.IngressCertificate{InstanceID: id, Endpoint: nodeagent.LocalEndpoint(id), Certificate: string(data), SHA256: hex.EncodeToString(sum[:])}}}
}
func (f storeFixture) place(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.Pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2);`, f.id, f.node); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pool.Exec(ctx, `UPDATE deployments SET state='ready' WHERE id=$1`, f.id); err != nil {
		t.Fatal(err)
	}
}
func TestRetainedEndpointObservationReplayAndPrivateBundle(t *testing.T) {
	f := routingFixture(t)
	ctx := context.Background()
	endpoint, err := f.Reserve(ctx, "operator", f.id)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.ForwardPort != 35000 || endpoint.Basis != "queued" {
		t.Fatal(endpoint)
	}
	again, err := f.Reserve(ctx, "operator", f.id)
	if err != nil || !reflect.DeepEqual(again, endpoint) {
		t.Fatal("reservation replay changed identity", err)
	}
	if _, err := f.Export(ctx); !errors.Is(err, ErrUnqualified) {
		t.Fatal("unplaced intent exported", err)
	}
	f.place(t)
	before, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector)
	if err != nil || before != after || f.inspector.calls.Load() != 1 {
		t.Fatal("observation replay changed receipt", err)
	}
	if _, err := f.Observe(ctx, "different-operator", f.id, f.node, requestOne, f.inspector); !errors.Is(err, ErrConflict) {
		t.Fatal("replay actor changed", err)
	}
	bundle, err := f.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Inventory.Generation != before.Sequence || len(bundle.Inventory.Endpoints) != 1 || len(bundle.Inventory.Handles) != 0 {
		t.Fatal("unexpected authoritative inventory")
	}
	directory := filepath.Join(t.TempDir(), "bundle")
	if err := WriteBundle(directory, bundle); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteBundle(directory, bundle); err == nil {
		t.Fatal("retained bundle replaced")
	}
	afterData, err := os.ReadFile(filepath.Join(directory, "inventory.json"))
	if err != nil || string(afterData) != string(data) {
		t.Fatal("retained manifest changed")
	}
	if _, err := managededge.Render(bundle.Inventory, filepath.Join(directory, "certificates"), time.Now()); err != nil {
		t.Fatal("export does not satisfy candidate contract", err)
	}
}
func TestStaleOrChangedPlacementCannotExportRetainedCertificate(t *testing.T) {
	for _, change := range []string{"missing", "node-key", "disabled", "stale", "new-node", "latest-invalid"} {
		t.Run(change, func(t *testing.T) {
			f := routingFixture(t)
			ctx := context.Background()
			if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
				t.Fatal(err)
			}
			f.place(t)
			if _, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector); err != nil {
				t.Fatal(err)
			}
			var err error
			switch change {
			case "missing":
				_, err = f.Pool.Exec(ctx, `DELETE FROM instance_placements WHERE deployment_id=$1`, f.id)
			case "node-key":
				_, err = f.Pool.Exec(ctx, `UPDATE hosting_nodes SET public_key_hash=$2 WHERE id=$1`, f.node, make([]byte, 31))
			case "disabled":
				_, err = f.Pool.Exec(ctx, `UPDATE hosting_nodes SET state='disabled' WHERE id=$1`, f.node)
			case "stale":
				_, err = f.Pool.Exec(ctx, `UPDATE managed_edge_observations SET observed_at=now()-interval '16 minutes' WHERE deployment_id=$1`, f.id)
			case "new-node":
				var node string
				err = f.Pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9092',1,1,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node)
				if err == nil {
					_, err = f.Pool.Exec(ctx, `UPDATE instance_placements SET node_id=$2 WHERE deployment_id=$1`, f.id, node)
				}
			case "latest-invalid":
				_, err = f.Pool.Exec(ctx, `INSERT INTO managed_edge_observations(deployment_id,request_key,node_id,node_key_hash,certificate_pem,certificate_sha256,actor) SELECT deployment_id,$2,node_id,node_key_hash,'invalid'::bytea,certificate_sha256,actor FROM managed_edge_observations WHERE deployment_id=$1`, f.id, requestTwo)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Export(ctx); !errors.Is(err, ErrUnqualified) {
				t.Fatal("stale binding exported", err)
			}
			retained, err := f.Reserve(ctx, "operator", f.id)
			if err != nil || retained.DeploymentID != f.id || retained.ForwardPort != 35000 {
				t.Fatal("placement changed public reservation", err)
			}
		})
	}
}
func TestEmptyFixtureRequiresAuthenticatedZeroAccounts(t *testing.T) {
	f := routingFixture(t)
	ctx := context.Background()
	f.place(t)
	if _, err := f.Reserve(ctx, "operator", f.id); !errors.Is(err, ErrConflict) {
		t.Fatal("ready endpoint silently reserved", err)
	}
	f.inspector.accounts = 1
	if _, err := f.ReserveEmptyFixture(ctx, "operator", f.id, f.inspector); !errors.Is(err, ErrUnqualified) {
		t.Fatal("populated fixture accepted", err)
	}
	f.inspector.accounts = 0
	endpoint, err := f.ReserveEmptyFixture(ctx, "operator", f.id, f.inspector)
	if err != nil || endpoint.Basis != "empty_fixture" {
		t.Fatal("empty fixture reservation failed", err)
	}
	var hostname, endpointURL string
	if err := f.Pool.QueryRow(ctx, `SELECT managed_hostname,endpoint_url FROM deployments WHERE id=$1`, f.id).Scan(&hostname, &endpointURL); err != nil {
		t.Fatal(err)
	}
	if hostname != f.id+".localhost" || endpointURL != nodeagent.LocalEndpoint(f.id) {
		t.Fatal("reservation renamed native endpoint")
	}
}
func TestConcurrentObservationAndLateRecoveryHold(t *testing.T) {
	f := routingFixture(t)
	ctx := context.Background()
	if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
		t.Fatal(err)
	}
	f.place(t)
	var wg sync.WaitGroup
	results := make(chan Observation, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector)
			results <- o
			errs <- err
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
	var sequence int64
	for o := range results {
		if sequence != 0 && sequence != o.Sequence {
			t.Fatal("concurrent replay duplicated receipt")
		}
		sequence = o.Sequence
	}
	if f.inspector.calls.Load() != 1 {
		t.Fatal("concurrent replay repeated transport")
	}
	f.inspector.entered = make(chan struct{})
	f.inspector.release = make(chan struct{})
	done := make(chan error, 1)
	go func() { _, err := f.Observe(ctx, "operator", f.id, f.node, requestTwo, f.inspector); done <- err }()
	<-f.inspector.entered
	if _, err := f.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	close(f.inspector.release)
	if err := <-done; !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("late recovery hold ignored", err)
	}
	var count int
	if err := f.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_edge_observations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("late hold committed observation", err)
	}
	if _, err := f.Export(ctx); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held inventory exported", err)
	}
}

func TestBundleCannotWriteUnboundCertificateNames(t *testing.T) {
	f := routingFixture(t)
	ctx := context.Background()
	if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
		t.Fatal(err)
	}
	f.place(t)
	if _, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector); err != nil {
		t.Fatal(err)
	}
	bundle, err := f.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Inventory.Endpoints = append(bundle.Inventory.Endpoints, bundle.Inventory.Endpoints[0])
	bundle.Certificates["../../escape"] = bundle.Certificates[f.id]
	directory := filepath.Join(t.TempDir(), "refused-bundle")
	if err := WriteBundle(directory, bundle); err == nil {
		t.Fatal("unbound certificate filename accepted")
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid bundle created filesystem output")
	}
}
