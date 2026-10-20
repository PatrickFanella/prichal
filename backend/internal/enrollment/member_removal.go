package enrollment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// ErrRemovalNotRequested is returned only after current accepted-member access is confirmed.
var ErrRemovalNotRequested = errors.New("member removal not requested")

type MemberRemovalArgs struct {
	RemovalID string `json:"removalId"`
}

func (MemberRemovalArgs) Kind() string { return "member_removal" }

type MemberRemoval struct {
	ID              string        `json:"id"`
	RequestKey      string        `json:"idempotencyKey"`
	InvitationID    string        `json:"invitationId"`
	DeploymentID    string        `json:"deploymentId"`
	DID             string        `json:"did"`
	State           string        `json:"state"`
	Message         string        `json:"message"`
	RequestedBy     string        `json:"requestedBy"`
	CreatedAt       time.Time     `json:"createdAt"`
	HostRequestedAt *time.Time    `json:"hostRequestedAt"`
	DeactivateAfter *time.Time    `json:"deactivateAfter"`
	CompletedAt     *time.Time    `json:"completedAt"`
	Events          []MemberEvent `json:"events"`
}

const removalRequestedMessage = "Member removal is recorded. The migration window begins when the server confirms it."

// RemoveMember retains the accepted identity and atomically records intent, actor and work.
// Repeating the same key can retry a review outcome without changing the host ticket.
func (s *Store) RemoveMember(ctx context.Context, actor, invitation string, request MemberRequest) (MemberRemoval, error) {
	if s.Cipher == nil || s.Queue == nil || !uuid.MatchString(actor) || !uuid.MatchString(invitation) || !uuid.MatchString(request.Key) {
		return MemberRemoval{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MemberRemoval{}, err
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return MemberRemoval{}, err
	}
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM member_invitations WHERE id=$1`, invitation).Scan(&deployment); errors.Is(err, pgx.ErrNoRows) {
		return MemberRemoval{}, ErrNotFound
	} else if err != nil {
		return MemberRemoval{}, err
	}
	owner, err := s.memberAccess(ctx, tx, actor, deployment, true)
	if err != nil {
		return MemberRemoval{}, err
	}
	var state, did, invitationOwner string
	if err = tx.QueryRow(ctx, `SELECT state,coalesce(account_did,''),owner_id::text FROM member_invitations WHERE id=$1 FOR UPDATE`, invitation).Scan(&state, &did, &invitationOwner); err != nil {
		return MemberRemoval{}, err
	}
	if state != "accepted" || owner != invitationOwner {
		return MemberRemoval{}, ErrConflict
	}
	message := removalRequestedMessage
	var id, key, previous string
	err = tx.QueryRow(ctx, `SELECT id::text,idempotency_key::text,state FROM member_removals WHERE invitation_id=$1 FOR UPDATE`, invitation).Scan(&id, &key, &previous)
	if err == nil {
		if key != request.Key {
			return MemberRemoval{}, ErrConflict
		}
		if previous != "review" {
			if err = tx.Commit(ctx); err != nil {
				return MemberRemoval{}, err
			}
			return s.GetMemberRemoval(ctx, actor, invitation)
		}
		message = "Member removal is awaiting reconciliation. The original server ticket and any confirmed migration window are retained."
		if _, err = tx.Exec(ctx, `UPDATE member_removals SET state=CASE WHEN deactivate_after IS NULL THEN 'requested' ELSE 'migration_window' END,message=$2,updated_at=now() WHERE id=$1`, id, message); err != nil {
			return MemberRemoval{}, err
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		var keyUsed, exiting bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM member_removals WHERE deployment_id=$1 AND idempotency_key=$2),EXISTS(SELECT 1 FROM hosting_exits WHERE deployment_id=$1 AND state<>'reversed')`, deployment, request.Key).Scan(&keyUsed, &exiting); err != nil {
			return MemberRemoval{}, err
		}
		if keyUsed {
			return MemberRemoval{}, ErrConflict
		}
		if exiting {
			return MemberRemoval{}, ErrHostingExit
		}
		if err = tx.QueryRow(ctx, `INSERT INTO member_removals(invitation_id,deployment_id,owner_id,requested_by,idempotency_key,account_did,message) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, invitation, deployment, owner, actor, request.Key, did, removalRequestedMessage).Scan(&id); err != nil {
			return MemberRemoval{}, err
		}
	} else {
		return MemberRemoval{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO member_removal_events(removal_id,state,message,actor_id) SELECT id,state,$2,$3 FROM member_removals WHERE id=$1`, id, message, actor); err != nil {
		return MemberRemoval{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, MemberRemovalArgs{id}, &river.InsertOpts{MaxAttempts: 5}); err != nil {
		return MemberRemoval{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MemberRemoval{}, err
	}
	return s.GetMemberRemoval(ctx, actor, invitation)
}

func (s *Store) GetMemberRemoval(ctx context.Context, actor, invitation string) (MemberRemoval, error) {
	var result MemberRemoval
	if !uuid.MatchString(actor) || !uuid.MatchString(invitation) {
		return result, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return result, err
	}
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM member_invitations WHERE id=$1`, invitation).Scan(&deployment); errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	} else if err != nil {
		return result, err
	}
	owner, err := s.memberAccess(ctx, tx, actor, deployment, false)
	if err != nil {
		return result, err
	}
	var acceptedOwner, memberState, did string
	if err = tx.QueryRow(ctx, `SELECT owner_id::text,state,coalesce(account_did,'') FROM member_invitations WHERE id=$1 FOR SHARE`, invitation).Scan(&acceptedOwner, &memberState, &did); err != nil {
		return result, err
	}
	if acceptedOwner != owner || memberState != "accepted" {
		return result, ErrNotFound
	}
	result.InvitationID, result.DeploymentID, result.DID = invitation, deployment, did
	err = tx.QueryRow(ctx, `SELECT r.id::text,r.idempotency_key::text,r.invitation_id::text,r.deployment_id::text,r.account_did,r.state,r.message,r.requested_by::text,r.created_at,r.host_requested_at,r.deactivate_after,r.completed_at FROM member_removals r JOIN member_invitations m ON m.id=r.invitation_id WHERE r.invitation_id=$1 AND r.owner_id=$2 AND m.owner_id=$2 AND m.state='accepted' AND m.account_did=r.account_did FOR SHARE OF r,m`, invitation, owner).Scan(&result.ID, &result.RequestKey, &result.InvitationID, &result.DeploymentID, &result.DID, &result.State, &result.Message, &result.RequestedBy, &result.CreatedAt, &result.HostRequestedAt, &result.DeactivateAfter, &result.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM member_removals WHERE invitation_id=$1)`, invitation).Scan(&exists); e != nil {
			return result, e
		}
		if exists {
			return result, ErrNotFound
		}
		return result, ErrRemovalNotRequested
	}
	if err != nil {
		return result, err
	}
	result.Events = []MemberEvent{}
	rows, err := tx.Query(ctx, `SELECT state,message,created_at,actor_id::text FROM (SELECT id,state,message,created_at,actor_id FROM member_removal_events WHERE removal_id=$1 ORDER BY id DESC LIMIT 100) history ORDER BY id`, result.ID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var e MemberEvent
		if err = rows.Scan(&e.State, &e.Message, &e.CreatedAt, &e.ActorID); err != nil {
			rows.Close()
			return result, err
		}
		result.Events = append(result.Events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
