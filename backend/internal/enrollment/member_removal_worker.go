package enrollment

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type MemberRemovalAgent interface {
	MemberExit(context.Context, nodeagent.Peer, string, string) (nodeagent.MemberExit, error)
}
type MemberRemovalWorker struct {
	river.WorkerDefaults[MemberRemovalArgs]
	Pool  *pgxpool.Pool
	Agent MemberRemovalAgent
}

func (w *MemberRemovalWorker) Work(ctx context.Context, job *river.Job[MemberRemovalArgs]) error {
	if err := recovery.Check(ctx, w.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.perform(ctx, job.Args.RemovalID)
	var snooze *river.JobSnoozeError
	if err == nil || errors.As(err, &snooze) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.Attempt >= job.MaxAttempts {
		tx, e := w.Pool.Begin(ctx)
		if e != nil {
			return errors.New("member removal requires review")
		}
		defer tx.Rollback(ctx)
		if e = checkEnrollmentRecovery(ctx, tx); e != nil {
			return e
		}
		const message = "Member removal could not be confirmed. Retained data and the original migration window are preserved; retry for reconciliation."
		row, e := tx.Exec(ctx, `UPDATE member_removals SET state='review',message=$2,updated_at=now() WHERE id=$1 AND state IN ('requested','migration_window')`, job.Args.RemovalID, message)
		if e != nil {
			return errors.New("member removal requires review")
		}
		if row.RowsAffected() > 0 {
			if _, e = tx.Exec(ctx, `INSERT INTO member_removal_events(removal_id,state,message) VALUES($1,'review',$2)`, job.Args.RemovalID, message); e != nil {
				return errors.New("member removal requires review")
			}
		}
		if tx.Commit(ctx) != nil {
			return errors.New("member removal requires review")
		}
	}
	return errors.New("member removal could not be confirmed")
}

func (w *MemberRemovalWorker) perform(ctx context.Context, id string) error {
	if w.Agent == nil {
		return errors.New("member removal agent unavailable")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM member_removals WHERE id=$1`, id).Scan(&deployment); err != nil {
		return err
	}
	var currentOwner, runtimeState string
	if err = tx.QueryRow(ctx, `SELECT o.owner_id::text,d.state FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND d.kind='agent' FOR UPDATE OF d FOR SHARE OF o`, deployment).Scan(&currentOwner, &runtimeState); err != nil {
		return err
	}
	var invitation, owner, did, state string
	var created time.Time
	var hostStart, deadline *time.Time
	if err = tx.QueryRow(ctx, `SELECT invitation_id::text,owner_id::text,account_did,state,created_at,host_requested_at,deactivate_after FROM member_removals WHERE id=$1 FOR UPDATE`, id).Scan(&invitation, &owner, &did, &state, &created, &hostStart, &deadline); err != nil {
		return err
	}
	if state == "deactivated" || state == "review" {
		return tx.Commit(ctx)
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return err
	}
	var memberMatches, exiting bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM member_invitations WHERE id=$1 AND deployment_id=$2 AND owner_id=$3 AND state='accepted' AND account_did=$4),EXISTS(SELECT 1 FROM hosting_exits WHERE deployment_id=$2 AND state<>'reversed')`, invitation, deployment, owner, did).Scan(&memberMatches, &exiting); err != nil {
		return err
	}
	if currentOwner != owner || runtimeState != "ready" || !memberMatches || exiting {
		return errors.New("retained member and ready hosting must match")
	}
	if deadline != nil && time.Now().Before(*deadline) {
		return river.JobSnooze(time.Until(*deadline))
	}
	var peer nodeagent.Peer
	if err = tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining')`, deployment).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return err
	}
	result, err := w.Agent.MemberExit(ctx, peer, deployment, invitation)
	if err != nil || !result.Valid(deployment, invitation) || result.DID != did {
		return errors.New("native member exit identity or state differs")
	}
	start, end := time.UnixMilli(result.RequestedAt), time.UnixMilli(result.DeactivateAfter)
	if start.Before(created.Add(-time.Minute)) || (hostStart != nil && (!hostStart.Equal(start) || !deadline.Equal(end))) {
		return errors.New("native member exit window differs")
	}
	message := "The migration window is recorded. This removal will not deactivate the account before its deadline. Existing account restrictions remain in effect."
	var completed *time.Time
	if result.State == "deactivated" {
		now := time.Now()
		completed = &now
		message = "The member account is deactivated. Its data remains retained; this removal did not request account deletion."
	}
	if _, err = tx.Exec(ctx, `UPDATE member_removals SET state=$2,message=$3,host_requested_at=$4,deactivate_after=$5,completed_at=$6,updated_at=now() WHERE id=$1`, id, result.State, message, start, end, completed); err != nil {
		return err
	}
	if state != result.State {
		if _, err = tx.Exec(ctx, `INSERT INTO member_removal_events(removal_id,state,message) VALUES($1,$2,$3)`, id, result.State, message); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if result.State == "migration_window" {
		delay := time.Until(end)
		if delay < time.Minute {
			delay = time.Minute
		}
		return river.JobSnooze(delay)
	}
	return nil
}
