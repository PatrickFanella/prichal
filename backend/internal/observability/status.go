package observability

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status is safe to expose only after authorizing the deployment's owner.
type Status struct {
	State         string     `json:"state"`
	Lifecycle     string     `json:"lifecycle"`
	Account       string     `json:"account"`
	AccountReason string     `json:"accountReason"`
	Reason        string     `json:"reason"`
	CheckedAt     *time.Time `json:"checkedAt"`
	Backup        string     `json:"backup"`
}

func DeploymentStatus(ctx context.Context, pool *pgxpool.Pool, id string) (Status, error) {
	result := Status{State: "unknown", Backup: "unconfigured", Lifecycle: "running", Account: "running"}
	var desired, state string
	err := pool.QueryRow(ctx, `SELECT desired_state,state,reason FROM service_operations WHERE deployment_id=$1 AND kind='service' ORDER BY generation DESC LIMIT 1`, id).Scan(&desired, &state, &result.Reason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if err == nil {
		result.Lifecycle = state
		if state == "applied" {
			result.Lifecycle = desired
		}
	}

	var accountDesired, accountState string
	err = pool.QueryRow(ctx, `SELECT desired_state,state,reason FROM service_operations WHERE deployment_id=$1 AND kind='account' ORDER BY generation DESC LIMIT 1`, id).Scan(&accountDesired, &accountState, &result.AccountReason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if err == nil {
		result.Account = accountState
		if accountState == "applied" {
			result.Account = accountDesired
		}
	}
	var bad *bool
	if err := pool.QueryRow(ctx, `SELECT max(last_observed_at),bool_or(active) FROM service_observations WHERE deployment_id=$1 AND kind='runtime_unhealthy'`, id).Scan(&result.CheckedAt, &bad); err != nil {
		return result, err
	}
	if result.CheckedAt == nil || bad == nil {
		return result, nil
	}
	result.State = "healthy"
	if result.Lifecycle == "suspended" {
		result.State = "suspended"
	}
	if result.Lifecycle == "pending" || result.Lifecycle == "failed" {
		result.State = "degraded"
	}
	if *bad {
		result.State = "degraded"
	}
	if time.Since(*result.CheckedAt) > time.Minute {
		result.State = "stale"
	}
	return result, nil
}

// UsageStatus exposes tenant counters after owner authorization; shared host headroom is operator-only.
type UsageStatus struct {
	State        string     `json:"state"`
	CheckedAt    *time.Time `json:"checkedAt"`
	DataBytes    int64      `json:"dataBytes"`
	BlobBytes    int64      `json:"blobBytes"`
	Accounts     int64      `json:"accounts"`
	StorageLimit string     `json:"storageLimit"`
}

func DeploymentUsage(ctx context.Context, pool *pgxpool.Pool, id string) (UsageStatus, error) {
	result := UsageStatus{State: "unknown", StorageLimit: "unconfigured"}
	var unavailable bool
	err := pool.QueryRow(ctx, `SELECT data_bytes,blob_bytes,accounts,measured_at,EXISTS(SELECT 1 FROM service_observations WHERE deployment_id=$1 AND kind='usage_unavailable' AND active) FROM instance_usage WHERE deployment_id=$1`, id).Scan(&result.DataBytes, &result.BlobBytes, &result.Accounts, &result.CheckedAt, &unavailable)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.State = "observed"
	if unavailable {
		result.State = "unavailable"
	}
	if result.CheckedAt == nil || time.Since(*result.CheckedAt) > time.Minute {
		result.State = "stale"
	}
	return result, nil
}
