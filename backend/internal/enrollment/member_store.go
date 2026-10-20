package enrollment

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

var ErrOwnerAccountRequired = errors.New("owner first account required")
var ErrMemberCeiling = errors.New("member ceiling reached")
var ErrHostingExit = errors.New("hosting exit in progress")

// MemberArgs drives issuance, refresh, expiry and withdrawal for one invitation.
type MemberArgs struct {
	InvitationID string `json:"invitationId"`
}

func (MemberArgs) Kind() string { return "member_invitation" }

type MemberRequest struct {
	Key string `json:"idempotencyKey"`
}
type MemberInvitation struct {
	ID                  string        `json:"id"`
	DeploymentID        string        `json:"deploymentId"`
	State               string        `json:"state"`
	Message             string        `json:"message"`
	Code                string        `json:"code,omitempty"`
	ExpiresAt           *time.Time    `json:"expiresAt"`
	AccountURL          string        `json:"accountUrl"`
	DID                 string        `json:"did,omitempty"`
	Handle              string        `json:"handle,omitempty"`
	WithdrawalRequested bool          `json:"withdrawalRequested"`
	CreatedAt           time.Time     `json:"createdAt"`
	Events              []MemberEvent `json:"events"`
}
type MemberEvent struct {
	Event
	ActorID *string `json:"actorId"`
}
type MemberAccount struct {
	InvitationID string `json:"invitationId"`
	DID          string `json:"did"`
	Handle       string `json:"handle"`
}
type MemberSummary struct {
	Accounts    []MemberAccount    `json:"accounts"`
	Ceiling     int                `json:"ceiling"`
	Members     int                `json:"members"`
	Open        int                `json:"open"`
	Invitations []MemberInvitation `json:"invitations"`
}

const memberHourlyLimit = 10

func memberScope(id, deployment, organization string) string {
	return "member/" + id + "/" + deployment + "/" + organization
}

// memberAccess locks the organization against concurrent role changes. Mutations
// also serialize the deployment's invitation limit and idempotency checks.
func (s *Store) memberAccess(ctx context.Context, tx pgx.Tx, actor, deployment string, mutation bool) (string, error) {
	var organization, owner, state, kind string
	lock := " FOR SHARE OF d,o"
	if mutation {
		lock = " FOR UPDATE OF d FOR SHARE OF o"
	}
	err := tx.QueryRow(ctx, `SELECT d.organization_id::text,o.owner_id::text,d.state,d.kind FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1`+lock, deployment).Scan(&organization, &owner, &state, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if actor != owner {
		if !s.MemberAdministrators {
			return "", ErrNotFound
		}
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND owner_id=$2 AND state='active' AND role='administrator')`, organization, actor).Scan(&allowed); err != nil {
			return "", err
		}
		if !allowed {
			return "", ErrNotFound
		}
	}
	if mutation && (state != "ready" || kind != "agent") {
		return "", ErrConflict
	}
	return owner, nil
}

// MemberAdmission returns request policy only to a current invitation manager.
func (s *Store) MemberAdmission(ctx context.Context, actor, deployment string) (Admission, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(deployment) {
		return Admission{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Admission{}, err
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return Admission{}, err
	}
	if _, err = s.memberAccess(ctx, tx, actor, deployment, false); err != nil {
		return Admission{}, err
	}
	var result Admission
	if err = tx.QueryRow(ctx, `SELECT open,updated_at FROM enrollment_admission WHERE singleton=true`).Scan(&result.Open, &result.UpdatedAt); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// SubmitMember records one member invitation request and its job atomically.
func (s *Store) SubmitMember(ctx context.Context, owner, deployment string, request MemberRequest) (MemberInvitation, error) {
	actor := owner
	if s.Cipher == nil || !uuid.MatchString(owner) || !uuid.MatchString(deployment) || !uuid.MatchString(request.Key) {
		return MemberInvitation{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MemberInvitation{}, err
	}
	defer tx.Rollback(ctx)
	if owner, err = s.memberAccess(ctx, tx, actor, deployment, true); err != nil {
		return MemberInvitation{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM member_invitations WHERE deployment_id=$1 AND idempotency_key=$2 AND owner_id=$3`, deployment, request.Key, owner).Scan(&id)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return MemberInvitation{}, err
		}
		return s.GetMember(ctx, actor, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MemberInvitation{}, err
	}
	if err = checkAdmission(ctx, tx); err != nil {
		return MemberInvitation{}, err
	}
	var ownerAccount, exiting bool
	var active, recent int
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrollment_operations WHERE deployment_id=$1 AND state IN ('account_created','verified') AND account_did IS NOT NULL),
 EXISTS(SELECT 1 FROM hosting_exits WHERE deployment_id=$1 AND state<>'reversed'),
 (SELECT count(*) FROM member_invitations WHERE deployment_id=$1 AND state IN ('pending','invitation_ready','withdrawing','accepted')),
 (SELECT count(*) FROM member_invitations WHERE deployment_id=$1 AND created_at>now()-interval '1 hour')`, deployment).Scan(&ownerAccount, &exiting, &active, &recent); err != nil {
		return MemberInvitation{}, err
	}
	switch {
	case !ownerAccount:
		return MemberInvitation{}, ErrOwnerAccountRequired
	case exiting:
		return MemberInvitation{}, ErrHostingExit
	case active >= nodeagent.MemberCeiling:
		return MemberInvitation{}, ErrMemberCeiling
	case recent >= memberHourlyLimit:
		return MemberInvitation{}, ErrConflict
	}
	const message = "Preparing a member invitation."
	if err = tx.QueryRow(ctx, `INSERT INTO member_invitations(deployment_id,owner_id,idempotency_key,message) VALUES($1,$2,$3,$4) RETURNING id::text`, deployment, owner, request.Key, message).Scan(&id); err != nil {
		return MemberInvitation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO member_invitation_events(invitation_id,state,message,actor_id) VALUES($1,'pending',$2,$3)`, id, message, actor); err != nil {
		return MemberInvitation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, MemberArgs{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return MemberInvitation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MemberInvitation{}, err
	}
	return s.GetMember(ctx, actor, id)
}

