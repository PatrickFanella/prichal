package exports

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var ErrNotFound = errors.New("export reference not found")
var ErrLimit = errors.New("local retained export limit reached")
var ErrUnavailable = errors.New("portable export unavailable")

// Args queues an export reference only, never native credentials or content.
type Args struct {
	ExportID string `json:"exportId"`
}

func (Args) Kind() string { return "portable_export" }

// Operation contains owner-safe export progress and content integrity metadata.
type Operation struct {
	ID                string     `json:"id"`
	DeploymentID      string     `json:"deploymentId"`
	State             string     `json:"state"`
	Bytes             *int64     `json:"bytes"`
	SHA256            *string    `json:"sha256"`
	CapturedAt        *time.Time `json:"capturedAt"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	DownloadAvailable bool       `json:"downloadAvailable"`
}

// Store atomically retains customer intent and work without host credentials.
type Store struct {
	Pool      *pgxpool.Pool
	Queue     *river.Client[pgx.Tx]
	Artifacts *Artifacts
}

// New creates the export producer with separate artifact custody.
func New(pool *pgxpool.Pool, artifacts *Artifacts) (*Store, error) {
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Store{Pool: pool, Queue: queue, Artifacts: artifacts}, nil
}

// Submit preserves stable retries and limits new retained local requests.
func (s *Store) Submit(ctx context.Context, owner, deployment, key string) (Operation, error) {
	if !uuid.MatchString(deployment) || !uuid.MatchString(key) {
		return Operation{}, ErrNotFound
	}
	if s.Artifacts == nil {
		return Operation{}, ErrUnavailable
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return Operation{}, err
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
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM export_operations WHERE deployment_id=$1 AND idempotency_key=$2 AND owner_id=$3`, deployment, key, owner).Scan(&id)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM export_operations WHERE deployment_id=$1`, deployment).Scan(&count); err != nil {
		return Operation{}, err
	}
	if count >= 3 {
		return Operation{}, ErrLimit
	}
	var did string
	err = tx.QueryRow(ctx, `SELECT account_did FROM enrollment_operations WHERE deployment_id=$1 AND state='verified' ORDER BY created_at DESC LIMIT 1`, deployment).Scan(&did)
	if err != nil {
		return Operation{}, ErrUnavailable
	}
	if err = tx.QueryRow(ctx, `INSERT INTO export_operations(deployment_id,owner_id,idempotency_key,account_did) VALUES($1,$2,$3,$4) RETURNING id`, deployment, owner, key, did).Scan(&id); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO export_events(export_id,state) VALUES($1,'queued')`, id); err != nil {
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

// List returns bounded owner-authorized export progress.
func (s *Store) List(ctx context.Context, owner, deployment string) ([]Operation, error) {
	if !uuid.MatchString(deployment) {
		return nil, ErrNotFound
	}
	var found string
	if err := s.Pool.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2`, deployment, owner).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM export_operations WHERE deployment_id=$1 AND owner_id=$2 ORDER BY created_at DESC LIMIT 3`, deployment, owner)
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
		item, err := s.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// Get rechecks current organization ownership and reveals no storage path or encryption key.
func (s *Store) Get(ctx context.Context, owner, id string) (Operation, error) {
	if !uuid.MatchString(id) {
		return Operation{}, ErrNotFound
	}
	var result Operation
	err := s.Pool.QueryRow(ctx, `SELECT e.id,e.deployment_id,e.state,e.archive_bytes,e.archive_sha256,e.captured_at,e.expires_at,COALESCE(e.state='ready' AND e.expires_at>clock_timestamp(),false) FROM export_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2`, id, owner).Scan(&result.ID, &result.DeploymentID, &result.State, &result.Bytes, &result.SHA256, &result.CapturedAt, &result.ExpiresAt, &result.DownloadAvailable)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	return result, err
}

// Download authenticates the retained artifact and its management checkpoint before disclosure.
func (s *Store) Download(ctx context.Context, owner, id string) ([]byte, error) {
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return nil, err
	}
	operation, err := s.Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if !operation.DownloadAvailable || operation.SHA256 == nil || operation.Bytes == nil {
		return nil, ErrUnavailable
	}
	var did string
	if err = s.Pool.QueryRow(ctx, `SELECT account_did FROM export_operations WHERE id=$1 AND owner_id=$2`, id, owner).Scan(&did); err != nil {
		return nil, err
	}
	plain, _, err := s.Artifacts.Read(id, operation.DeploymentID, did)
	if err != nil || int64(len(plain)) != *operation.Bytes || digest(plain) != *operation.SHA256 {
		return nil, ErrUnavailable
	}
	return plain, nil
}
