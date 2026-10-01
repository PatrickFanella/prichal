package hostingexit

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/exports"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const owner = "11111111-1111-4111-8111-111111111111"
const foreign = "22222222-2222-4222-8222-222222222222"
const instance = "33333333-3333-4333-8333-333333333333"
const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
const key = "44444444-4444-4444-8444-444444444444"
const reverseKey = "55555555-5555-4555-8555-555555555555"

type fakeAgent struct {
	calls, accountCalls int
	fail                bool
}

func (a *fakeAgent) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: instance, State: "running", Healthy: true}, nil
}
func (a *fakeAgent) Export(context.Context, nodeagent.Peer, string, string) (nodeagent.PortableExport, error) {
	return nodeagent.PortableExport{DID: did, SigningKey: "did:key:z222222222222222222222222222222222222222222222", CAR: []byte("fixture signed CAR"), Preferences: []byte(`{"namespace":"app.bsky","preferences":[]}`), CapturedAt: time.Now().UTC()}, nil
}
func (a *fakeAgent) Service(_ context.Context, _ nodeagent.Peer, id string, r nodeagent.ServiceRequest) (nodeagent.ServiceResult, error) {
	a.calls++
	if a.fail {
		return nodeagent.ServiceResult{}, errors.New("PRIVATE HOST ERROR")
	}
	return nodeagent.ServiceResult{InstanceID: id, ServiceRequest: r}, nil
}
func (a *fakeAgent) Account(context.Context, nodeagent.Peer, string, nodeagent.ServiceRequest) (nodeagent.AccountResult, error) {
	a.accountCalls++
	return nodeagent.AccountResult{}, errors.New("account moderation forbidden")
}
func fixture(t *testing.T) (*Store, string, *fakeAgent) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.NewRecovery(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{owner, foreign} {
		if _, err := pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) VALUES($1,'local',$2,'Owner')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	var org, node string
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Exit',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,$2,'exit','local','exit','agent','ready','exit.localhost')`, instance, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,state,account_did,email_confirmed) VALUES($1,$2,gen_random_uuid(),'verified',$3,true)`, instance, owner, did); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',3,3,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, instance, node); err != nil {
		t.Fatal(err)
	}
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "key")
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, secret, 0600); err != nil {
		t.Fatal(err)
	}
	artifacts, err := exports.NewArtifacts(directory, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := exports.New(pool, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	export, err := portable.Submit(ctx, owner, instance, key)
	if err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{}
	capture := &exports.Worker{Store: portable, Agent: agent}
	if err = capture.Work(ctx, &river.Job[exports.Args]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: exports.Args{ExportID: export.ID}}); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool, &Policy{Version: 1, DelaySeconds: 30, RetentionDays: 7}, portable)
	if err != nil {
		t.Fatal(err)
	}
	return store, export.ID, agent
}
func exitJob(id string) *river.Job[Args] {
	return &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5}, Args: Args{id}}
}
func lifeJob(id string) *river.Job[lifecycle.Args] {
	return &river.Job[lifecycle.Args]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: lifecycle.Args{OperationID: id}}
}
func request(export string) Request {
	return Request{Key: key, ExportID: export, ConfirmTimeline: true, ConfirmExportSaved: true, DelaySeconds: 30, RetentionDays: 7}
}
func due(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `UPDATE hosting_exits SET created_at=now()-interval '2 minutes',service_end_at=now()-interval '1 minute',retention_review_at=now()-interval '1 minute'+make_interval(hours=>retention_days*24) WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
}
func stop(t *testing.T, s *Store, id string, agent *fakeAgent) {
	t.Helper()
	due(t, s, id)
	worker := &Worker{Store: s}
	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), exitJob(id)); !errors.As(err, &snooze) {
		t.Fatalf("stop intent: %v", err)
	}
	var stopID string
	if err := s.Pool.QueryRow(context.Background(), `SELECT stop_operation_id FROM hosting_exits WHERE id=$1`, id).Scan(&stopID); err != nil {
		t.Fatal(err)
	}
	native := &lifecycle.Worker{Pool: s.Pool, Agent: agent}
	if err := native.Work(context.Background(), lifeJob(stopID)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(context.Background(), exitJob(id)); err != nil {
		t.Fatal(err)
	}
}
func TestOwnedExitEarlyAndEndedReversalPreserveData(t *testing.T) {
	s, export, agent := fixture(t)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, foreign, instance, request(export)); err != ErrNotFound {
		t.Fatal("foreign schedule accepted")
	}
	op, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil {
		t.Fatal(err)
	}
	if op.State != "scheduled" || op.AutomaticDeletion || op.RetentionReviewAt.Sub(op.ServiceEndAt) != 7*24*time.Hour {
		t.Fatal("unsafe timeline")
	}
	duplicate, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil || duplicate.ID != op.ID {
		t.Fatal("duplicate intent")
	}
	if _, err = s.Get(ctx, foreign, op.ID); err != ErrNotFound {
		t.Fatal("foreign progress")
	}
	if _, err = s.Reverse(ctx, foreign, op.ID, reverseKey); err != ErrNotFound {
		t.Fatal("foreign reversal")
	}
	worker := &Worker{Store: s}
	if err = worker.Work(ctx, exitJob(op.ID)); err == nil {
		t.Fatal("service ended early")
	}
	early, err := s.Reverse(ctx, owner, op.ID, reverseKey)
	if err != nil || early.State != "reversed" {
		t.Fatal("early reversal failed")
	}
	if err = worker.Work(ctx, exitJob(op.ID)); err != nil || agent.calls != 0 {
		t.Fatal("withdrawn exit touched host")
	}
	second := request(export)
	second.Key = reverseKey
	op, err = s.Schedule(ctx, owner, instance, second)
	if err != nil {
		t.Fatal(err)
	}
	stop(t, s, op.ID, agent)
	ended, err := s.Get(ctx, owner, op.ID)
	if err != nil || ended.State != "ended" || !ended.CanReverse {
		t.Fatal("ended state unavailable")
	}
	download, err := s.Exports.Download(ctx, owner, export)
	if err != nil || len(download) == 0 {
		t.Fatal("stopped service lost retained export")
	}
	resuming, err := s.Reverse(ctx, owner, op.ID, key)
	if err != nil || resuming.State != "resuming" {
		t.Fatalf("reversal: %v", err)
	}
	replay, err := s.Reverse(ctx, owner, op.ID, key)
	if err != nil || replay.ID != op.ID {
		t.Fatal("duplicate reversal")
	}
	var resumeID string
	if err = s.Pool.QueryRow(ctx, `SELECT resume_operation_id FROM hosting_exits WHERE id=$1`, op.ID).Scan(&resumeID); err != nil {
		t.Fatal(err)
	}
	native := &lifecycle.Worker{Pool: s.Pool, Agent: agent}
	if err = native.Work(ctx, lifeJob(resumeID)); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, exitJob(op.ID)); err != nil {
		t.Fatal(err)
	}
	result, err := s.Get(ctx, owner, op.ID)
	if err != nil || result.State != "reversed" || agent.calls != 2 || agent.accountCalls != 0 {
		t.Fatal("exit reversal changed moderation or lost state")
	}
}
func TestExitConcurrencyAndAtomicQueueRollback(t *testing.T) {
	s, export, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT exit_queue_failure CHECK(kind<>'hosting_exit_retention') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Schedule(ctx, owner, instance, request(export)); err == nil {
		t.Fatal("partial exit accepted")
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM hosting_exits)+(SELECT count(*) FROM hosting_exit_events)+(SELECT count(*) FROM river_job WHERE kind LIKE 'hosting_exit%')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("exit queue rollback failed")
	}
	if _, err := s.Pool.Exec(ctx, `ALTER TABLE river_job DROP CONSTRAINT exit_queue_failure`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, conflicts := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := request(export)
			r.Key = fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012x", i)
			_, err := s.Schedule(ctx, owner, instance, r)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted++
			} else if errors.Is(err, ErrConflict) {
				conflicts++
			} else {
				t.Errorf("concurrent schedule: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if accepted != 1 || conflicts != 7 {
		t.Fatal("multiple active exits accepted")
	}
}
func TestExitReversalCannotUndoLaterOperatorHold(t *testing.T) {
	s, export, agent := fixture(t)
	ctx := context.Background()
	op, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil {
		t.Fatal(err)
	}
	stop(t, s, op.ID, agent)
	hold, err := s.Lifecycle.Submit(ctx, "operator", instance, lifecycle.Request{Key: reverseKey, Kind: "service", State: "suspended", Reason: "safety_review"})
	if err != nil {
		t.Fatal(err)
	}
	native := &lifecycle.Worker{Pool: s.Pool, Agent: agent}
	if err = native.Work(ctx, lifeJob(hold.ID)); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(ctx, owner, op.ID)
	if err != nil || current.CanReverse {
		t.Fatal("operator-held service advertised reversible")
	}
	if _, err = s.Reverse(ctx, owner, op.ID, "66666666-6666-4666-8666-666666666666"); err != ErrConflict {
		t.Fatal("owner bypassed operator hold")
	}
	if agent.calls != 2 || agent.accountCalls != 0 {
		t.Fatal("foreign service fence modified")
	}
}
func TestRetentionReviewAndRecoveryHoldNeverDelete(t *testing.T) {
	s, export, agent := fixture(t)
	ctx := context.Background()
	op, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil {
		t.Fatal(err)
	}
	if err = recovery.Hold(ctx, s.Pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: s}
	if err = worker.Work(ctx, exitJob(op.ID)); err == nil || agent.calls != 0 {
		t.Fatal("held restore touched host")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now()`); err != nil {
		t.Fatal(err)
	}
	stop(t, s, op.ID, agent)
	if _, err = s.Pool.Exec(ctx, `UPDATE hosting_exits SET created_at=now()-interval '9 days',service_end_at=now()-interval '8 days',retention_review_at=now()-interval '1 day' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	retention := &RetentionWorker{Store: s}
	if err = retention.Work(ctx, &river.Job[RetentionArgs]{Args: RetentionArgs{op.ID}}); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(ctx, owner, op.ID)
	if err != nil || current.State != "review" || current.CanReverse || current.AutomaticDeletion {
		t.Fatal("retention expiry not retained for review")
	}
	if _, err = s.Reverse(ctx, owner, op.ID, reverseKey); err != ErrConflict {
		t.Fatal("late owner reversal accepted")
	}
	if _, err = s.Exports.Download(ctx, owner, export); err != nil || agent.calls != 1 || agent.accountCalls != 0 {
		t.Fatal("review deleted retained data")
	}
}
func TestDefaultDisabledAndFinalFailureRequireReview(t *testing.T) {
	s, export, _ := fixture(t)
	ctx := context.Background()
	s.Policy = nil
	if _, err := s.Schedule(ctx, owner, instance, request(export)); err != ErrUnavailable {
		t.Fatal("default policy allowed scheduling")
	}
	s.Policy = &Policy{Version: 1, DelaySeconds: 30, RetentionDays: 7}
	op, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil {
		t.Fatal(err)
	}
	due(t, s, op.ID)
	if _, err = s.Pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT exit_native_queue_failure CHECK(kind<>'service_lifecycle') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	job := exitJob(op.ID)
	job.Attempt = job.MaxAttempts
	worker := &Worker{Store: s}
	if err = worker.Work(ctx, job); err == nil || strings.Contains(err.Error(), "SQL") {
		t.Fatal("private failure leaked")
	}
	current, err := s.Get(ctx, owner, op.ID)
	if err != nil || current.State != "review" {
		t.Fatal("discarded exit stayed scheduled")
	}
}

func TestExitDoesNotReplacePreexistingOperatorServiceHold(t *testing.T) {
	s, export, agent := fixture(t)
	ctx := context.Background()
	op, err := s.Schedule(ctx, owner, instance, request(export))
	if err != nil {
		t.Fatal(err)
	}
	hold, err := s.Lifecycle.Submit(ctx, "operator", instance, lifecycle.Request{Key: reverseKey, Kind: "service", State: "suspended", Reason: "maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	native := &lifecycle.Worker{Pool: s.Pool, Agent: agent}
	if err = native.Work(ctx, lifeJob(hold.ID)); err != nil {
		t.Fatal(err)
	}
	due(t, s, op.ID)
	worker := &Worker{Store: s}
	if err = worker.Work(ctx, exitJob(op.ID)); err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(ctx, owner, op.ID)
	if err != nil || current.State != "review" || current.CanReverse || agent.calls != 1 {
		t.Fatal("exit replaced unrelated hold")
	}
	if _, err = s.Lifecycle.Submit(ctx, "operator", instance, lifecycle.Request{Key: key, Kind: "service", State: "running", Reason: "customer_exit"}); err == nil {
		t.Fatal("ordinary operator path accepted reserved exit reason")
	}
}

func TestReviewedPolicyIsBoundAndCommittedRetriesSurviveChange(t *testing.T) {
	s, export, _ := fixture(t)
	ctx := context.Background()
	r := request(export)
	r.RetentionDays = 1
	if _, err := s.Schedule(ctx, owner, instance, r); err != ErrConflict {
		t.Fatal("unreviewed policy accepted")
	}
	r = request(export)
	op, err := s.Schedule(ctx, owner, instance, r)
	if err != nil {
		t.Fatal(err)
	}
	s.Policy = &Policy{Version: 1, DelaySeconds: 60, RetentionDays: 14}
	replay, err := s.Schedule(ctx, owner, instance, r)
	if err != nil || replay.ID != op.ID {
		t.Fatal("policy change altered committed retry")
	}
	r.RetentionDays = 14
	if _, err = s.Schedule(ctx, owner, instance, r); err != ErrConflict {
		t.Fatal("request key reused with a different reviewed policy")
	}
	r.DelaySeconds = 0
	r.RetentionDays = 0
	s.Policy = nil
	if replay, err = s.Schedule(ctx, owner, instance, r); err != nil || replay.ID != op.ID {
		t.Fatal("legacy committed retry lost")
	}
}
