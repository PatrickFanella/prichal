package fleet

import (
	"context"
	"errors"
	"strings"
)

// SetCapacityCeiling records an operator policy, invalidating placement until authenticated measurements refresh.
func (s Store) SetCapacityCeiling(ctx context.Context, node, actor, reason string, ceiling int) error {
	if !nodeIDPattern.MatchString(node) || len(actor) < 1 || len(actor) > 128 || strings.TrimSpace(actor) != actor || ceiling < 1 || ceiling > 8 {
		return errors.New("invalid local capacity policy")
	}
	switch reason {
	case "local_isolation_qualification", "planned_capacity_change", "capacity_reduction":
	default:
		return errors.New("use a supported capacity policy reason")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var previous int
	if err = tx.QueryRow(ctx, `SELECT operator_slots FROM hosting_nodes WHERE id=$1 AND state IN ('active','draining') FOR UPDATE`, node).Scan(&previous); err != nil {
		return errors.New("registered local node required")
	}
	if previous == ceiling {
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE hosting_nodes SET operator_slots=$2,slots=LEAST(slots,$2),last_verified_at=NULL WHERE id=$1`, node, ceiling); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hosting_node_policy_events(node_id,actor,reason,previous_slots,requested_slots) VALUES($1,$2,$3,$4,$5)`, node, actor, reason, previous, ceiling); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
