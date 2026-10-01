package hostingexit

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/exports"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"regexp"
	"time"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var ErrNotFound = errors.New("hosting exit not found")
var ErrConflict = errors.New("hosting exit requires review")
var ErrUnavailable = errors.New("local hosting exit scheduling is disabled")

type Args struct {
	ExitID string `json:"exitId"`
}

func (Args) Kind() string { return "hosting_exit" }

type Request struct {
	Key                string `json:"idempotencyKey"`
	ExportID           string `json:"exportId"`
	DelaySeconds       int    `json:"delaySeconds"`
	RetentionDays      int    `json:"retentionDays"`
	ConfirmExportSaved bool   `json:"confirmExportSaved"`
	ConfirmTimeline    bool   `json:"confirmTimeline"`
}
type Operation struct {
	ID                string    `json:"id"`
	DeploymentID      string    `json:"deploymentId"`
	ExportID          string    `json:"exportId"`
	State             string    `json:"state"`
	ServiceEndAt      time.Time `json:"serviceEndAt"`
	RetentionReviewAt time.Time `json:"retentionReviewAt"`
	CanReverse        bool      `json:"canReverse"`
	AutomaticDeletion bool      `json:"automaticDeletion"`
}
type Store struct {
	Pool      *pgxpool.Pool
	Queue     *river.Client[pgx.Tx]
	Policy    *Policy
	Exports   *exports.Store
	Lifecycle *lifecycle.Store
}

func New(pool *pgxpool.Pool, policy *Policy, portable *exports.Store) (*Store, error) {
	if policy != nil && !policy.valid() {
		return nil, ErrUnavailable
	}
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	actions, err := lifecycle.New(pool)
	if err != nil {
		return nil, err
	}
	return &Store{pool, queue, policy, portable, actions}, nil
}
func (s *Store) ownedDeployment(ctx context.Context, owner, deployment string) error {
	if !uuid.MatchString(deployment) {
		return ErrNotFound
	}
	var found string
	err := s.Pool.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2`, deployment, owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (s *Store) Schedule(ctx context.Context, owner, deployment string, r Request) (Operation, error) {
	if !uuid.MatchString(r.Key) || !uuid.MatchString(r.ExportID) || !r.ConfirmTimeline || !r.ConfirmExportSaved {
		return Operation{}, ErrConflict
	}
	if err := s.ownedDeployment(ctx, owner, deployment); err != nil {
		return Operation{}, err
	}
	// Stable committed retries remain readable after scheduling configuration changes or export expiry.
	var priorID, priorExport string
	var priorDelay, priorRetention int
	err := s.Pool.QueryRow(ctx, `SELECT id,export_id,delay_seconds,retention_days FROM hosting_exits WHERE deployment_id=$1 AND owner_id=$2 AND idempotency_key=$3`, deployment, owner, r.Key).Scan(&priorID, &priorExport, &priorDelay, &priorRetention)
	if err == nil {
		if priorExport != r.ExportID || ((r.DelaySeconds != 0 || r.RetentionDays != 0) && (r.DelaySeconds != priorDelay || r.RetentionDays != priorRetention)) {
			return Operation{}, ErrConflict
		}
		return s.Get(ctx, owner, priorID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	if s.Policy == nil || s.Exports == nil {
		return Operation{}, ErrUnavailable
	}
	if r.DelaySeconds != s.Policy.DelaySeconds || r.RetentionDays != s.Policy.RetentionDays {
		return Operation{}, ErrConflict
	}
	if err = recovery.Check(ctx, s.Pool); err != nil {
		return Operation{}, err
	}
	if _, err = s.Exports.Download(ctx, owner, r.ExportID); err != nil {
		return Operation{}, ErrConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2 AND d.kind='agent' AND d.state='ready' FOR UPDATE OF d`, deployment, owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	// Resolve a concurrently committed identical request before the active-exit check.
	err = tx.QueryRow(ctx, `SELECT id,export_id,delay_seconds,retention_days FROM hosting_exits WHERE deployment_id=$1 AND owner_id=$2 AND idempotency_key=$3`, deployment, owner, r.Key).Scan(&priorID, &priorExport, &priorDelay, &priorRetention)
	if err == nil {
		if priorExport != r.ExportID || ((r.DelaySeconds != 0 || r.RetentionDays != 0) && (r.DelaySeconds != priorDelay || r.RetentionDays != priorRetention)) {
			return Operation{}, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, priorID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM export_operations e WHERE e.id=$1 AND e.owner_id=$2 AND e.deployment_id=$3 AND e.state='ready' AND e.expires_at>clock_timestamp() AND e.account_did=(SELECT account_did FROM enrollment_operations WHERE deployment_id=$3 AND state='verified' ORDER BY created_at DESC LIMIT 1)) AND NOT EXISTS(SELECT 1 FROM hosting_exits WHERE deployment_id=$3 AND state<>'reversed')`, r.ExportID, owner, deployment).Scan(&eligible); err != nil {
		return Operation{}, err
	}
	if !eligible {
		return Operation{}, ErrConflict
	}
	var id string
	var endAt, reviewAt time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO hosting_exits(deployment_id,owner_id,idempotency_key,export_id,delay_seconds,retention_days,service_end_at,retention_review_at) VALUES($1,$2,$3,$4,$5,$6,now()+make_interval(secs=>$5::integer),now()+make_interval(secs=>$5::integer)+make_interval(hours=>$6::integer*24)) RETURNING id,service_end_at,retention_review_at`, deployment, owner, r.Key, r.ExportID, s.Policy.DelaySeconds, s.Policy.RetentionDays).Scan(&id, &endAt, &reviewAt); err != nil {
		return Operation{}, err
	}
	if err = event(ctx, tx, id, "scheduled"); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 5, ScheduledAt: endAt}); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, RetentionArgs{id}, &river.InsertOpts{MaxAttempts: 5, ScheduledAt: reviewAt}); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
