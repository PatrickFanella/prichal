package entitlement_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*pgxpool.Pool, string, string, *deployment.Store) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','pilot','Pilot') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Pilot organization',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	store, err := deployment.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return pool, owner, org, store
}

const firstKey = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const revokeKey = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"

func TestNativeAllocationRequiresExplicitGrantAndReplaySurvivesRevocation(t *testing.T) {
	pool, owner, org, deploy := setup(t)
	ctx := context.Background()
	store := entitlement.Store{Pool: pool}
	request := deployment.Request{Kind: "agent", Name: "native-pilot", Region: "local", Key: "native-key"}
	if _, err := deploy.Submit(ctx, owner, org, request); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatalf("unentitled request accepted: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied request created work")
	}
	grant := entitlement.Request{Key: firstKey, State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"}
	op, err := store.Submit(ctx, "test-operator", org, grant)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Submit(ctx, "test-operator", org, grant)
	if err != nil || replay.ID != op.ID || !replay.ExpiresAt.Equal(*op.ExpiresAt) {
		t.Fatal("grant replay extended validity")
	}
	changed := grant
	changed.Plan = "local-small"
	if _, err = store.Submit(ctx, "test-operator", org, changed); !errors.Is(err, entitlement.ErrConflict) {
		t.Fatal("changed grant payload accepted")
	}
	if _, err = store.Submit(ctx, "another-operator", org, grant); !errors.Is(err, entitlement.ErrConflict) {
		t.Fatal("operator identity changed during replay")
	}
	native, err := deploy.Submit(ctx, owner, org, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(ctx, "test-operator", org, entitlement.Request{Key: revokeKey, State: "revoked", Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	result, err := store.Get(ctx, owner, org)
	if err != nil || result.State != "revoked" || result.CanDeploy || result.AllocatedInstances != 1 {
		t.Fatal("revocation did not retain allocation")
	}
	if _, err = store.Submit(ctx, "test-operator", org, grant); err != nil {
		t.Fatal(err)
	}
	result, err = store.Get(ctx, owner, org)
	if err != nil || result.State != "revoked" {
		t.Fatal("old grant replay regressed current entitlement")
	}
	repeated, err := deploy.Submit(ctx, owner, org, request)
	if err != nil || repeated.ID != native.ID {
		t.Fatal("revocation broke stable deployment replay")
	}
	request.Key = "other-key"
	request.Name = "second-native"
	if _, err = deploy.Submit(ctx, owner, org, request); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatal("revocation allowed new deployment")
	}
	if _, err = store.Get(ctx, "00000000-0000-0000-0000-000000000000", org); !errors.Is(err, entitlement.ErrNotFound) {
		t.Fatal("another owner read entitlement")
	}
}
func TestConcurrentNativeAllocationAndDowngradeRetainData(t *testing.T) {
	pool, owner, org, deploy := setup(t)
	ctx := context.Background()
	store := entitlement.Store{Pool: pool}
	if _, err := store.Submit(ctx, "test-operator", org, entitlement.Request{Key: firstKey, State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			_, err := deploy.Submit(ctx, owner, org, deployment.Request{Kind: "agent", Name: fmt.Sprintf("concurrent-%d", i), Region: "local", Key: fmt.Sprintf("key-%d", i)})
			results <- err
		}(i)
	}
	success := 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			success++
		} else if !errors.Is(err, entitlement.ErrLimit) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("concurrent allowance exceeded")
	}
	if _, err := pool.Exec(ctx, `UPDATE deployments SET state='failed' WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	request := deployment.Request{Kind: "agent", Name: "retained-request", Region: "local", Key: "retained-key"}
	if _, err := deploy.Submit(ctx, owner, org, request); !errors.Is(err, entitlement.ErrLimit) {
		t.Fatal("failed uncertain request implicitly freed allowance")
	}
	if _, err := store.Submit(ctx, "test-operator", org, entitlement.Request{Key: revokeKey, State: "active", Plan: "local-small", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.Submit(ctx, owner, org, request); err != nil {
		t.Fatal(err)
	}
	request.Name = "third-native"
	request.Key = "third-key"
	if _, err := deploy.Submit(ctx, owner, org, request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Submit(ctx, "test-operator", org, entitlement.Request{Key: "cccccccc-cccc-4ccc-cccc-cccccccccccc", State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Get(ctx, owner, org)
	if err != nil || status.AllocatedInstances != 3 || status.InstanceLimit != 1 || status.AvailableInstances != 0 || status.CanDeploy {
		t.Fatal("downgrade lost retained allocation or allowed growth")
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&jobs); err != nil || jobs != 3 {
		t.Fatal("downgrade modified durable work")
	}
}
func TestGrantExpiryAndValidation(t *testing.T) {
	pool, owner, org, deploy := setup(t)
	ctx := context.Background()
	store := entitlement.Store{Pool: pool}
	for _, request := range []entitlement.Request{{Key: firstKey, State: "active", Plan: "paid-unlimited", Hours: 24, Reason: "qualification"}, {Key: firstKey, State: "active", Plan: "local-single", Hours: 721, Reason: "qualification"}, {Key: firstKey, State: "revoked", Plan: "local-small", Reason: "pilot"}, {Key: firstKey, State: "active", Plan: "local-single", Hours: 24, Reason: "payment_received"}} {
		if _, err := store.Submit(ctx, "operator", org, request); !errors.Is(err, entitlement.ErrInvalid) {
			t.Fatal("invalid/unpaid exception accepted")
		}
	}
	if _, err := store.Submit(ctx, "operator", org, entitlement.Request{Key: firstKey, State: "active", Plan: "local-single", Hours: 1, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE pilot_entitlement_operations SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	status, err := store.Get(ctx, owner, org)
	if err != nil || status.State != "expired" || status.CanDeploy {
		t.Fatal("expired grant remained active")
	}
	if _, err := deploy.Submit(ctx, owner, org, deployment.Request{Kind: "agent", Name: "expired-native", Region: "local", Key: "expired-key"}); !errors.Is(err, entitlement.ErrDenied) {
		t.Fatal("expired grant allowed native work")
	}
	if _, err := deploy.Submit(ctx, owner, org, deployment.Request{Kind: "test", Name: "simulation", Region: "local", Key: "simulation-key"}); err != nil {
		t.Fatal("non-billable local simulation blocked")
	}
}
