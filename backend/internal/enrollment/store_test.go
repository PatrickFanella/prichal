package enrollment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type storeFixture struct {
	pool                   *pgxpool.Pool
	store                  *Store
	owner, bob, deployment string
}

func newStoreFixture(t *testing.T) storeFixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool, testCipher(t))
	if err != nil {
		t.Fatal(err)
	}
	result := storeFixture{pool: pool, store: store}
	for _, entry := range []struct {
		subject string
		id      *string
	}{{"alice", &result.owner}, {"bob", &result.bob}} {
		if err = pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local',$1,$1) RETURNING id::text`, entry.subject).Scan(entry.id); err != nil {
			t.Fatal(err)
		}
	}
	var org, node string
	if err = pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Enrollment',$1) RETURNING id::text`, result.owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&result.deployment); err != nil {
		t.Fatal(err)
	}
	id := result.deployment
	if _, err = pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,$2,'enrollment','local','enrollment','agent','ready',$3,$4)`, id, org, id+".localhost", nodeagent.LocalEndpoint(id)); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id::text`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	return result
}

type agentFixture struct {
	calls    int
	verified bool
	failure  bool
}

func (a *agentFixture) Enroll(_ context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.Enrollment, error) {
	a.calls++
	if a.failure {
		return nodeagent.Enrollment{}, errors.New("PRIVATE ADMINISTRATOR KEY")
	}
	result := nodeagent.Enrollment{InstanceID: id, Ticket: ticket, State: "invitation_ready", Code: "one-use-" + ticket, ExpiresAt: time.Now().Add(10 * time.Minute).UnixMilli()}
	if a.verified {
		result.State = "verified"
		result.Code = ""
		result.DID = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
		result.Handle = "first.test"
		result.EmailConfirmed = true
	}
	return result, nil
}
func job(id string, attempt int) *river.Job[Args] {
	return &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3}, Args: Args{id}}
}

func TestEnrollmentIntentIsAtomicOwnerScopedAndIdempotent(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	request := Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	var wait sync.WaitGroup
	results := make(chan Operation, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			operation, err := f.store.Submit(ctx, f.owner, f.deployment, request)
			if err != nil {
				failures <- err
			} else {
				results <- operation
			}
		}()
	}
	wait.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var id string
	for result := range results {
		if id == "" {
			id = result.ID
		}
		if result.ID != id {
			t.Fatal("duplicate intent")
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='first_account_enrollment'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("job insertion was not atomic")
	}
	if _, err := f.store.Get(ctx, f.bob, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner read invitation")
	}
	if _, err := f.store.Submit(ctx, f.bob, f.deployment, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner created invitation")
	}
	request.Recover = true
	if _, err := f.store.Submit(ctx, f.owner, f.deployment, request); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay payload accepted")
	}
}
func TestRecoveredCheckpointKeepsCodeEncryptedAndEventsBounded(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_enrollment_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'checkpoint unavailable'; END $$; CREATE TRIGGER fail_enrollment_checkpoint BEFORE UPDATE ON enrollment_operations FOR EACH ROW EXECUTE FUNCTION fail_enrollment_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err == nil {
		t.Fatal("lost checkpoint reported success")
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_enrollment_checkpoint ON enrollment_operations; DROP FUNCTION fail_enrollment_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, job(op.ID, 2)); err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || ready.State != "invitation_ready" || ready.Code == "" {
		t.Fatal("replay did not recover protected invitation")
	}
	var ciphertext []byte
	if err = f.pool.QueryRow(ctx, `SELECT code_ciphertext FROM enrollment_operations WHERE id=$1`, op.ID).Scan(&ciphertext); err != nil || strings.Contains(string(ciphertext), ready.Code) {
		t.Fatal("plaintext code stored")
	}
	if agent.calls != 2 || len(ready.Events) != 2 {
		t.Fatal("replay duplicated events")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE enrollment_operations SET expires_at=now()-interval '1 second' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || expired.State != "expired" || expired.Code != "" {
		t.Fatal("expired code disclosed")
	}
	agent.verified = true
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	verified, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || verified.State != "verified" || verified.Code != "" {
		t.Fatal("completed account retained invitation")
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Recover: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("completed account got another enrollment")
	}
}
func TestRecoveryBoundSupersessionAndFailureRedaction(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	var previous string
	for i, key := range []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "cccccccc-cccc-cccc-cccc-cccccccccccc"} {
		op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: key, Recover: i > 0})
		if err != nil {
			t.Fatal(err)
		}
		if previous != "" {
			old, err := f.store.Get(ctx, f.owner, previous)
			if err != nil || old.State != "superseded" || old.Code != "" {
				t.Fatal("replacement disclosed old code")
			}
		}
		if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
			t.Fatal(err)
		}
		previous = op.ID
	}
	if _, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "dddddddd-dddd-dddd-dddd-dddddddddddd", Recover: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("unbounded recovery accepted")
	}
	agent.failure = true
	if err := worker.Work(ctx, job(previous, 3)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("private native error escaped")
	}
	failed, err := f.store.Get(ctx, f.owner, previous)
	if err != nil || failed.State != "failed" || strings.Contains(failed.Message, "PRIVATE") {
		t.Fatal("failure was not safe")
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	calls := agent.calls
	if err = worker.Work(ctx, job(previous, 3)); err == nil || agent.calls != calls {
		t.Fatal("held enrollment executed a host action")
	}
}
