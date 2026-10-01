package exports

import (
	"context"
	"strings"
	"time"
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

// PreflightMigration reads only an owned unexpired artifact. No host or target is contacted.
func (s *Store) PreflightMigration(ctx context.Context, owner, id string) (MigrationPreflight, error) {
	return s.preflightMigration(ctx, owner, id, s.Artifacts.Read)
}

func (s *Store) preflightMigration(ctx context.Context, owner, id string, read func(string, string, string) ([]byte, Manifest, error)) (MigrationPreflight, error) {
	_, manifest, before, err := s.readOwnedArchive(ctx, owner, id, read)
	if err != nil {
		return MigrationPreflight{}, err
	}
	result := MigrationPreflight{State: "archive_inventory_only", Direction: "outbound_from_retained_export", ExportID: id, DeploymentID: before.DeploymentID, DID: before.DID, CapturedAt: manifest.CapturedAt, ArchiveVersion: manifest.Version, ArchiveBytes: *before.Bytes, ArchiveSHA256: *before.SHA256, CapturedRecordCount: manifest.SignedRecords, Preferences: "not_captured_v1", Integrity: "authenticated_archive_and_member_hashes", SignatureVerification: "not_rechecked", RemainingGates: []string{"current_source_account_authority_and_recovery_options", "independent_repository_signature_and_blob_reference_verification", "authorized_target_compatibility_and_capacity", "resumable_transfer_and_independent_import_inventory", "explicit_identity_switch_authorization", "post_switch_auth_signing_and_downstream_resolution", "external_application_state_and_preferences"}}
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
