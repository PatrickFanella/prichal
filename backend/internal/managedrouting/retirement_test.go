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
	"math/big"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

type retirementAgent struct {
	*inspectorFixture
	ids           []string
	afterUsage    func()
	afterCapacity func()
}

func (a retirementAgent) ManagedStatus(ctx context.Context, p nodeagent.Peer, id string) (nodeagent.Instance, error) {
	return a.Status(ctx, p, id)
}
func (a retirementAgent) Usage(ctx context.Context, p nodeagent.Peer, id string) (nodeagent.Usage, error) {
	u, e := a.inspectorFixture.Usage(ctx, p, id)
	if a.afterUsage != nil {
		a.afterUsage()
	}
	return u, e
}
func (a retirementAgent) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	if a.afterCapacity != nil {
		a.afterCapacity()
	}
	return nodeagent.Capacity{MaximumSlots: 1, CPUs: 3, InstanceIDs: a.ids}, nil
}

func TestRetirementReleaseRefusesChangedKeyAndLateRecoveryHold(t *testing.T) {
	for _, change := range []string{"node-key", "late-hold"} {
		t.Run(change, func(t *testing.T) {
			f, request, a := retirementFixture(t)
			ctx := context.Background()
			if _, e := f.PrepareRetirement(ctx, "operator", request, a); e != nil {
				t.Fatal(e)
			}
			a.ids = nil
			if change == "node-key" {
				if _, e := f.Pool.Exec(ctx, `UPDATE hosting_nodes SET public_key_hash=decode(repeat('01',32),'hex') WHERE id=$1`, f.node); e != nil {
					t.Fatal(e)
				}
			} else {
				a.afterCapacity = func() {
					if _, e := f.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); e != nil {
						t.Fatal(e)
					}
				}
			}
			_, e := f.ReleaseRetirement(ctx, "operator", f.id, request.Key, strings.Repeat("c", 64), strings.Repeat("d", 64), a)
			if e == nil {
				t.Fatal("changed authority released a slot")
			}
			var phase string
			var placements int
			if e = f.Pool.QueryRow(ctx, `SELECT phase,(SELECT count(*) FROM instance_placements WHERE deployment_id=$1) FROM managed_endpoint_retirements WHERE deployment_id=$1`, f.id).Scan(&phase, &placements); e != nil || phase != "prepared" || placements != 1 {
				t.Fatal("refusal changed retirement or placement", e)
			}
		})
	}
}
func retirementFixture(t *testing.T) (storeFixture, RetirementRequest, retirementAgent) {
	t.Helper()
	f, b, _ := forwardingFixture(t)
	migrator, e := rivermigrate.New(riverpgxv5.New(f.Pool), nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = migrator.Migrate(context.Background(), rivermigrate.DirectionUp, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := f.Pool.Exec(context.Background(), `UPDATE deployments SET managed_hostname=$2,endpoint_url='https://'||$2 WHERE id=$1`, f.id, b.Inventory.Endpoints[0].Hostname); e != nil {
		t.Fatal(e)
	}
	return f, RetirementRequest{DeploymentID: f.id, NodeID: f.node, Key: requestTwo, NativeBackupSHA256: strings.Repeat("a", 64), ManagementBackupSHA256: strings.Repeat("b", 64)}, retirementAgent{inspectorFixture: f.inspector, ids: []string{f.id}}
}
func TestRetirementKeepsSlotUntilAuthenticatedAbsenceAndPreservesHistory(t *testing.T) {
	f, request, a := retirementFixture(t)
	ctx := context.Background()
	r, e := f.PrepareRetirement(ctx, "operator", request, a)
	if e != nil || r.Phase != "prepared" {
		t.Fatal(r, e)
	}
	again, e := f.PrepareRetirement(ctx, "operator", request, a)
	if e != nil || again != r {
		t.Fatal("replay changed retirement", e)
	}
	var state, url string
	var count int
	if e = f.Pool.QueryRow(ctx, `SELECT state,coalesce(endpoint_url,''),(SELECT count(*) FROM instance_placements WHERE deployment_id=$1) FROM deployments WHERE id=$1`, f.id).Scan(&state, &url, &count); e != nil || state != "failed" || url != "" || count != 1 {
		t.Fatal("preparation released or advertised serving", e)
	}
	if _, e = f.Export(ctx); !errors.Is(e, ErrUnqualified) {
		t.Fatal("prepared retirement exported", e)
	}
	tx, e := f.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = CheckActiveTransaction(ctx, tx, f.id); !errors.Is(e, ErrConflict) {
		t.Fatal("retired reconciliation allowed", e)
	}
	tx.Rollback(ctx)
	w, n := strings.Repeat("c", 64), strings.Repeat("d", 64)
	if _, e = f.ReleaseRetirement(ctx, "operator", f.id, request.Key, w, n, a); !errors.Is(e, ErrUnqualified) {
		t.Fatal("retained namespace released", e)
	}
	a.ids = nil
	released, e := f.ReleaseRetirement(ctx, "operator", f.id, request.Key, w, n, a)
	if e != nil || released.Phase != "released" {
		t.Fatal(released, e)
	}
	if _, e = f.ReleaseRetirement(ctx, "operator", f.id, request.Key, w, n, a); e != nil {
		t.Fatal("release replay failed", e)
	}
	if _, e = f.ReleaseRetirement(ctx, "operator", f.id, request.Key, n, w, a); !errors.Is(e, ErrConflict) {
		t.Fatal("replay replaced custody receipts", e)
	}
	if e = f.Pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE deployment_id=$1`, f.id).Scan(&count); e != nil || count != 0 {
		t.Fatal("placement retained", e)
	}
	if e = f.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_edge_observations WHERE deployment_id=$1`, f.id).Scan(&count); e != nil || count != 1 {
		t.Fatal("observation history lost", e)
	}
	retained, e := f.Reserve(ctx, "operator", f.id)
	if e != nil || retained.ForwardPort != 35000 {
		t.Fatal("reservation lost", e)
	}
	// A new, qualified endpoint exports even while retired identity/history remain reserved.
	var other string
	if e = f.Pool.QueryRow(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname) SELECT fresh.id,d.organization_id,'new','local','new','agent','queued',fresh.id::text||'.localhost' FROM deployments d CROSS JOIN(SELECT gen_random_uuid()id)fresh WHERE d.id=$1 RETURNING deployments.id`, f.id).Scan(&other); e != nil {
		t.Fatal(e)
	}
	endpoint, e := f.Reserve(ctx, "operator", other)
	if e != nil || endpoint.ForwardPort != 35001 {
		t.Fatal("retired forward port reused", e)
	}
	// An unqualified new endpoint must still refuse a partial export.
	if _, e = f.Export(ctx); !errors.Is(e, ErrUnqualified) {
		t.Fatal("partial inventory exported", e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{other + ".localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	hash := sha256.Sum256(certificate)
	if _, e = f.Pool.Exec(ctx, `UPDATE deployments SET state='ready',managed_hostname=$2 WHERE id=$1`, other, endpoint.Hostname); e != nil {
		t.Fatal(e)
	}
	if _, e = f.Pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, other, f.node); e != nil {
		t.Fatal(e)
	}
	inspector := &inspectorFixture{value: nodeagent.IngressCertificate{InstanceID: other, Endpoint: "https://" + endpoint.Hostname, Certificate: string(certificate), SHA256: hex.EncodeToString(hash[:])}}
	if _, e = f.Observe(ctx, "operator", other, f.node, requestOne, inspector); e != nil {
		t.Fatal(e)
	}
	bundle, e := f.Export(ctx)
	if e != nil || len(bundle.Inventory.Endpoints) != 1 || bundle.Inventory.Endpoints[0].DeploymentID != other {
		t.Fatal("released history blocked fresh complete export", e)
	}
}
func TestRetirementRefusesPopulatedChangedAndLateHeldAuthority(t *testing.T) {
	for _, change := range []string{"populated", "node", "digest", "late-hold"} {
		t.Run(change, func(t *testing.T) {
			f, request, a := retirementFixture(t)
			ctx := context.Background()
			switch change {
			case "populated":
				a.accounts = 1
			case "node":
				request.NodeID = requestOne
			case "digest":
				request.NativeBackupSHA256 = "untrusted"
			case "late-hold":
				a.afterUsage = func() {
					if _, e := f.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); e != nil {
						t.Fatal(e)
					}
				}
			}
			if _, e := f.PrepareRetirement(ctx, "operator", request, a); e == nil {
				t.Fatal("unsafe retirement accepted")
			} else if change == "late-hold" && !errors.Is(e, recovery.ErrHeld) {
				t.Fatal(e)
			}
			var count int
			if e := f.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_endpoint_retirements`).Scan(&count); e != nil || count != 0 {
				t.Fatal("refusal retained retirement", e)
			}
			var state string
			if e := f.Pool.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1`, f.id).Scan(&state); e != nil || state != "ready" {
				t.Fatal("refusal changed deployment", e)
			}
		})
	}
}
