package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Status is safe to expose only after authorizing the deployment's owner.
type Status struct {
	State     string     `json:"state"`
	CheckedAt *time.Time `json:"checkedAt"`
	Backup    string     `json:"backup"`
}

func DeploymentStatus(ctx context.Context, pool *pgxpool.Pool, id string) (Status, error) {
	result := Status{State: "unknown", Backup: "unconfigured"}
	var bad *bool
	if err := pool.QueryRow(ctx, `SELECT max(last_observed_at),bool_or(active) FROM service_observations WHERE deployment_id=$1 AND kind='runtime_unhealthy'`, id).Scan(&result.CheckedAt, &bad); err != nil {
		return result, err
	}
	if result.CheckedAt == nil || bad == nil {
		return result, nil
	}
	result.State = "healthy"
	if *bad {
		result.State = "degraded"
	}
	if time.Since(*result.CheckedAt) > time.Minute {
		result.State = "stale"
	}
	return result, nil
}
