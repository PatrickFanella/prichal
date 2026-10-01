package exports

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

type archiveCheckpoint struct {
	Operation
	DID string
}

func (s *Store) archiveCheckpoint(ctx context.Context, owner, id string) (archiveCheckpoint, error) {
	var c archiveCheckpoint
	if !uuid.MatchString(id) {
		return c, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx, `SELECT e.id,e.deployment_id,e.state,e.archive_bytes,e.archive_sha256,e.captured_at,e.expires_at,COALESCE(e.state='ready' AND e.expires_at>statement_timestamp() AND NOT EXISTS(SELECT 1 FROM management_recovery WHERE state='held'),false),e.account_did FROM export_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2`, id, owner).Scan(&c.ID, &c.DeploymentID, &c.State, &c.Bytes, &c.SHA256, &c.CapturedAt, &c.ExpiresAt, &c.DownloadAvailable, &c.DID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if !c.DownloadAvailable || c.Bytes == nil || c.SHA256 == nil || c.CapturedAt == nil || c.ExpiresAt == nil {
		return c, ErrUnavailable
	}
	return c, nil
}

// readOwnedArchive authenticates retained custody and rechecks current disclosure authority.
func (s *Store) readOwnedArchive(ctx context.Context, owner, id string, read func(string, string, string) ([]byte, Manifest, error)) ([]byte, Manifest, archiveCheckpoint, error) {
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return nil, Manifest{}, archiveCheckpoint{}, err
	}
	before, err := s.archiveCheckpoint(ctx, owner, id)
	if err != nil {
		return nil, Manifest{}, archiveCheckpoint{}, err
	}
	plain, manifest, err := read(id, before.DeploymentID, before.DID)
	if err != nil || int64(len(plain)) != *before.Bytes || digest(plain) != *before.SHA256 || !manifest.CapturedAt.Truncate(time.Microsecond).Equal(*before.CapturedAt) || manifest.SignedRecords < 0 || manifest.SignedRecords > 10000 {
		return nil, Manifest{}, archiveCheckpoint{}, ErrUnavailable
	}
	// Archive reads may outlast cancellation, expiry, handover or a recovery hold.
	// Recheck scope and the complete checkpoint before disclosing any content or inventory.
	if err = recovery.Check(ctx, s.Pool); err != nil {
		return nil, Manifest{}, archiveCheckpoint{}, err
	}
	after, err := s.archiveCheckpoint(ctx, owner, id)
	if err != nil {
		return nil, Manifest{}, archiveCheckpoint{}, err
	}
	if before.ID != after.ID || before.DeploymentID != after.DeploymentID || before.DID != after.DID || *before.Bytes != *after.Bytes || *before.SHA256 != *after.SHA256 || !before.CapturedAt.Equal(*after.CapturedAt) || !before.ExpiresAt.Equal(*after.ExpiresAt) {
		return nil, Manifest{}, archiveCheckpoint{}, ErrUnavailable
	}
	return plain, manifest, before, nil
}
