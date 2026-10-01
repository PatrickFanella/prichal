// Package lifecycle records operator service intent independently of host execution.
package lifecycle

import (
	"context"
	"errors"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var ErrConflict = errors.New("service transition conflicts with retained intent")

type Args struct {
	OperationID string `json:"operationId"`
}

func (Args) Kind() string { return "service_lifecycle" }

type Request struct{ Key, State, Reason string }
type Operation struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deploymentId"`
	Generation   int64     `json:"generation"`
	DesiredState string    `json:"desiredState"`
	PriorState   string    `json:"priorState"`
	Reason       string    `json:"reason"`
	State        string    `json:"state"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type Store struct {
	Pool  *pgxpool.Pool
	Queue *river.Client[pgx.Tx]
}

// New creates a queue producer with no host credentials.
func New(pool *pgxpool.Pool) (*Store, error) {
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{Pool: pool, Queue: queue}, nil
}

// Submit is an operator-only contract. There is no customer HTTP mutation endpoint.
func (s *Store) Submit(ctx context.Context, actor, deployment string, request Request) (Operation, error) {
	if !uuid.MatchString(deployment) || !uuid.MatchString(request.Key) || len(actor) < 1 || len(actor) > 128 || (request.State != "running" && request.State != "suspended") || (request.Reason != "maintenance" && request.Reason != "safety_review" && request.Reason != "resource_pressure") {
		return Operation{}, errors.New("invalid fixed service intent")
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return Operation{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var state, kind string
	if err = tx.QueryRow(ctx, `SELECT state,kind FROM deployments WHERE id=$1 FOR UPDATE`, deployment).Scan(&state, &kind); err != nil {
		return Operation{}, errors.New("managed deployment unavailable")
	}
	if state != "ready" || kind != "agent" {
		return Operation{}, ErrConflict
	}
	var id, desired, reason, previousActor string
	err = tx.QueryRow(ctx, `SELECT id::text,desired_state,reason,actor FROM service_operations WHERE deployment_id=$1 AND idempotency_key=$2`, deployment, request.Key).Scan(&id, &desired, &reason, &previousActor)
	if err == nil {
		if desired != request.State || reason != request.Reason || previousActor != actor {
			return Operation{}, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	var generation int64
	var latestState string
	prior := "running"
	err = tx.QueryRow(ctx, `SELECT generation,state,desired_state FROM service_operations WHERE deployment_id=$1 ORDER BY generation DESC LIMIT 1`, deployment).Scan(&generation, &latestState, &prior)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	if latestState == "pending" {
		return Operation{}, ErrConflict
	}
	if latestState == "failed" {
		prior = "unknown"
	}
	if generation >= 1<<53 {
		return Operation{}, ErrConflict
	}
	if err = tx.QueryRow(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,desired_state,prior_state,reason,actor) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, deployment, request.Key, generation+1, request.State, prior, request.Reason, actor).Scan(&id); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_operation_events(operation_id,state) VALUES($1,'pending')`, id); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, id)
}

// Get reads an operator result without exposing protected native metadata.
func (s *Store) Get(ctx context.Context, id string) (Operation, error) {
	var op Operation
	err := s.Pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,generation,desired_state,prior_state,reason,state,updated_at FROM service_operations WHERE id=$1`, id).Scan(&op.ID, &op.DeploymentID, &op.Generation, &op.DesiredState, &op.PriorState, &op.Reason, &op.State, &op.UpdatedAt)
	return op, err
}
