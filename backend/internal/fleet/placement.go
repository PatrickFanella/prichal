package fleet

import (
	"context"
	"errors"
	"slices"
	"strings"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
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
	rows, err := tx.Query(ctx, `SELECT id,agent_endpoint,public_key_hash,slots FROM hosting_nodes WHERE region=$1 AND state='active' AND public_key_hash IS NOT NULL AND last_verified_at>now()-interval '1 minute' ORDER BY created_at,id FOR UPDATE SKIP LOCKED`, region)
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
	port := loopbackPort(deploymentID)
	for _, node := range candidates {
		placed, err := placedDeployments(ctx, tx, node.NodeID)
		if err != nil {
			return Placement{}, err
		}
		if len(placed) >= node.Slots || port == "" || slices.ContainsFunc(placed, func(id string) bool { return loopbackPort(id) == port }) {
			// A same-node loopback port collision would leave the second edge unable to start.
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

func placedDeployments(ctx context.Context, tx pgx.Tx, node string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT deployment_id::text FROM instance_placements WHERE node_id=$1`, node)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// loopbackPort is the agent's fixed private ingress port for a deployment, derived from its ID.
func loopbackPort(id string) string {
	endpoint := nodeagent.LocalEndpoint(id)
	if endpoint == "" {
		return ""
	}
	return endpoint[strings.LastIndex(endpoint, ":")+1:]
}