// GetMember returns one invitation after checking current deployment ownership.
func (s *Store) GetMember(ctx context.Context, owner, id string) (MemberInvitation, error) {
	if !uuid.MatchString(owner) || !uuid.MatchString(id) {
		return MemberInvitation{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MemberInvitation{}, err
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return MemberInvitation{}, err
	}
	invitation, err := s.readMember(ctx, tx, owner, id, true)
	if err != nil {
		return MemberInvitation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MemberInvitation{}, err
	}
	return invitation, nil
}

func (s *Store) readMember(ctx context.Context, tx pgx.Tx, owner, id string, events bool) (MemberInvitation, error) {
	var invitation MemberInvitation
	var deployment string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM member_invitations WHERE id=$1`, id).Scan(&deployment); errors.Is(err, pgx.ErrNoRows) {
		return invitation, ErrNotFound
	} else if err != nil {
		return invitation, err
	}
	canonicalOwner, err := s.memberAccess(ctx, tx, owner, deployment, false)
	if err != nil {
		return invitation, err
	}
	owner = canonicalOwner
	var encoded []byte
	var organization, endpoint string
	err = tx.QueryRow(ctx, `SELECT m.id::text,m.deployment_id::text,m.state,m.message,m.code_ciphertext,m.expires_at,coalesce(m.account_did,''),coalesce(m.account_handle,''),m.withdrawal_requested_at IS NOT NULL,m.created_at,d.organization_id::text,d.endpoint_url FROM member_invitations m JOIN deployments d ON d.id=m.deployment_id JOIN organizations o ON o.id=d.organization_id WHERE m.id=$1 AND m.owner_id=$2 AND o.owner_id=$2 FOR SHARE OF m,d,o`, id, owner).Scan(&invitation.ID, &invitation.DeploymentID, &invitation.State, &invitation.Message, &encoded, &invitation.ExpiresAt, &invitation.DID, &invitation.Handle, &invitation.WithdrawalRequested, &invitation.CreatedAt, &organization, &endpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return invitation, ErrNotFound
	}
	if err != nil {
		return invitation, err
	}
	if endpoint != nodeagent.LocalEndpoint(invitation.DeploymentID) {
		return MemberInvitation{}, errors.New("member endpoint differs from managed identity")
	}
	invitation.AccountURL = endpoint + "/account"
	if invitation.State == "invitation_ready" {
		if invitation.ExpiresAt == nil || !invitation.ExpiresAt.After(time.Now()) {
			invitation.State = "expired"
			invitation.Message = "This member invitation expired. Prepare a new one if the person still needs an account."
		} else if invitation.Code, err = s.Cipher.open(encoded, memberScope(invitation.ID, invitation.DeploymentID, organization)); err != nil {
			return MemberInvitation{}, err
		}
	}
	invitation.Events = []MemberEvent{}
	if !events {
		return invitation, nil
	}
	rows, err := tx.Query(ctx, `SELECT state,message,created_at,actor_id::text FROM member_invitation_events WHERE invitation_id=$1 ORDER BY id LIMIT 100`, id)
	if err != nil {
		return invitation, err
	}
	defer rows.Close()
	for rows.Next() {
		var event MemberEvent
		if err = rows.Scan(&event.State, &event.Message, &event.CreatedAt, &event.ActorID); err != nil {
			return invitation, err
		}
		invitation.Events = append(invitation.Events, event)
	}
	return invitation, rows.Err()
}

// ListMembers returns the complete bounded accepted roster and newest invitations.
func (s *Store) ListMembers(ctx context.Context, owner, deployment string) (MemberSummary, error) {
	actor := owner
	summary := MemberSummary{Ceiling: nodeagent.MemberCeiling, Invitations: []MemberInvitation{}, Accounts: []MemberAccount{}}
	if !uuid.MatchString(owner) || !uuid.MatchString(deployment) {
		return summary, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return summary, err
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return summary, err
	}
	if owner, err = s.memberAccess(ctx, tx, actor, deployment, false); err != nil {
		return summary, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='accepted'),count(*) FILTER (WHERE state='invitation_ready' AND expires_at>now()) FROM member_invitations WHERE deployment_id=$1`, deployment).Scan(&summary.Members, &summary.Open); err != nil {
		return summary, err
	}
	roster, err := tx.Query(ctx, `SELECT id::text,account_did,account_handle FROM member_invitations WHERE deployment_id=$1 AND owner_id=$2 AND state='accepted' ORDER BY account_handle,id LIMIT 26`, deployment, owner)
	if err != nil {
		return summary, err
	}
	for roster.Next() {
		var member MemberAccount
		if err = roster.Scan(&member.InvitationID, &member.DID, &member.Handle); err != nil {
			roster.Close()
			return summary, err
		}
		summary.Accounts = append(summary.Accounts, member)
	}
	roster.Close()
	if err = roster.Err(); err != nil {
		return summary, err
	}
	if len(summary.Accounts) > nodeagent.MemberCeiling || len(summary.Accounts) != summary.Members {
		return summary, errors.New("member roster differs from recorded count")
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM member_invitations WHERE deployment_id=$1 AND owner_id=$2 ORDER BY created_at DESC,id DESC LIMIT 100`, deployment, owner)
	if err != nil {
		return summary, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return summary, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return summary, err
	}
	for _, id := range ids {
		invitation, err := s.readMember(ctx, tx, actor, id, false)
		if err != nil {
			return summary, err
		}
		summary.Invitations = append(summary.Invitations, invitation)
	}
	if err = tx.Commit(ctx); err != nil {
		return summary, err
	}
	return summary, nil
}

// RefreshMember queues a native check of an open invitation, at most every five seconds.
func (s *Store) RefreshMember(ctx context.Context, owner, id string) (MemberInvitation, error) {
	return s.changeMember(ctx, owner, id, false)
}

// WithdrawMember asks the node to disable an unused code. A code used first is
// recorded as an accepted member; no account is changed.
func (s *Store) WithdrawMember(ctx context.Context, owner, id string) (MemberInvitation, error) {
	return s.changeMember(ctx, owner, id, true)
}

func (s *Store) changeMember(ctx context.Context, owner, id string, withdraw bool) (MemberInvitation, error) {
	actor := owner
	if !uuid.MatchString(owner) || !uuid.MatchString(id) {
		return MemberInvitation{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return MemberInvitation{}, err
	}
	defer tx.Rollback(ctx)
	var deployment string
	err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM member_invitations WHERE id=$1`, id).Scan(&deployment)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberInvitation{}, ErrNotFound
	}
	if err != nil {
		return MemberInvitation{}, err
	}
	if owner, err = s.memberAccess(ctx, tx, actor, deployment, true); err != nil {
		return MemberInvitation{}, err
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return MemberInvitation{}, err
	}
	var state string
	var expires, refreshed *time.Time
	if err = tx.QueryRow(ctx, `SELECT state,expires_at,last_refresh_at FROM member_invitations WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&state, &expires, &refreshed); errors.Is(err, pgx.ErrNoRows) {
		return MemberInvitation{}, ErrNotFound
	} else if err != nil {
		return MemberInvitation{}, err
	}
	switch {
	case state == "pending" || (withdraw && state == "withdrawing"):
		// Already queued: replay the current status.
	case state != "invitation_ready" || expires == nil || !expires.After(time.Now()):
		return MemberInvitation{}, ErrConflict
	case withdraw:
		const message = "Withdrawing the member invitation. An account created first is preserved."
		if _, err = tx.Exec(ctx, `UPDATE member_invitations SET state='withdrawing',code_ciphertext=NULL,withdrawal_requested_at=now(),message=$2,updated_at=now() WHERE id=$1`, id, message); err != nil {
			return MemberInvitation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO member_invitation_events(invitation_id,state,message,actor_id) VALUES($1,'withdrawing',$2,$3)`, id, message, actor); err != nil {
			return MemberInvitation{}, err
		}
		if _, err = s.Queue.InsertTx(ctx, tx, MemberArgs{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
			return MemberInvitation{}, err
		}
	default:
		if refreshed != nil && time.Since(*refreshed) < 5*time.Second {
			return MemberInvitation{}, ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO member_invitation_events(invitation_id,state,message,actor_id) VALUES($1,'invitation_ready','Native member invitation check requested.',$2)`, id, actor); err != nil {
			return MemberInvitation{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE member_invitations SET last_refresh_at=now() WHERE id=$1`, id); err != nil {
			return MemberInvitation{}, err
		}
		if _, err = s.Queue.InsertTx(ctx, tx, MemberArgs{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
			return MemberInvitation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return MemberInvitation{}, err
	}
	return s.GetMember(ctx, actor, id)
}
