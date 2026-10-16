package observability

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
)

type ReleaseStatus struct {
	State     string                    `json:"state"`
	CheckedAt *time.Time                `json:"checkedAt"`
	Runtime   *nodeagent.RuntimeRelease `json:"runtime"`
}

type releaseSample struct {
	id, node  string
	checkedAt time.Time
	runtime   *nodeagent.RuntimeRelease
}

// DeploymentRelease is called only inside an authorized owner/operator projection.
func DeploymentRelease(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) (ReleaseStatus, error) {
	result := ReleaseStatus{State: "unknown"}
	var reference, imageID *string
	var version string
	err := db.QueryRow(ctx, `SELECT r.image_ref,r.image_id,coalesce(r.version,''),r.checked_at FROM instance_runtime_releases r JOIN instance_placements p ON p.deployment_id=r.deployment_id AND p.node_id=r.node_id JOIN deployments d ON d.id=r.deployment_id WHERE r.deployment_id=$1 AND d.kind='agent' AND d.state='ready'`, id).Scan(&reference, &imageID, &version, &result.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if reference != nil && imageID != nil {
		result.Runtime = nodeagent.ParseRuntimeRelease(*reference, *imageID, version)
	}
	if result.Runtime != nil {
		result.State = "observed"
	}
	if result.CheckedAt != nil && time.Since(*result.CheckedAt) > time.Minute {
		result.State = "stale"
	}
	return result, nil
}
