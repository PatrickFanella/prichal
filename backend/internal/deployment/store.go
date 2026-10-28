// Package deployment persists customer operations separately from host execution.
package deployment

import (
	"context"
	"errors"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// ErrEndpointPending prevents silent fallback from an unverified custom choice.
var ErrEndpointPending = errors.New("custom endpoint choice requires qualification or cancellation")

var ErrConflict = errors.New("request conflicts with an existing deployment")
var ErrNotFound = errors.New("deployment or organization not found")
var ErrInvalid = errors.New("invalid deployment request")
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$|^[a-z]$`)
var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type Request struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Key    string `json:"idempotencyKey"`
}
type Event struct {
	State     string    `json:"state"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}
type Operation struct {
	Release         observability.ReleaseStatus `json:"release"`
	Service         observability.Status        `json:"service"`
	Usage           observability.UsageStatus   `json:"usage"`
	ManagedHostname string                      `json:"managedHostname"`
	Endpoint        string                      `json:"endpoint"`
	Kind            string                      `json:"kind"`
	ID              string                      `json:"id"`
	OrganizationID  string                      `json:"organizationId"`
	Name            string                      `json:"name"`
	Region          string                      `json:"region"`
	State           string                      `json:"state"`
	Message         string                      `json:"message"`
	Events          []Event                     `json:"events"`
}
type Args struct {
	DeploymentID string `json:"deploymentId"`
}

func (Args) Kind() string { return "deploy" }

type Store struct {
	Pool           *pgxpool.Pool
	Queue          *river.Client[pgx.Tx]
	Staging        bool
	ManagedIntents bool
}

func New(pool *pgxpool.Pool) (*Store, error) {
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{Pool: pool, Queue: queue}, nil
}
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Keep a session lock across River's independently committed migrations.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(73647302)`); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock(73647302)`); err != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}
func (s *Store) Submit(ctx context.Context, owner, org string, r Request) (Operation, error) {
	if s.ManagedIntents && !s.Staging {
		return Operation{}, ErrInvalid
	}
	if r.Kind == "" {
		r.Kind = "test"
	}
	if s.Staging && r.Kind != "agent" {
		return Operation{}, ErrInvalid
	}
	if (r.Kind != "test" && r.Kind != "agent") || !namePattern.MatchString(r.Name) || r.Region != "local" || !keyPattern.MatchString(r.Key) {
		return Operation{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var authorized string
	// Serialize organization submissions to make name and key conflicts deterministic.
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2 FOR UPDATE`, org, owner).Scan(&authorized); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	} else if err != nil {
		return Operation{}, err
	}
	var id, name, region, kind string
	err = tx.QueryRow(ctx, `SELECT id,name,region,kind FROM deployments WHERE organization_id=$1 AND idempotency_key=$2`, org, r.Key).Scan(&id, &name, &region, &kind)
	if err == nil {
		if name != r.Name || region != r.Region || kind != r.Kind {
			return Operation{}, ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	if err = checkAdmission(ctx, tx); err != nil {
		return Operation{}, err
	}
	if r.Kind == "agent" {
		var pending bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM endpoint_choices WHERE organization_id=$1 AND state='awaiting_ownership')`, org).Scan(&pending); err != nil {
			return Operation{}, err
		}
		if pending {
			return Operation{}, ErrEndpointPending
		}
		if err = entitlement.CheckAllocation(ctx, tx, org); err != nil {
			return Operation{}, err
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,managed_hostname) SELECT id,$1,$2,$3,$4,$5,id::text||'.localhost' FROM (SELECT gen_random_uuid() AS id) identity RETURNING id`, org, r.Name, r.Region, r.Key, r.Kind).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Operation{}, ErrConflict
	}
	if err != nil {
		return Operation{}, err
	}
	if r.Kind == "agent" {
		if s.ManagedIntents {
			endpoint, err := managedrouting.ReserveQueuedTx(ctx, tx, "owner:"+owner, id)
			if err != nil {
				return Operation{}, err
			}
			if _, err = tx.Exec(ctx, `UPDATE deployments SET managed_hostname=$2 WHERE id=$1`, id, endpoint.Hostname); err != nil {
				return Operation{}, err
			}
		} else if _, err = tx.Exec(ctx, `UPDATE deployments SET endpoint_url=$2 WHERE id=$1`, id, nodeagent.LocalEndpoint(id)); err != nil {
			return Operation{}, err
		}
	}
	message := "Waiting for a worker."
	if s.ManagedIntents {
		message = "Managed hostname reserved. Public activation is pending."
		if _, err = tx.Exec(ctx, `UPDATE deployments SET message=$2 WHERE id=$1`, id, message); err != nil {
			return Operation{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'queued',$2)`, id, message); err != nil {
		return Operation{}, err
	}
	queue := river.QueueDefault
	if s.Staging {
		queue = StagingQueue
	}
	if s.ManagedIntents {
		queue = ManagedIntentQueue
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3, Queue: queue}); err != nil {
		return Operation{}, err
	}
	if s.ManagedIntents {
		if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
			return Operation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
func (s *Store) Get(ctx context.Context, owner, id string) (Operation, error) {
	var op Operation
	err := s.Pool.QueryRow(ctx, `SELECT d.id,d.organization_id,d.name,d.region,d.state,d.message,d.kind,d.managed_hostname,coalesce(d.endpoint_url,'') FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2`, id, owner).Scan(&op.ID, &op.OrganizationID, &op.Name, &op.Region, &op.State, &op.Message, &op.Kind, &op.ManagedHostname, &op.Endpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return op, ErrNotFound
	}
	if err != nil {
		return op, err
	}
	op.Release, err = observability.DeploymentRelease(ctx, s.Pool, id)
	if err != nil {
		return op, err
	}
	op.Service, err = observability.DeploymentStatus(ctx, s.Pool, id)
	if err != nil {
		return op, err
	}
	op.Usage, err = observability.DeploymentUsage(ctx, s.Pool, id)
	if err != nil {
		return op, err
	}
	op.Events = []Event{}
	rows, err := s.Pool.Query(ctx, `SELECT state,message,created_at FROM deployment_events WHERE deployment_id=$1 ORDER BY id`, id)
	if err != nil {
		return op, err
	}
	defer rows.Close()
	for rows.Next() {
		var event Event
		if err = rows.Scan(&event.State, &event.Message, &event.CreatedAt); err != nil {
			return op, err
		}
		op.Events = append(op.Events, event)
	}
	return op, rows.Err()
}