func (s *Store) Get(ctx context.Context, owner, id string) (Operation, error) {
	if !uuid.MatchString(id) {
		return Operation{}, ErrNotFound
	}
	var op Operation
	err := s.Pool.QueryRow(ctx, `SELECT e.id,e.deployment_id,e.export_id,e.state,e.service_end_at,e.retention_review_at,(e.retention_review_at>clock_timestamp() AND (e.state='scheduled' OR (e.state='ended' AND e.stop_operation_id=(SELECT id FROM service_operations WHERE deployment_id=e.deployment_id AND kind='service' ORDER BY generation DESC LIMIT 1) AND EXISTS(SELECT 1 FROM service_operations WHERE id=e.stop_operation_id AND state='applied' AND reason='customer_exit' AND actor='hosting-exit:'||e.id::text) AND NOT EXISTS(SELECT 1 FROM service_operations WHERE deployment_id=e.deployment_id AND state='pending')))) FROM hosting_exits e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2`, id, owner).Scan(&op.ID, &op.DeploymentID, &op.ExportID, &op.State, &op.ServiceEndAt, &op.RetentionReviewAt, &op.CanReverse)
	if errors.Is(err, pgx.ErrNoRows) {
		return op, ErrNotFound
	}
	return op, err
}
func (s *Store) List(ctx context.Context, owner, deployment string) ([]Operation, error) {
	if err := s.ownedDeployment(ctx, owner, deployment); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM hosting_exits WHERE deployment_id=$1 AND owner_id=$2 ORDER BY created_at DESC LIMIT 10`, deployment, owner)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]Operation, 0, len(ids))
	for _, id := range ids {
		op, err := s.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, nil
}
func event(ctx context.Context, tx pgx.Tx, id, state string) error {
	_, err := tx.Exec(ctx, `INSERT INTO hosting_exit_events(exit_id,state) VALUES($1,$2)`, id, state)
	return err
}
func (s *Store) Reverse(ctx context.Context, owner, id, key string) (Operation, error) {
	if !uuid.MatchString(id) || !uuid.MatchString(key) {
		return Operation{}, ErrNotFound
	}
	op, err := s.Get(ctx, owner, id)
	if err != nil {
		return Operation{}, err
	}
	if err = recovery.Check(ctx, s.Pool); err != nil {
		return Operation{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2 FOR UPDATE OF d`, op.DeploymentID, owner).Scan(&found); err != nil {
		return Operation{}, ErrNotFound
	}
	var state string
	var within bool
	var stop *string
	if err = tx.QueryRow(ctx, `SELECT state,retention_review_at>clock_timestamp(),stop_operation_id FROM hosting_exits WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&state, &within, &stop); err != nil {
		return Operation{}, err
	}
	var prior bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hosting_exit_reversals WHERE exit_id=$1 AND owner_id=$2 AND idempotency_key=$3)`, id, owner, key).Scan(&prior); err != nil {
		return Operation{}, err
	}
	if prior {
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, id)
	}
	if !within || (state != "scheduled" && state != "ended") {
		return Operation{}, ErrConflict
	}
	next := "reversed"
	var resume any
	if state == "ended" {
		if stop == nil {
			return Operation{}, ErrConflict
		}
		resumeID, err := s.Lifecycle.InsertExitTx(ctx, tx, id, op.DeploymentID, key, *stop)
		if err != nil {
			return Operation{}, ErrConflict
		}
		resume = resumeID
		next = "resuming"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hosting_exit_reversals(exit_id,owner_id,idempotency_key) VALUES($1,$2,$3)`, id, owner, key); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE hosting_exits SET state=$2,resume_operation_id=$3,updated_at=now() WHERE id=$1`, id, next, resume); err != nil {
		return Operation{}, err
	}
	if err = event(ctx, tx, id, next); err != nil {
		return Operation{}, err
	}
	if next == "resuming" {
		if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 5}); err != nil {
			return Operation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
