package enrollment

import (
	"context"
	"errors"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

var regexpDID = regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`)

type Agent interface {
	Enroll(context.Context, nodeagent.Peer, string, string) (nodeagent.Enrollment, error)
	WithdrawEnrollment(context.Context, nodeagent.Peer, string, string) (nodeagent.Enrollment, error)
}
type Worker struct {
	river.WorkerDefaults[Args]
	Pool   *pgxpool.Pool
	Cipher *Cipher
	Agent  Agent
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	if err := recovery.Check(ctx, w.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	withdrawalAttempt := -1
	err := w.perform(ctx, job.Args.OperationID, &withdrawalAttempt)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.Attempt >= job.MaxAttempts {
		if err := w.recordFailure(ctx, job.Args.OperationID, withdrawalAttempt); err != nil {
			return err
		}
	}
	return errors.New("enrollment is temporarily unavailable")
}
func (w *Worker) recordFailure(ctx context.Context, id string, withdrawalAttempt int) error {
	tx, txErr := w.Pool.Begin(ctx)
	if txErr != nil {
		return errors.New("enrollment result could not be confirmed")
	}
	defer tx.Rollback(ctx)
	if err := checkEnrollmentRecovery(ctx, tx); err != nil {
		return err
	}
	// A later owner intent must not be failed by an older attempt whose
	// native transaction has already released its locks.
	result, txErr := tx.Exec(ctx, `UPDATE enrollment_operations SET state='failed',code_ciphertext=NULL,message=CASE WHEN withdrawal_requested_at IS NOT NULL THEN 'Invitation withdrawal could not be confirmed. Retry withdrawal or contact support. An existing account is preserved.' ELSE 'Enrollment could not be confirmed. Request bounded recovery or contact support.' END,updated_at=now() WHERE id=$1 AND withdrawal_attempts=$2 AND state NOT IN ('superseded','verified','account_created','withdrawn','cancelled')`, id, withdrawalAttempt)
	if txErr == nil && result.RowsAffected() > 0 {
		_, txErr = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) VALUES($1,'failed','Enrollment could not be confirmed. Native resources are preserved.')`, id)
	}
	if txErr != nil {
		return errors.New("enrollment result could not be confirmed")
	}
	if txErr = tx.Commit(ctx); txErr != nil {
		return errors.New("enrollment result could not be confirmed")
	}
	return nil
}

func (w *Worker) NextRetry(job *river.Job[Args]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}
func (w *Worker) perform(ctx context.Context, id string, withdrawalAttempt *int) error {
	cancelled, err := w.reserveDispatch(ctx, id)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	return w.performReserved(ctx, id, withdrawalAttempt)
}
func (w *Worker) performReserved(ctx context.Context, id string, withdrawalAttempt *int) error {
	if w.Agent == nil || w.Cipher == nil {
		return errors.New("enrollment worker is not configured")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM enrollment_operations WHERE id=$1`, id).Scan(&deployment); err != nil {
		return err
	}
	var organization, owner, runtimeState string
	if err = tx.QueryRow(ctx, `SELECT d.organization_id::text,o.owner_id::text,d.state FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND d.kind='agent' FOR UPDATE OF d FOR SHARE OF o`, deployment).Scan(&organization, &owner, &runtimeState); err != nil {
		return err
	}
	var opOwner, previous string
	var withdrawal *time.Time
	if err = tx.QueryRow(ctx, `SELECT owner_id::text,state,withdrawal_requested_at,withdrawal_attempts FROM enrollment_operations WHERE id=$1 FOR UPDATE`, id).Scan(&opOwner, &previous, &withdrawal, withdrawalAttempt); err != nil {
		return err
	}
	if previous == "cancelled" || previous == "superseded" || previous == "withdrawn" || (withdrawal != nil && (previous == "failed" || previous == "verified")) {
		return tx.Commit(ctx)
	}
	if opOwner != owner || runtimeState != "ready" {
		return errors.New("owner and ready deployment must match enrollment")
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return err
	}
	var peer nodeagent.Peer
	if err = tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining')`, deployment).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return err
	}
	var result nodeagent.Enrollment
	if withdrawal != nil {
		result, err = w.Agent.WithdrawEnrollment(ctx, peer, deployment, id)
	} else {
		result, err = w.Agent.Enroll(ctx, peer, deployment, id)
	}
	if err != nil {
		return errors.New("native enrollment could not be confirmed")
	}
	if result.InstanceID != deployment || result.Ticket != id {
		return errors.New("native enrollment identity differs")
	}
	var code []byte
	var expires *time.Time
	var did, handle *string
	var message string
	switch result.State {
	case "invitation_ready":
		if withdrawal != nil || result.Code == "" || result.ExpiresAt <= time.Now().UnixMilli() || result.ExpiresAt > time.Now().Add(15*time.Minute+5*time.Second).UnixMilli() {
			return errors.New("native invitation lifetime invalid")
		}
		code, err = w.Cipher.seal(result.Code, id+"/"+deployment+"/"+organization)
		if err != nil {
			return err
		}
		deadline := time.UnixMilli(result.ExpiresAt)
		expires = &deadline
		message = "Your one-use invitation is ready. Create the account directly on your PDS."
	case "account_created", "verified":
		if result.Code != "" || !regexpDID.MatchString(result.DID) || result.Handle == "" || len(result.Handle) > 253 || (result.State == "verified" && !result.EmailConfirmed) {
			return errors.New("native account status invalid")
		}
		did = &result.DID
		handle = &result.Handle
		message = "Your account exists. Confirm its email directly through your PDS."
		if result.State == "verified" {
			message = "Your first account and email are verified. Sign in through a compatible AT client."
		}
	case "expired":
		if withdrawal != nil || result.Code != "" {
			return errors.New("expired invitation contained a code")
		}
		deadline := time.UnixMilli(result.ExpiresAt)
		expires = &deadline
		message = "This invitation expired. Request recovery to replace it."
	case "withdrawn":
		if withdrawal == nil || result.Code != "" || result.DID != "" || result.Handle != "" || result.ExpiresAt != 0 || result.EmailConfirmed {
			return errors.New("invalid withdrawn enrollment")
		}
		message = "The invitation is withdrawn. No first account was found. Request recovery with a new ticket if you want to enroll."
	default:
		return errors.New("native enrollment state unsupported")
	}
	if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET state=$2,code_ciphertext=$3,expires_at=$4,account_did=$5,account_handle=$6,email_confirmed=$7,message=$8,updated_at=now() WHERE id=$1`, id, result.State, code, expires, did, handle, result.EmailConfirmed, message); err != nil {
		return err
	}
	if previous != result.State {
		if _, err = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) VALUES($1,$2,$3)`, id, result.State, message); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
