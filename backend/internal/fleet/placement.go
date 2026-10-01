package fleet

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrCapacity means no active registered node has an unreserved slot.
var ErrCapacity = errors.New("no registered node has available capacity")

// Placement binds one deployment to a verified node, including its public key pin.
type Placement struct {
	NodeID        string
	Endpoint      string
	PublicKeyHash []byte
}

// Reserve serializes per-deployment placement and node capacity without deleting reservations on uncertainty.
func (s Store) Reserve(ctx context.Context, deploymentID string) (Placement, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Placement{}, err
	}
	defer tx.Rollback(ctx)
	var region string
	if err = tx.QueryRow(ctx, `SELECT region FROM deployments WHERE id=$1 FOR UPDATE`, deploymentID).Scan(&region); err != nil {
		return Placement{}, err
	}
	var placement Placement
	err = tx.QueryRow(ctx, `SELECT n.id,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1`, deploymentID).Scan(&placement.NodeID, &placement.Endpoint, &placement.PublicKeyHash)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Placement{}, err
		}
		return placement, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Placement{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id,agent_endpoint,public_key_hash,slots FROM hosting_nodes WHERE region=$1 AND state='active' AND public_key_hash IS NOT NULL ORDER BY created_at,id FOR UPDATE SKIP LOCKED`, region)
	if err != nil {
		return Placement{}, err
	}
	type candidate struct {
		Placement
		Slots int
	}
	candidates := []candidate{}
	for rows.Next() {
		var node candidate
		if err = rows.Scan(&node.NodeID, &node.Endpoint, &node.PublicKeyHash, &node.Slots); err != nil {
			break
		}
		candidates = append(candidates, node)
	}
	rows.Close()
	if err != nil {
		return Placement{}, err
	}
	if err = rows.Err(); err != nil {
		return Placement{}, err
	}
	for _, node := range candidates {
		var reserved int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE node_id=$1`, node.NodeID).Scan(&reserved); err != nil {
			return Placement{}, err
		}
		if reserved >= node.Slots {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, deploymentID, node.NodeID); err != nil {
			return Placement{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return Placement{}, err
		}
		return node.Placement, nil
	}
	return Placement{}, ErrCapacity
}
