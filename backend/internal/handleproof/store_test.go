package handleproof

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"net"
	"testing"
)

func newStoreFixture(t *testing.T) (*Store, string, string, string) {
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, bob, org, deploy string
	for _, entry := range []struct {
		subject string
		id      *string
	}{{"alice", &owner}, {"bob", &bob}} {
		if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local',$1,$1) RETURNING id::text`, entry.subject).Scan(entry.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Handle test',$1) RETURNING id::text`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&deploy); err != nil {
		t.Fatal(err)
	}
	endpoint := nodeagent.LocalEndpoint(deploy)
	const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,$2,'handle-test','local','handle-test','agent','ready',$3,$4)`, deploy, org, deploy+".localhost", endpoint); err != nil {
		t.Fatal(err)
	}
	checker := Checker{Local: true, DNS: dnsFixture{records: []string{"did=" + did}}, Directory: directoryFixture{doc: Document{ID: did, AlsoKnownAs: []string{"at://creator.example.org"}, Service: []Service{{ID: "#atproto_pds", Type: "AtprotoPersonalDataServer", Endpoint: endpoint}}}}}
	store := &Store{Pool: pool, Checker: checker}
	if _, err := store.Set(ctx, owner, deploy, "creator.example.org"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unconfirmed account accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO enrollment_operations(id,deployment_id,owner_id,idempotency_key,state,message,account_did,account_handle,email_confirmed) VALUES(gen_random_uuid(),$1,$2,gen_random_uuid(),'verified','Confirmed',$3,'original.test',true)`, deploy, owner, did); err != nil {
		t.Fatal(err)
	}
	return store, owner, bob, deploy
}

func TestStoreOwnerScopeBoundedRechecksAndLifecycle(t *testing.T) {
	store, owner, bob, deploy := newStoreFixture(t)
	pool := store.Pool
	ctx := context.Background()
	endpoint := nodeagent.LocalEndpoint(deploy)
	const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := store.Set(ctx, bob, deploy, "creator.example.org"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner configured handle")
	}
	value, err := store.Set(ctx, owner, deploy, "Creator.Example.org")
	if err != nil {
		t.Fatal(err)
	}
	if value.Handle != "creator.example.org" || value.TXTName != "_atproto.creator.example.org" || value.TXTValue != "did="+did || value.Revision != 1 {
		t.Fatal("incorrect exact TXT instructions")
	}
	again, err := store.Set(ctx, owner, deploy, value.Handle)
	if err != nil || again.Revision != value.Revision {
		t.Fatal("unchanged intent advanced revision")
	}
	var events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM handle_events`).Scan(&events); err != nil || events != 1 {
		t.Fatal("duplicate intent event")
	}
	verified, err := store.Check(ctx, owner, deploy)
	if err != nil || verified.State != "verified" {
		t.Fatal("valid bidirectional proof failed")
	}
	if _, err = store.Get(ctx, bob, deploy); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner read configuration")
	}
	if _, err = store.Check(ctx, owner, deploy); !errors.Is(err, ErrConflict) {
		t.Fatal("unbounded recheck")
	}
	changed, err := store.Set(ctx, owner, deploy, "other.example.org")
	if err != nil || changed.Revision != 2 || changed.State != "pending_dns" {
		t.Fatal("domain change not isolated")
	}
	if _, err = store.Check(ctx, owner, deploy); !errors.Is(err, ErrConflict) {
		t.Fatal("domain change bypassed recheck limit")
	}
	if _, err = pool.Exec(ctx, `UPDATE handle_configurations SET checked_at=now()-interval '31 seconds'`); err != nil {
		t.Fatal(err)
	}
	store.Checker.DNS = dnsFixture{err: &net.DNSError{IsNotFound: true}}
	pending, err := store.Check(ctx, owner, deploy)
	if err != nil || pending.State != "pending_dns" {
		t.Fatal("DNS disappearance not reported")
	}
	var retainedEndpoint, state string
	if err = pool.QueryRow(ctx, `SELECT endpoint_url,state FROM deployments WHERE id=$1`, deploy).Scan(&retainedEndpoint, &state); err != nil || retainedEndpoint != endpoint || state != "ready" {
		t.Fatal("handle lifecycle changed deployment")
	}
}

type blockingDNS struct {
	entered chan struct{}
	release chan struct{}
}

func (d blockingDNS) LookupTXT(ctx context.Context, _ string) ([]string, error) {
	close(d.entered)
	select {
	case <-d.release:
		return []string{"did=did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestChangedIntentDiscardsInFlightProof(t *testing.T) {
	store, owner, _, deploy := newStoreFixture(t)
	ctx := context.Background()
	if _, err := store.Set(ctx, owner, deploy, "creator.example.org"); err != nil {
		t.Fatal(err)
	}
	dns := blockingDNS{make(chan struct{}), make(chan struct{})}
	store.Checker.DNS = dns
	result := make(chan error, 1)
	go func() { _, err := store.Check(ctx, owner, deploy); result <- err }()
	<-dns.entered
	if _, err := store.Set(ctx, owner, deploy, "other.example.org"); err != nil {
		t.Fatal(err)
	}
	close(dns.release)
	if err := <-result; !errors.Is(err, ErrConflict) {
		t.Fatal("stale proof was accepted")
	}
	current, err := store.Get(ctx, owner, deploy)
	if err != nil || current.Handle != "other.example.org" || current.State != "pending_dns" {
		t.Fatal("old proof overwrote changed domain")
	}
}
