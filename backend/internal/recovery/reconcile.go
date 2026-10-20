package recovery

import (
	"context"
	"errors"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Observer can only inspect registered hosts, never create or start a runtime.
type Observer interface {
	Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error)
	Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
}

// Reconcile releases the hold only when every registered host inventory matches
// retained placements, and every placed instance matches its confirmed service intent.
// Unknown namespaces or missing hosts require operator investigation. Nothing is deleted.
func Reconcile(ctx context.Context, pool *pgxpool.Pool, observer Observer) error {
	if observer == nil {
		return errors.New("authenticated host observer required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM management_recovery WHERE singleton=true FOR UPDATE`).Scan(&state); err != nil {
		return errors.New("recovery hold required")
	}
	if state != "held" {
		return errors.New("recovery is not held")
	}
	rows, err := tx.Query(ctx, `SELECT id::text,agent_endpoint,public_key_hash FROM hosting_nodes WHERE enrolled_at IS NOT NULL ORDER BY id FOR UPDATE`)
	if err != nil {
		return err
	}
	var peers []nodeagent.Peer
	for rows.Next() {
		var peer nodeagent.Peer
		if err = rows.Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
			rows.Close()
			return err
		}
		peers = append(peers, peer)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var total, verified int
	verifiedHosts := map[string]nodeagent.Peer{}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM instance_placements`).Scan(&total); err != nil {
		return err
	}
	for _, peer := range peers {
		capacity, err := observer.Capacity(ctx, peer)
		if err != nil {
			return errors.New("registered host inventory could not be verified")
		}
		inventory := map[string]bool{}
		for _, id := range capacity.InstanceIDs {
			if inventory[id] {
				return errors.New("duplicate host namespace")
			}
			inventory[id] = true
		}
		placements, err := tx.Query(ctx, `SELECT deployment_id::text FROM instance_placements WHERE node_id=$1 ORDER BY deployment_id FOR UPDATE`, peer.NodeID)
		if err != nil {
			return err
		}
		var ids []string
		for placements.Next() {
			var id string
			if err = placements.Scan(&id); err != nil {
				placements.Close()
				return err
			}
			ids = append(ids, id)
		}
		placements.Close()
		if err = placements.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if !inventory[id] {
				return errors.New("placed namespace missing from host inventory")
			}
			var generation int64
			var phase, desired, ticket string
			if err = tx.QueryRow(ctx, `SELECT coalesce(c.generation,0),coalesce(c.state,'applied'),coalesce(c.desired_state,'running'),coalesce(c.id::text,'') FROM deployments d LEFT JOIN LATERAL (SELECT id,generation,state,desired_state FROM service_operations WHERE deployment_id=d.id AND kind='service' ORDER BY generation DESC LIMIT 1) c ON true WHERE d.id=$1`, id).Scan(&generation, &phase, &desired, &ticket); err != nil {
				return err
			}
			if phase != "applied" {
				return errors.New("uncertain service lifecycle prevents recovery release")
			}
			result, err := observer.Status(ctx, peer, id)
			expected := result.State == "running" && result.Healthy
			if desired == "suspended" {
				expected = result.State == "suspended" && !result.Healthy
			}
			metadata := result.ServiceGeneration == generation && result.ServiceTicket == ticket
			if generation > 0 {
				metadata = metadata && result.ServicePhase == "applied"
			} else {
				metadata = metadata && result.ServicePhase == ""
			}
			if err != nil || result.ID != id || result.Endpoint != nodeagent.LocalEndpoint(id) || !expected || !metadata {
				return errors.New("placed runtime does not match confirmed service intent")
			}

			var accountGeneration int64
			var accountPhase, accountDesired, accountTicket, expectedAccountDID string
			accountErr := tx.QueryRow(ctx, `SELECT generation,state,desired_state,id::text,coalesce((SELECT account_did FROM enrollment_operations WHERE deployment_id=$1 AND state='verified' ORDER BY created_at DESC LIMIT 1),'') FROM service_operations WHERE deployment_id=$1 AND kind='account' ORDER BY generation DESC LIMIT 1`, id).Scan(&accountGeneration, &accountPhase, &accountDesired, &accountTicket, &expectedAccountDID)
			if accountErr != nil && !errors.Is(accountErr, pgx.ErrNoRows) {
				return accountErr
			}
			if accountErr == nil {
				if accountPhase != "applied" || desired == "suspended" {
					return errors.New("native account state requires inspection before recovery release")
				}
				inspector, ok := observer.(interface {
					AccountStatus(context.Context, nodeagent.Peer, string) (nodeagent.AccountStatus, error)
				})
				if !ok {
					return errors.New("authenticated native moderation observer required")
				}
				account, err := inspector.AccountStatus(ctx, peer, id)
				if err != nil || account.InstanceID != id || expectedAccountDID == "" || account.DID != expectedAccountDID || account.State != accountDesired || account.Generation != accountGeneration || account.Ticket != accountTicket || account.Phase != "applied" {
					return errors.New("native account state differs from recovery intent")
				}
			}
			delete(inventory, id)
			verifiedHosts[id] = peer
			verified++
		}
		retained, err := tx.Query(ctx, `SELECT instance_id::text FROM recovery_retained_namespaces WHERE node_id=$1`, peer.NodeID)
		if err != nil {
			return err
		}
		var retainedIDs []string
		for retained.Next() {
			var id string
			if err = retained.Scan(&id); err != nil {
				retained.Close()
				return err
			}
			retainedIDs = append(retainedIDs, id)
		}
		retained.Close()
		if err = retained.Err(); err != nil {
			return err
		}
		for _, id := range retainedIDs {
			var managed bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE id=$1)`, id).Scan(&managed); err != nil {
				return err
			}
			if managed || !inventory[id] {
				return errors.New("retained namespace acknowledgement no longer matches inventory")
			}
			delete(inventory, id)
		}
		if len(inventory) != 0 {
			return errors.New("untracked host namespaces require operator reconciliation")
		}
	}
	if total != verified {
		return errors.New("placement has no verifiable registered host")
	}
	if err := reconcileMemberExits(ctx, tx, observer, verifiedHosts); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now() WHERE singleton=true`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
