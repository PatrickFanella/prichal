package fleet

import (
	"context"
	"errors"
)

// Nodes returns registered active nodes eligible for authenticated capacity refresh.
func (s Store) Nodes(ctx context.Context, region string) ([]Placement, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,agent_endpoint,public_key_hash FROM hosting_nodes WHERE region=$1 AND state='active' AND public_key_hash IS NOT NULL ORDER BY created_at,id`, region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []Placement{}
	for rows.Next() {
		var node Placement
		if err = rows.Scan(&node.NodeID, &node.Endpoint, &node.PublicKeyHash); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// RecordCapacity subtracts retained namespaces unknown to this node's placement inventory.
func (s Store) RecordCapacity(ctx context.Context, nodeID string, maximum, cpus int, memory int64, instanceIDs []string) error {
	if maximum < 0 || maximum > 1000 || cpus < 1 || memory < 1 {
		return errors.New("invalid node measurements")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ceiling int
	if err = tx.QueryRow(ctx, `SELECT operator_slots FROM hosting_nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&ceiling); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT deployment_id FROM instance_placements WHERE node_id=$1`, nodeID)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		known[id] = true
	}
	rows.Close()
	if err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	unmanaged := 0
	seen := map[string]bool{}
	for _, id := range instanceIDs {
		if !nodeIDPattern.MatchString(id) || seen[id] {
			return errors.New("invalid node namespace inventory")
		}
		seen[id] = true
		if !known[id] {
			unmanaged++
		}
	}
	available := maximum - unmanaged
	if available < 0 {
		available = 0
	}
	if available > ceiling {
		available = ceiling
	}
	if _, err = tx.Exec(ctx, `UPDATE hosting_nodes SET slots=$2,measured_cpus=$3,measured_memory_bytes=$4,last_verified_at=now() WHERE id=$1`, nodeID, available, cpus, memory); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
