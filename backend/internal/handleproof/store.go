package handleproof

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("handle configuration not found")
var ErrConflict = errors.New("confirmed account or bounded recheck required")

type Configuration struct {
	DeploymentID string     `json:"deploymentId"`
	Handle       string     `json:"handle"`
	DID          string     `json:"did"`
	Endpoint     string     `json:"endpoint"`
	TXTName      string     `json:"txtName"`
	TXTValue     string     `json:"txtValue"`
	State        string     `json:"state"`
	Message      string     `json:"message"`
	CheckedAt    *time.Time `json:"checkedAt"`
	Revision     int64      `json:"revision"`
}
type Store struct {
	Pool    *pgxpool.Pool
	Checker Checker
}

func (s *Store) account(ctx context.Context, owner, deployment string) (string, string, error) {
	var did, endpoint string
	err := s.Pool.QueryRow(ctx, `SELECT e.account_did,d.endpoint_url FROM deployments d JOIN organizations o ON o.id=d.organization_id JOIN LATERAL (SELECT state,account_did,owner_id FROM enrollment_operations WHERE deployment_id=d.id ORDER BY created_at DESC,id DESC LIMIT 1) e ON true WHERE d.id=$1 AND o.owner_id=$2 AND e.owner_id=$2 AND d.kind='agent' AND d.state='ready' AND e.state='verified'`, deployment, owner).Scan(&did, &endpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return did, endpoint, err
}
func (s *Store) Get(ctx context.Context, owner, deployment string) (Configuration, error) {
	did, endpoint, err := s.account(ctx, owner, deployment)
	if err != nil {
		return Configuration{}, err
	}
	var value Configuration
	value.DeploymentID = deployment
	value.DID = did
	value.Endpoint = endpoint
	err = s.Pool.QueryRow(ctx, `SELECT handle,state,message,checked_at,revision FROM handle_configurations WHERE deployment_id=$1 AND owner_id=$2`, deployment, owner).Scan(&value.Handle, &value.State, &value.Message, &value.CheckedAt, &value.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrNotFound
	}
	value.TXTName = "_atproto." + value.Handle
	value.TXTValue = "did=" + did
	return value, err
}

// Set records domain intent. It never signs or submits a native handle change.
func (s *Store) Set(ctx context.Context, owner, deployment, handle string) (Configuration, error) {
	normalized, err := Normalize(handle, s.Checker.Local)
	if err != nil {
		return Configuration{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Configuration{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize domain changes with deployment lifecycle transitions.
	var id string
	if err = tx.QueryRow(ctx, `SELECT d.id::text FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2 FOR UPDATE OF d`, deployment, owner).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Configuration{}, ErrNotFound
		}
		return Configuration{}, err
	}
	if _, _, err = s.account(ctx, owner, deployment); err != nil {
		return Configuration{}, err
	}
	const message = "Publish the TXT record, then check again after DNS propagates."
	var changed string
	err = tx.QueryRow(ctx, `INSERT INTO handle_configurations(deployment_id,owner_id,handle,state,message) VALUES($1,$2,$3,'pending_dns',$4) ON CONFLICT(deployment_id) DO UPDATE SET owner_id=$2,handle=$3,revision=handle_configurations.revision+1,state='pending_dns',message=$4,updated_at=now() WHERE handle_configurations.handle<>$3 RETURNING handle`, deployment, owner, normalized, message).Scan(&changed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, err
	}
	if err == nil {
		if _, err = tx.Exec(ctx, `INSERT INTO handle_events(deployment_id,handle,state,message) VALUES($1,$2,'pending_dns',$3)`, deployment, normalized, message); err != nil {
			return Configuration{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Configuration{}, err
	}
	return s.Get(ctx, owner, deployment)
}

// Check reserves a bounded recheck, then discards results if the owner changed domain intent.
func (s *Store) Check(ctx context.Context, owner, deployment string) (Configuration, error) {
	value, err := s.Get(ctx, owner, deployment)
	if err != nil {
		return Configuration{}, err
	}
	result, err := s.Pool.Exec(ctx, `UPDATE handle_configurations SET checked_at=now(),state='checking',message='Checking domain and identity claims.' WHERE deployment_id=$1 AND owner_id=$2 AND revision=$3 AND (checked_at IS NULL OR checked_at<now()-interval '30 seconds')`, deployment, owner, value.Revision)
	if err != nil {
		return Configuration{}, err
	}
	if result.RowsAffected() != 1 {
		return Configuration{}, ErrConflict
	}
	proof := s.Checker.Check(ctx, value.Handle, value.DID, value.Endpoint)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Configuration{}, err
	}
	defer tx.Rollback(ctx)
	result, err = tx.Exec(ctx, `UPDATE handle_configurations SET state=$4,message=$5,updated_at=now() WHERE deployment_id=$1 AND owner_id=$2 AND revision=$3`, deployment, owner, value.Revision, proof.State, proof.Message)
	if err != nil {
		return Configuration{}, err
	}
	if result.RowsAffected() != 1 {
		return Configuration{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO handle_events(deployment_id,handle,state,message) VALUES($1,$2,$3,$4)`, deployment, value.Handle, proof.State, proof.Message); err != nil {
		return Configuration{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Configuration{}, err
	}
	return s.Get(ctx, owner, deployment)
}
