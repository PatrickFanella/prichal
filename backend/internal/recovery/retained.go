package recovery

import (
	"context"
	"errors"
	"regexp"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AcknowledgeRetained records an operator-reviewed local fixture without adopting,
// deleting or enabling it. A managed deployment can never receive this exemption.
func AcknowledgeRetained(ctx context.Context, pool *pgxpool.Pool, observer Observer, node, id string) error {
	uuid := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	if observer == nil || !uuid.MatchString(node) || !uuid.MatchString(id) {
		return errors.New("registered node and retained fixture UUID required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var database, state string
	if err = tx.QueryRow(ctx, `SELECT current_database(),state FROM management_recovery WHERE singleton=true FOR UPDATE`).Scan(&database, &state); err != nil {
		return errors.New("recovery hold required")
	}
	if !regexp.MustCompile(`^pds_recovery_[a-f0-9]{32}$`).MatchString(database) || state != "held" {
		return errors.New("held disposable local restore required")
	}
	var managed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE id=$1)`, id).Scan(&managed); err != nil {
		return err
	}
	if managed {
		return errors.New("managed deployment cannot be exempted from reconciliation")
	}
	var peer nodeagent.Peer
	if err = tx.QueryRow(ctx, `SELECT id::text,agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1 AND enrolled_at IS NOT NULL FOR UPDATE`, node).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return errors.New("registered node required")
	}
	inventory, err := observer.Capacity(ctx, peer)
	if err != nil {
		return errors.New("retained host inventory could not be verified")
	}
	found := false
	for _, entry := range inventory.InstanceIDs {
		if entry == id {
			found = true
		}
	}
	if !found {
		return errors.New("retained fixture missing from authenticated inventory")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO recovery_retained_namespaces(node_id,instance_id,reason) VALUES($1,$2,'local-qualification-fixture') ON CONFLICT DO NOTHING`, node, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
