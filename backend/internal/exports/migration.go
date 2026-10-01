package exports

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

// MigrationPreflight reports retained inventory, never authority to move an identity.
type MigrationPreflight struct {
	State                 string    `json:"state"`
	Direction             string    `json:"direction"`
	ExportID              string    `json:"exportId"`
	DeploymentID          string    `json:"deploymentId"`
	DID                   string    `json:"did"`
	CapturedAt            time.Time `json:"capturedAt"`
	ArchiveVersion        int       `json:"archiveVersion"`
	ArchiveBytes          int64     `json:"archiveBytes"`
	ArchiveSHA256         string    `json:"archiveSha256"`
	RepositoryBytes       int64     `json:"repositoryBytes"`
	CapturedRecordCount   int       `json:"capturedRecordCount"`
	BlobCount             int       `json:"blobCount"`
	BlobBytes             int64     `json:"blobBytes"`
	Preferences           string    `json:"preferences"`
	Integrity             string    `json:"integrity"`
	SignatureVerification string    `json:"signatureVerification"`
	CanTransfer           bool      `json:"canTransfer"`
	CanSwitchIdentity     bool      `json:"canSwitchIdentity"`
	RemainingGates        []string  `json:"remainingGates"`
}

type migrationCheckpoint struct {
	Operation
	DID string
}

func (s *Store) migrationCheckpoint(ctx context.Context, owner, id string) (migrationCheckpoint, error) {
	var c migrationCheckpoint
	if !uuid.MatchString(id) {
		return c, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx, `SELECT e.id,e.deployment_id,e.state,e.archive_bytes,e.archive_sha256,e.captured_at,e.expires_at,COALESCE(e.state='ready' AND e.expires_at>statement_timestamp(),false),e.account_did FROM export_operations e JOIN deployments d ON d.id=e.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE e.id=$1 AND e.owner_id=$2 AND o.owner_id=$2`, id, owner).Scan(&c.ID, &c.DeploymentID, &c.State, &c.Bytes, &c.SHA256, &c.CapturedAt, &c.ExpiresAt, &c.DownloadAvailable, &c.DID)
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

// PreflightMigration reads only an owned unexpired artifact. No host or target is contacted.
func (s *Store) PreflightMigration(ctx context.Context, owner, id string) (MigrationPreflight, error) {
	return s.preflightMigration(ctx, owner, id, s.Artifacts.Read)
}

func (s *Store) preflightMigration(ctx context.Context, owner, id string, read func(string, string, string) ([]byte, Manifest, error)) (MigrationPreflight, error) {
	var result MigrationPreflight
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return result, err
	}
	before, err := s.migrationCheckpoint(ctx, owner, id)
	if err != nil {
		return result, err
	}
	plain, manifest, err := read(id, before.DeploymentID, before.DID)
	if err != nil || int64(len(plain)) != *before.Bytes || digest(plain) != *before.SHA256 || !manifest.CapturedAt.Truncate(time.Microsecond).Equal(*before.CapturedAt) || manifest.SignedRecords < 0 || manifest.SignedRecords > 10000 {
		return result, ErrUnavailable
	}
	// Archive reads may outlast cancellation, expiry, handover or a recovery hold.
	// Recheck scope and the complete checkpoint before publishing any inventory.
	if err = recovery.Check(ctx, s.Pool); err != nil {
		return result, err
	}
	after, err := s.migrationCheckpoint(ctx, owner, id)
	if err != nil {
		return result, err
	}
	if before.ID != after.ID || before.DeploymentID != after.DeploymentID || before.DID != after.DID || *before.Bytes != *after.Bytes || *before.SHA256 != *after.SHA256 || !before.CapturedAt.Equal(*after.CapturedAt) || !before.ExpiresAt.Equal(*after.ExpiresAt) {
		return result, ErrUnavailable
	}
	result = MigrationPreflight{State: "archive_inventory_only", Direction: "outbound_from_retained_export", ExportID: id, DeploymentID: before.DeploymentID, DID: before.DID, CapturedAt: manifest.CapturedAt, ArchiveVersion: manifest.Version, ArchiveBytes: *before.Bytes, ArchiveSHA256: *before.SHA256, CapturedRecordCount: manifest.SignedRecords, Preferences: "not_captured_v1", Integrity: "authenticated_archive_and_member_hashes", SignatureVerification: "not_rechecked", RemainingGates: []string{"current_source_account_authority_and_recovery_options", "independent_repository_signature_and_blob_reference_verification", "authorized_target_compatibility_and_capacity", "resumable_transfer_and_independent_import_inventory", "explicit_identity_switch_authorization", "post_switch_auth_signing_and_downstream_resolution", "external_application_state_and_preferences"}}
	for _, file := range manifest.Files {
		switch {
		case file.Name == "repo.car":
			result.RepositoryBytes = file.Bytes
		case file.Name == "preferences.json":
			result.Preferences = "captured_app_bsky_only"
		case strings.HasPrefix(file.Name, "blobs/"):
			result.BlobCount++
			result.BlobBytes += file.Bytes
		}
	}
	return result, nil
}
