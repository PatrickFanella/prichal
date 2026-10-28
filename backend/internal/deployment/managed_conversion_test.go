package deployment

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

type conversionInspector struct {
	id, host    string
	certificate nodeagent.IngressCertificate
	accounts    int64
	calls       atomic.Int64
	onStatus    func()
}

func (i *conversionInspector) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: i.id, Endpoint: nodeagent.LocalEndpoint(i.id), State: "running", Healthy: true}, nil
}
func (i *conversionInspector) ManagedStatus(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	i.calls.Add(1)
	if i.onStatus != nil {
		i.onStatus()
	}
	return nodeagent.Instance{ID: i.id, Endpoint: "https://" + i.host, State: "running", Healthy: true}, nil
}
func (i *conversionInspector) Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error) {
	return nodeagent.Usage{InstanceID: i.id, Accounts: i.accounts, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}, nil
}
func (i *conversionInspector) IngressCertificate(context.Context, nodeagent.Peer, string) (nodeagent.IngressCertificate, error) {
	return i.certificate, nil
}

func conversionFixture(t *testing.T) (*Store, string, string, string, *conversionInspector, EmptyConversionRequest) {
	t.Helper()
	s, owner, org := managedIntentFixture(t)
	s.ManagedIntents = false
	ctx := context.Background()
	op, err := s.Submit(ctx, owner, org, Request{Kind: "agent", Name: "private", Region: "local", Key: "private"})
	if err != nil {
		t.Fatal(err)
	}
	var node string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',1,1,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2);`, op.ID, node); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE deployments SET state='ready' WHERE id=$1;`, op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE args->>'deploymentId'=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{op.ID + ".localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	hash := sha256.Sum256(data)
	certHash := hex.EncodeToString(hash[:])
	host, _ := managednamespace.Endpoint(op.ID)
	i := &conversionInspector{id: op.ID, host: host, certificate: nodeagent.IngressCertificate{InstanceID: op.ID, Endpoint: nodeagent.LocalEndpoint(op.ID), Certificate: string(data), SHA256: certHash}}
	routes := managedrouting.Store{Pool: s.Pool}
	if _, err := routes.ReserveEmptyFixture(ctx, "operator", op.ID, i); err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Observe(ctx, "operator", op.ID, node, "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", i); err != nil {
		t.Fatal(err)
	}
	i.certificate.Endpoint = "https://" + host
	r := EmptyConversionRequest{Key: "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb", NativeBackupSHA256: strings.Repeat("a", 64), ManagementBackupSHA256: strings.Repeat("b", 64), OperatorReceiptSHA256: strings.Repeat("c", 64), CredentialsSHA256: strings.Repeat("d", 64), CertificateSHA256: certHash, NativeReceiptSHA256: strings.Repeat("e", 64)}
	return s, owner, op.ID, node, i, r
}

func TestEmptyConversionConfirmationAtomicReplay(t *testing.T) {
	s, owner, id, node, i, r := conversionFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan EmptyConversion, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.ConfirmEmptyManagedConversion(ctx, "operator", id, node, r, i)
			results <- v
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
	var first EmptyConversion
	for result := range results {
		if first.DeploymentID != "" && result != first {
			t.Fatal("replay changed conversion receipt")
		}
		first = result
	}
	if i.calls.Load() != 1 {
		t.Fatal("concurrent replay repeated native confirmation")
	}
	op, err := s.Get(ctx, owner, id)
	if err != nil || op.State != "configuring" || op.Endpoint != "" || op.ManagedHostname != i.host {
		t.Fatal("conversion advertised serving or lost identity", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE queue=$1 AND args->>'deploymentId'=$2`, ManagedIntentQueue, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("conversion replay duplicated job", err)
	}
	r.NativeBackupSHA256 = strings.Repeat("f", 64)
	if _, err := s.ConfirmEmptyManagedConversion(ctx, "operator", id, node, r, i); !errors.Is(err, ErrConflict) {
		t.Fatal("changed custody accepted", err)
	}
	a := &managedRemote{ids: map[string]bool{}, healthy: true}
	if err := (AgentProvisioner{Pool: s.Pool, Client: a, ManagedRuntime: true}).Step(ctx, id); err != nil {
		t.Fatal("recorded conversion did not permit managed reconciliation", err)
	}
}

func TestEmptyConversionRefusesAccountKeyAndLateHold(t *testing.T) {
	for _, change := range []string{"accounts", "node-key", "job-insert", "late-hold", "unfinished-job"} {
		t.Run(change, func(t *testing.T) {
			s, owner, id, node, i, r := conversionFixture(t)
			ctx := context.Background()
			switch change {
			case "accounts":
				i.accounts = 1
			case "node-key":
				if _, err := s.Pool.Exec(ctx, `UPDATE hosting_nodes SET public_key_hash=$2 WHERE id=$1`, node, []byte(strings.Repeat("x", 32))); err != nil {
					t.Fatal(err)
				}
			case "job-insert":
				if _, err := s.Pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT refuse_conversion CHECK(queue<>'staging_managed_deployments') NOT VALID`); err != nil {
					t.Fatal(err)
				}
			case "unfinished-job":
				if _, err := s.Pool.Exec(ctx, `UPDATE river_job SET state='available',finalized_at=NULL`); err != nil {
					t.Fatal(err)
				}
			case "late-hold":
				i.onStatus = func() {
					if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
						t.Error(err)
					}
				}
			}
			_, err := s.ConfirmEmptyManagedConversion(ctx, "operator", id, node, r, i)
			if err == nil {
				t.Fatal("unsafe conversion confirmed")
			}
			if change == "late-hold" && !errors.Is(err, recovery.ErrHeld) {
				t.Fatal("late hold lost", err)
			}
			op, err := s.Get(ctx, owner, id)
			if err != nil || op.State != "ready" || op.ManagedHostname != id+".localhost" || op.Endpoint != nodeagent.LocalEndpoint(id) {
				t.Fatal("failed conversion changed private management intent", err)
			}
			var count int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_native_conversions`).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed conversion retained authority", err)
			}
		})
	}
}
