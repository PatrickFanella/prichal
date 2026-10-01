// Package entitlement controls deployment allocation independently of payment delivery and PDS data.
package entitlement

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = errors.New("an active hosting entitlement is required")
var ErrLimit = errors.New("native deployment allowance exhausted")
var ErrConflict = errors.New("entitlement request key reused with different payload")
var ErrNotFound = errors.New("organization not found")
var ErrInvalid = errors.New("invalid pilot entitlement request")
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Request is an explicit local operator exception, not proof of payment.
type Request struct {
	Key    string `json:"idempotencyKey"`
	State  string `json:"state"`
	Plan   string `json:"plan"`
	Hours  int    `json:"durationHours"`
	Reason string `json:"reason"`
}

// Operation records who applied an exception; customer views omit operator identity.
type Operation struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Generation     int64  `json:"generation"`
	Request
	Actor     string     `json:"actor"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// Status reports conservative retained native allocation, never a storage or account quota.
type Status struct {
	State              string     `json:"state"`
	Source             string     `json:"source"`
	Plan               string     `json:"plan"`
	InstanceLimit      int        `json:"instanceLimit"`
	AllocatedInstances int        `json:"allocatedInstances"`
	AvailableInstances int        `json:"availableInstances"`
	CanDeploy          bool       `json:"canDeploy"`
	ExpiresAt          *time.Time `json:"expiresAt"`
}

// Store persists local operator exceptions. Paid entitlement adapters remain unconfigured.
type Store struct{ Pool *pgxpool.Pool }

func limit(plan string) int {
	switch plan {
	case "local-single":
		return 1
	case "local-small":
		return 3
	}
	return 0
}
func valid(r Request) bool {
	return uuid.MatchString(r.Key) && (r.Reason == "pilot" || r.Reason == "qualification") && ((r.State == "active" && limit(r.Plan) > 0 && r.Hours >= 1 && r.Hours <= 720) || (r.State == "revoked" && r.Plan == "" && r.Hours == 0))
}

// Submit serializes with deployment submission using the same organization row lock.
func (s Store) Submit(ctx context.Context, actor, org string, r Request) (Operation, error) {
	if !valid(r) || !uuid.MatchString(org) || actor == "" || len(actor) > 100 {
		return Operation{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Operation{}, ErrNotFound
		}
		return Operation{}, err
	}
	prior, err := readOperation(ctx, tx, org, r.Key)
	if err == nil {
		if prior.Request != r || prior.Actor != actor {
			return Operation{}, ErrConflict
		}
		return prior, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO pilot_entitlement_operations(organization_id,idempotency_key,generation,state,plan,duration_hours,reason,actor,expires_at) SELECT $1,$2,coalesce(max(generation),0)+1,$3,nullif($4,''),$5,$6,$7,CASE WHEN $3='active' THEN now()+make_interval(hours=>$5) ELSE NULL END FROM pilot_entitlement_operations WHERE organization_id=$1 RETURNING id`, org, r.Key, r.State, r.Plan, r.Hours, r.Reason, actor).Scan(&id)
	if err != nil {
		return Operation{}, err
	}
	op, err := readOperation(ctx, tx, org, r.Key)
	if err != nil {
		return Operation{}, err
	}
	return op, tx.Commit(ctx)
}
func readOperation(ctx context.Context, tx pgx.Tx, org, key string) (Operation, error) {
	var op Operation
	err := tx.QueryRow(ctx, `SELECT id,organization_id,generation,idempotency_key,state,coalesce(plan,''),duration_hours,reason,actor,created_at,expires_at FROM pilot_entitlement_operations WHERE organization_id=$1 AND idempotency_key=$2`, org, key).Scan(&op.ID, &op.OrganizationID, &op.Generation, &op.Key, &op.State, &op.Plan, &op.Hours, &op.Reason, &op.Actor, &op.CreatedAt, &op.ExpiresAt)
	return op, err
}
func status(ctx context.Context, tx pgx.Tx, org string) (Status, error) {
	result := Status{State: "unconfigured", Source: "none"}
	var now time.Time
	err := tx.QueryRow(ctx, `SELECT state,coalesce(plan,''),expires_at,clock_timestamp() FROM pilot_entitlement_operations WHERE organization_id=$1 ORDER BY generation DESC LIMIT 1`, org).Scan(&result.State, &result.Plan, &result.ExpiresAt, &now)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if err == nil {
		result.Source = "local_pilot"
		result.InstanceLimit = limit(result.Plan)
		if result.State == "active" && !now.Before(*result.ExpiresAt) {
			result.State = "expired"
		}
	}
	// Failed requests may own uncertain or retained resources. They are not released implicitly.
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM deployments WHERE organization_id=$1 AND kind='agent'`, org).Scan(&result.AllocatedInstances); err != nil {
		return result, err
	}
	if result.State == "active" {
		result.AvailableInstances = max(0, result.InstanceLimit-result.AllocatedInstances)
		result.CanDeploy = result.AvailableInstances > 0
	}
	return result, nil
}

// CheckAllocation must run inside a transaction that has locked the organization row.
// It applies only to new native intents, after stable-key replay has been resolved.
func CheckAllocation(ctx context.Context, tx pgx.Tx, org string) error {
	result, err := status(ctx, tx, org)
	if err != nil {
		return err
	}
	if result.State != "active" {
		return ErrDenied
	}
	if !result.CanDeploy {
		return ErrLimit
	}
	return nil
}

// Get is owner scoped and does not expose internal grant actor or audit keys.
func (s Store) Get(ctx context.Context, owner, org string) (Status, error) {
	if !uuid.MatchString(org) {
		return Status{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2 FOR SHARE`, org, owner).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Status{}, ErrNotFound
		}
		return Status{}, err
	}
	result, err := status(ctx, tx, org)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
