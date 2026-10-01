// Package enrollment persists owner-scoped first-account jobs; it never executes a host command.
package enrollment

import (
	"context"
	"errors"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var ErrNotFound = errors.New("enrollment not found")
var ErrConflict = errors.New("enrollment is already active or recovery limit reached")
var ErrInvalid = errors.New("invalid enrollment request")
var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Args struct {
	OperationID string `json:"operationId"`
}

func (Args) Kind() string { return "first_account_enrollment" }

type Request struct {
	Key     string `json:"idempotencyKey"`
	Recover bool   `json:"recover"`
}
type Event struct {
	State     string    `json:"state"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}
type Operation struct {
	ID             string     `json:"id"`
	DeploymentID   string     `json:"deploymentId"`
	State          string     `json:"state"`
	Message        string     `json:"message"`
	Code           string     `json:"code,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	AccountURL     string     `json:"accountUrl"`
	DID            string     `json:"did,omitempty"`
	Handle         string     `json:"handle,omitempty"`
	EmailConfirmed bool       `json:"emailConfirmed"`
	Events         []Event    `json:"events"`
}
type Store struct {
	Pool   *pgxpool.Pool
	Cipher *Cipher
	Queue  *river.Client[pgx.Tx]
}

func New(pool *pgxpool.Pool, cipher *Cipher) (*Store, error) {
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{pool, cipher, queue}, nil
}
func (s *Store) Submit(ctx context.Context, owner, deployment string, request Request) (Operation, error) {
	if s.Cipher == nil || !uuid.MatchString(owner) || !uuid.MatchString(deployment) || !uuid.MatchString(request.Key) {
		return Operation{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var organization, state, kind string
	if err = tx.QueryRow(ctx, `SELECT d.organization_id::text,d.state,d.kind FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2 FOR UPDATE OF d`, deployment, owner).Scan(&organization, &state, &kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Operation{}, ErrNotFound
		}
		return Operation{}, err
	}
	if state != "ready" || kind != "agent" {
		return Operation{}, ErrConflict
	}
	var id string
	var recoveryRequested bool
	err = tx.QueryRow(ctx, `SELECT id::text,recovery_requested FROM enrollment_operations WHERE deployment_id=$1 AND idempotency_key=$2 AND owner_id=$3`, deployment, request.Key, owner).Scan(&id, &recoveryRequested)
	if err == nil {
		if request.Recover != recoveryRequested {
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
	var latest string
	err = tx.QueryRow(ctx, `SELECT state FROM enrollment_operations WHERE deployment_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, deployment).Scan(&latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	if latest == "account_created" || latest == "verified" || latest == "pending" {
		return Operation{}, ErrConflict
	}
	if latest != "" && !request.Recover {
		return Operation{}, ErrConflict
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM enrollment_operations WHERE deployment_id=$1 AND created_at>now()-interval '1 hour'`, deployment).Scan(&count); err != nil {
		return Operation{}, err
	}
	if count >= 3 {
		return Operation{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) SELECT id,'superseded','Replaced by an owner-authorized recovery request.' FROM enrollment_operations WHERE deployment_id=$1 AND state IN ('invitation_ready','expired','failed')`, deployment); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET state='superseded',code_ciphertext=NULL,message='Replaced by an owner-authorized recovery request.',updated_at=now() WHERE deployment_id=$1 AND state IN ('invitation_ready','expired','failed')`, deployment); err != nil {
		return Operation{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,recovery_requested) VALUES($1,$2,$3,$4) RETURNING id::text`, deployment, owner, request.Key, request.Recover).Scan(&id); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) VALUES($1,'pending','Preparing a first-account invitation.')`, id); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
func (s *Store) Get(ctx context.Context, owner, id string) (Operation, error) {
	var op Operation
	var encoded []byte
	var organization, endpoint string
	err := s.Pool.QueryRow(ctx, `SELECT e.id::text,e.deployment_id::text,e.state,e.message,e.code_ciphertext,e.expires_at,coalesce(e.account_did,''),coalesce(e.account_handle,''),e.email_confirmed,d.organization_id::text,d.endpoint_url FROM enrollment_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2`, id, owner).Scan(&op.ID, &op.DeploymentID, &op.State, &op.Message, &encoded, &op.ExpiresAt, &op.DID, &op.Handle, &op.EmailConfirmed, &organization, &endpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return op, ErrNotFound
	}
	if err != nil {
		return op, err
	}
	if endpoint != nodeagent.LocalEndpoint(op.DeploymentID) {
		return Operation{}, errors.New("enrollment endpoint differs from managed identity")
	}
	op.AccountURL = endpoint + "/account"
	if op.State == "invitation_ready" {
		if op.ExpiresAt == nil || !op.ExpiresAt.After(time.Now()) {
			op.State = "expired"
			op.Message = "This invitation expired. Request recovery to replace it."
		} else {
			op.Code, err = s.Cipher.open(encoded, op.ID+"/"+op.DeploymentID+"/"+organization)
			if err != nil {
				return Operation{}, err
			}
		}
	}
	op.Events = []Event{}
	rows, err := s.Pool.Query(ctx, `SELECT state,message,created_at FROM enrollment_events WHERE enrollment_id=$1 ORDER BY id LIMIT 100`, id)
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
func (s *Store) Latest(ctx context.Context, owner, deployment string) (Operation, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT e.id::text FROM enrollment_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND e.owner_id=$2 AND o.owner_id=$2 ORDER BY e.created_at DESC,e.id DESC LIMIT 1`, deployment, owner).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
func (s *Store) Refresh(ctx context.Context, owner, id string) (Operation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	var refreshed *time.Time
	err = tx.QueryRow(ctx, `SELECT e.state,e.last_refresh_at FROM enrollment_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2 FOR UPDATE OF e`, id, owner).Scan(&state, &refreshed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	if state == "pending" {
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, id)
	}
	if state == "superseded" || state == "failed" {
		return Operation{}, ErrConflict
	}
	if refreshed != nil && time.Since(*refreshed) < 5*time.Second {
		return Operation{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET last_refresh_at=now() WHERE id=$1`, id); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}
