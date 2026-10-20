package recovery

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
)

// A restored removal must already have a confirmed outcome and retain its exact
// accepted identity and host deadline. Inspection cannot start or repair removal.
func reconcileMemberExits(ctx context.Context, tx pgx.Tx, observer Observer, hosts map[string]nodeagent.Peer) error {
	rows, err := tx.Query(ctx, `SELECT r.deployment_id::text,r.invitation_id::text,r.account_did,r.state,r.host_requested_at,r.deactivate_after,
 m.state='accepted' AND m.deployment_id=r.deployment_id AND m.owner_id=r.owner_id AND m.account_did=r.account_did AND o.owner_id=r.owner_id
 FROM member_removals r JOIN member_invitations m ON m.id=r.invitation_id JOIN deployments d ON d.id=r.deployment_id JOIN organizations o ON o.id=d.organization_id
 ORDER BY r.deployment_id,r.id FOR SHARE OF r,m,o`)
	if err != nil {
		return err
	}
	type intent struct {
		deployment, ticket, did, state string
		start, deadline                *time.Time
	}
	var intents []intent
	for rows.Next() {
		var item intent
		var matches bool
		if err = rows.Scan(&item.deployment, &item.ticket, &item.did, &item.state, &item.start, &item.deadline, &matches); err != nil {
			rows.Close()
			return err
		}
		_, placed := hosts[item.deployment]
		if !matches || !placed || item.start == nil || item.deadline == nil || (item.state != "migration_window" && item.state != "deactivated") {
			rows.Close()
			return errors.New("uncertain member removal requires operator reconciliation")
		}
		intents = append(intents, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(intents) == 0 {
		return nil
	}
	inspector, ok := observer.(interface {
		MemberExitStatus(context.Context, nodeagent.Peer, string, string) (nodeagent.MemberExit, error)
	})
	if !ok {
		return errors.New("authenticated member exit observer required")
	}
	for _, item := range intents {
		got, err := inspector.MemberExitStatus(ctx, hosts[item.deployment], item.deployment, item.ticket)
		if err != nil || !got.Valid(item.deployment, item.ticket) || got.DID != item.did || got.State != item.state || !time.UnixMilli(got.RequestedAt).Equal(*item.start) || !time.UnixMilli(got.DeactivateAfter).Equal(*item.deadline) {
			return errors.New("member exit differs from confirmed recovery intent")
		}
	}
	return nil
}
