package enrollment

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type MemberAgent interface {
	MemberInvitation(context.Context, nodeagent.Peer, string, string) (nodeagent.MemberInvitation, error)
	WithdrawMemberInvitation(context.Context, nodeagent.Peer, string, string) (nodeagent.MemberInvitation, error)
}
type MemberWorker struct {
	river.WorkerDefaults[MemberArgs]
	Pool   *pgxpool.Pool
	Cipher *Cipher
	Agent  MemberAgent
	// Queue is optional; the running worker client is used when unset.
	Queue *river.Client[pgx.Tx]
}

var memberMessages = map[string]string{
	"invitation_ready": "The member invitation is ready. Share the one-use code with the person joining; it expires in 72 hours.",
	"accepted":         "The member created their account. Their password and email stay on the PDS.",
	"expired":          "This member invitation expired and its code no longer works. Prepare a new one if the person still needs an account.",
	"withdrawn":        "The member invitation is withdrawn and its code no longer works.",
	"ceiling_reached":  "This server has reached its member limit. No code was issued.",
	"owner_required":   "The owner's first account must exist before members can join. No code was issued.",
}

func (w *MemberWorker) Work(ctx context.Context, job *river.Job[MemberArgs]) error {
	if err := recovery.Check(ctx, w.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.perform(ctx, job.Args.InvitationID)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.Attempt >= job.MaxAttempts {
		if err := w.recordFailure(ctx, job.Args.InvitationID); err != nil {
			return err
		}
	}
	return errors.New("member invitation is temporarily unavailable")
}

func (w *MemberWorker) NextRetry(job *river.Job[MemberArgs]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}

// recordFailure fails only work that has not reached a native result. An open
// invitation stays open: its native code and expiry remain authoritative.
func (w *MemberWorker) recordFailure(ctx context.Context, id string) error {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return errors.New("member invitation result could not be confirmed")
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return err
	}
	const message = "The member invitation could not be confirmed. Native accounts and codes are preserved; contact support."
	result, err := tx.Exec(ctx, `UPDATE member_invitations SET state='failed',code_ciphertext=NULL,message=$2,updated_at=now() WHERE id=$1 AND state IN ('pending','withdrawing')`, id, message)
	if err == nil && result.RowsAffected() > 0 {
		_, err = tx.Exec(ctx, `INSERT INTO member_invitation_events(invitation_id,state,message) VALUES($1,'failed',$2)`, id, message)
	}
	if err != nil || tx.Commit(ctx) != nil {
		return errors.New("member invitation result could not be confirmed")
	}
	return nil
}

func (w *MemberWorker) perform(ctx context.Context, id string) error {
	queue := w.Queue
	if queue == nil {
		queue, _ = river.ClientFromContextSafely[pgx.Tx](ctx)
	}
	if w.Agent == nil || w.Cipher == nil || queue == nil {
		return errors.New("member worker is not configured")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment, invitationOwner, previous string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text,owner_id::text,state FROM member_invitations WHERE id=$1`, id).Scan(&deployment, &invitationOwner, &previous); err != nil {
		return err
	}
	var organization, owner, runtimeState string
	if err = tx.QueryRow(ctx, `SELECT d.organization_id::text,o.owner_id::text,d.state FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND d.kind='agent' FOR UPDATE OF d FOR SHARE OF o`, deployment).Scan(&organization, &owner, &runtimeState); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT state FROM member_invitations WHERE id=$1 FOR UPDATE`, id).Scan(&previous); err != nil {
		return err
	}
	if previous != "pending" && previous != "invitation_ready" && previous != "withdrawing" {
		return tx.Commit(ctx)
	}
	if invitationOwner != owner || runtimeState != "ready" {
		return errors.New("owner and ready deployment must match member invitation")
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return err
	}
	var peer nodeagent.Peer
	if err = tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining')`, deployment).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return err
	}
	withdraw := previous == "withdrawing"
	var result nodeagent.MemberInvitation
	if withdraw {
		result, err = w.Agent.WithdrawMemberInvitation(ctx, peer, deployment, id)
	} else {
		result, err = w.Agent.MemberInvitation(ctx, peer, deployment, id)
	}
	if err != nil {
		return errors.New("native member invitation could not be confirmed")
	}
	if !result.Valid(deployment, id, withdraw) {
		return errors.New("native member invitation identity or state differs")
	}
	if previous == "invitation_ready" && (result.State == "ceiling_reached" || result.State == "owner_required" || result.State == "withdrawn") {
		return errors.New("issued member invitation cannot regress")
	}
	var code []byte
	var expires *time.Time
	var did, handle *string
	switch result.State {
	case "invitation_ready":
		if code, err = w.Cipher.seal(result.Code, memberScope(id, deployment, organization)); err != nil {
			return err
		}
		deadline := time.UnixMilli(result.ExpiresAt)
		expires = &deadline
	case "accepted":
		did, handle = &result.DID, &result.Handle
	case "expired":
		deadline := time.UnixMilli(result.ExpiresAt)
		expires = &deadline
	}
	message := memberMessages[result.State]
	if _, err = tx.Exec(ctx, `UPDATE member_invitations SET state=$2,code_ciphertext=$3,expires_at=coalesce($4,expires_at),account_did=$5,account_handle=$6,message=$7,updated_at=now() WHERE id=$1`, id, result.State, code, expires, did, handle, message); err != nil {
		return err
	}
	if previous != result.State {
		if _, err = tx.Exec(ctx, `INSERT INTO member_invitation_events(invitation_id,state,message) VALUES($1,$2,$3)`, id, result.State, message); err != nil {
			return err
		}
		if result.State == "invitation_ready" {
			// The node disables the code when this job finds the invitation expired.
			if _, err = queue.InsertTx(ctx, tx, MemberArgs{id}, &river.InsertOpts{MaxAttempts: 3, ScheduledAt: expires.Add(2 * time.Second)}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}
