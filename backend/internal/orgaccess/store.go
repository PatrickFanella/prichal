package orgaccess

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var ErrNotFound = errors.New("organization access not found")
var ErrConflict = errors.New("organization access requires review")

type Store struct {
	Pool    *pgxpool.Pool
	Enabled bool
	Issuer  string
}
type Invitation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	TargetID       string    `json:"targetId"`
	Role           string    `json:"role"`
	State          string    `json:"state"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func roleValid(role string) bool { return role == "administrator" || role == "viewer" }
func audit(ctx context.Context, tx pgx.Tx, org, actor, action string, target *string, role *string) error {
	_, err := tx.Exec(ctx, `INSERT INTO organization_access_events(organization_id,actor_id,action,target_id,role) VALUES($1,$2,$3,$4,$5)`, org, actor, action, target, role)
	return err
}
func (s *Store) begin(ctx context.Context, actor, org string, admin bool) (pgx.Tx, error) {
	if !s.Enabled || !uuid.MatchString(org) {
		return nil, ErrNotFound
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var owner string
	err = tx.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&owner)
	if err == nil && owner != actor {
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT $3 AND EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND owner_id=$2 AND state='active' AND role='administrator')`, org, actor, admin).Scan(&allowed)
		if err == nil && !allowed {
			err = ErrNotFound
		}
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return nil, err
	}
	return tx, nil
}
func invitation(ctx context.Context, tx pgx.Tx, id string) (Invitation, error) {
	var v Invitation
	err := tx.QueryRow(ctx, `SELECT id,organization_id,target_id,role,CASE WHEN state='pending' AND expires_at<=clock_timestamp() THEN 'expired' ELSE state END,expires_at FROM organization_invitations WHERE id=$1`, id).Scan(&v.ID, &v.OrganizationID, &v.TargetID, &v.Role, &v.State, &v.ExpiresAt)
	return v, err
}
func (s *Store) Invite(ctx context.Context, actor, org, target, role, key string) (Invitation, error) {
	if !uuid.MatchString(target) || !uuid.MatchString(key) || !roleValid(role) || actor == target {
		return Invitation{}, ErrConflict
	}
	tx, err := s.begin(ctx, actor, org, false)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback(ctx)
	var id, priorTarget, priorRole, priorActor string
	err = tx.QueryRow(ctx, `SELECT id,target_id,role,inviter_id FROM organization_invitations WHERE organization_id=$1 AND idempotency_key=$2`, org, key).Scan(&id, &priorTarget, &priorRole, &priorActor)
	if err == nil {
		if priorTarget != target || priorRole != role || priorActor != actor {
			return Invitation{}, ErrConflict
		}
		return invitation(ctx, tx, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, err
	}
	var issuer, subject string
	if err = tx.QueryRow(ctx, `SELECT issuer,subject FROM owners WHERE id=$1 AND issuer=$2`, target, s.Issuer).Scan(&issuer, &subject); err != nil {
		return Invitation{}, ErrConflict
	}
	var conflict bool
	if err = tx.QueryRow(ctx, `SELECT (SELECT owner_id=$2 FROM organizations WHERE id=$1) OR EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND owner_id=$2 AND state='active') OR (SELECT count(*)>=30 FROM organization_members WHERE organization_id=$1 AND state='active') OR (SELECT count(*)>=30 FROM organization_invitations WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org, target).Scan(&conflict); err != nil {
		return Invitation{}, err
	}
	if conflict {
		return Invitation{}, ErrConflict
	}
	expired, err := tx.Exec(ctx, `UPDATE organization_invitations SET state='expired' WHERE organization_id=$1 AND target_id=$2 AND state='pending' AND expires_at<=clock_timestamp()`, org, target)
	if err != nil {
		return Invitation{}, err
	}
	if expired.RowsAffected() > 0 {
		if err = audit(ctx, tx, org, actor, "expire", &target, nil); err != nil {
			return Invitation{}, err
		}
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_invitations WHERE organization_id=$1 AND target_id=$2 AND state='pending')`, org, target).Scan(&pending); err != nil {
		return Invitation{}, err
	}
	if pending {
		return Invitation{}, ErrConflict
	}
	if err = tx.QueryRow(ctx, `INSERT INTO organization_invitations(organization_id,inviter_id,target_id,target_issuer,target_subject,role,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, org, actor, target, issuer, subject, role, key).Scan(&id); err != nil {
		return Invitation{}, err
	}
	if err = audit(ctx, tx, org, actor, "invite", &target, &role); err != nil {
		return Invitation{}, err
	}
	result, err := invitation(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Invitation{}, err
	}
	return result, nil
}
func (s *Store) Accept(ctx context.Context, actor, id string) (Invitation, error) {
	if !s.Enabled || !uuid.MatchString(id) {
		return Invitation{}, ErrNotFound
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return Invitation{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback(ctx)
	// Lock organization before invitations/members, matching every owner mutation.
	var org string
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM organization_invitations WHERE id=$1 AND target_id=$2`, id, actor).Scan(&org); err != nil {
		return Invitation{}, ErrNotFound
	}
	var current string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&current); err != nil {
		return Invitation{}, err
	}
	var role, state, inviter string
	var eligible, identity bool
	if err = tx.QueryRow(ctx, `SELECT i.role,i.state,i.inviter_id,i.expires_at>clock_timestamp(),o.issuer=i.target_issuer AND o.subject=i.target_subject FROM organization_invitations i JOIN owners o ON o.id=i.target_id WHERE i.id=$1 AND i.target_id=$2 FOR UPDATE OF i`, id, actor).Scan(&role, &state, &inviter, &eligible, &identity); err != nil {
		return Invitation{}, ErrNotFound
	}
	if !identity {
		return Invitation{}, ErrConflict
	}
	if state == "accepted" {
		return invitation(ctx, tx, id)
	}
	if state != "pending" || !eligible || current != inviter || current == actor {
		return Invitation{}, ErrConflict
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM organization_members WHERE organization_id=$1 AND state='active'`, org).Scan(&count); err != nil {
		return Invitation{}, err
	}
	if count >= 30 {
		return Invitation{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) VALUES($1,$2,$3,'active') ON CONFLICT(organization_id,owner_id) DO UPDATE SET role=$3,state='active',updated_at=now()`, org, actor, role); err != nil {
		return Invitation{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_invitations SET state='accepted' WHERE id=$1`, id); err != nil {
		return Invitation{}, err
	}
	if err = audit(ctx, tx, org, actor, "accept", &actor, &role); err != nil {
		return Invitation{}, err
	}
	result, err := invitation(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Invitation{}, err
	}
	return result, nil
}
func (s *Store) ChangeMember(ctx context.Context, actor, org, target, role string) error {
	if !uuid.MatchString(target) || (role != "revoked" && !roleValid(role)) {
		return ErrConflict
	}
	tx, err := s.begin(ctx, actor, org, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		return err
	}
	if owner == target {
		return ErrConflict
	}
	var current, state string
	err = tx.QueryRow(ctx, `SELECT role,state FROM organization_members WHERE organization_id=$1 AND owner_id=$2`, org, target).Scan(&current, &state)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if role == "revoked" {
		changed, err := tx.Exec(ctx, `UPDATE organization_members SET state='revoked',updated_at=now() WHERE organization_id=$1 AND owner_id=$2 AND state='active'`, org, target)
		if err != nil {
			return err
		}
		pending, err := tx.Exec(ctx, `UPDATE organization_invitations SET state='revoked' WHERE organization_id=$1 AND target_id=$2 AND state='pending'`, org, target)
		if err != nil {
			return err
		}
		if changed.RowsAffected()+pending.RowsAffected() > 0 {
			if err = audit(ctx, tx, org, actor, "revoke", &target, nil); err != nil {
				return err
			}
		}
	} else {
		if errors.Is(err, pgx.ErrNoRows) || state != "active" {
			return ErrNotFound
		}
		if current != role {
			if _, err = tx.Exec(ctx, `UPDATE organization_members SET role=$3,updated_at=now() WHERE organization_id=$1 AND owner_id=$2`, org, target, role); err != nil {
				return err
			}
			if err = audit(ctx, tx, org, actor, "role", &target, &role); err != nil {
				return err
			}
		}
	}
	if role != "administrator" {
		if _, err = tx.Exec(ctx, `UPDATE organization_transfers SET state='revoked' WHERE organization_id=$1 AND next_owner_id=$2 AND state='pending'`, org, target); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Rename(ctx context.Context, actor, org, name string) error {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 100 {
		return ErrConflict
	}
	tx, err := s.begin(ctx, actor, org, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	changed, err := tx.Exec(ctx, `UPDATE organizations SET name=$2 WHERE id=$1 AND name<>$2`, org, name)
	if err != nil {
		return err
	}
	if changed.RowsAffected() > 0 {
		if err = audit(ctx, tx, org, actor, "rename", nil, nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Shared(ctx context.Context, actor, org string) (json.RawMessage, error) {
	if !s.Enabled || !uuid.MatchString(org) {
		return nil, ErrNotFound
	}
	var result []byte
	err := s.Pool.QueryRow(ctx, `SELECT jsonb_build_object('organizationId',o.id,'name',o.name,'role',CASE WHEN o.owner_id=$2 THEN 'owner' ELSE m.role END,'instances',coalesce((SELECT jsonb_agg(jsonb_build_object('id',d.id,'name',d.name,'state',d.state,'kind',d.kind) ORDER BY d.created_at,d.id) FROM (SELECT id,name,state,kind,created_at FROM deployments WHERE organization_id=o.id ORDER BY created_at,id LIMIT 100) d),'[]'::jsonb),'hasMore',(SELECT count(*)>100 FROM deployments WHERE organization_id=o.id)) FROM organizations o LEFT JOIN organization_members m ON m.organization_id=o.id AND m.owner_id=$2 AND m.state='active' WHERE o.id=$1 AND (o.owner_id=$2 OR m.owner_id=$2)`, org, actor).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}

// Transfer offers responsibility; it does not change ownership until the target accepts.
func (s *Store) Transfer(ctx context.Context, actor, org, target, key string, confirmed bool) (string, error) {
	if !uuid.MatchString(target) || !uuid.MatchString(key) || !confirmed || actor == target {
		return "", ErrConflict
	}
	tx, err := s.begin(ctx, actor, org, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id, prior, next string
	err = tx.QueryRow(ctx, `SELECT id,previous_owner_id,next_owner_id FROM organization_transfers WHERE organization_id=$1 AND idempotency_key=$2`, org, key).Scan(&id, &prior, &next)
	if err == nil {
		if prior != actor || next != target {
			return "", ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND owner_id=$2 AND state='active' AND role='administrator') AND NOT EXISTS(SELECT 1 FROM deployments WHERE organization_id=$1) AND NOT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE organization_id=$1)`, org, target).Scan(&eligible); err != nil {
		return "", err
	}
	if !eligible {
		return "", ErrConflict
	}
	expired, err := tx.Exec(ctx, `UPDATE organization_transfers SET state='expired' WHERE organization_id=$1 AND state='pending' AND expires_at<=clock_timestamp()`, org)
	if err != nil {
		return "", err
	}
	if expired.RowsAffected() > 0 {
		if err = audit(ctx, tx, org, actor, "expire", nil, nil); err != nil {
			return "", err
		}
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending')`, org).Scan(&pending); err != nil {
		return "", err
	}
	if pending {
		return "", ErrConflict
	}
	if err = tx.QueryRow(ctx, `INSERT INTO organization_transfers(organization_id,previous_owner_id,next_owner_id,idempotency_key) VALUES($1,$2,$3,$4) RETURNING id`, org, actor, target, key).Scan(&id); err != nil {
		return "", err
	}
	if err = audit(ctx, tx, org, actor, "transfer_offer", &target, nil); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
func (s *Store) AcceptTransfer(ctx context.Context, actor, id string, confirmed bool) (string, error) {
	if !s.Enabled || !uuid.MatchString(id) || !confirmed {
		return "", ErrConflict
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM organization_transfers WHERE id=$1 AND next_owner_id=$2`, id, actor).Scan(&org); err != nil {
		return "", ErrNotFound
	}
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1 FOR UPDATE`, org).Scan(&owner); err != nil {
		return "", ErrNotFound
	}
	var prior, state string
	var unexpired bool
	if err = tx.QueryRow(ctx, `SELECT previous_owner_id,state,expires_at>clock_timestamp() FROM organization_transfers WHERE id=$1 AND next_owner_id=$2 FOR UPDATE`, id, actor).Scan(&prior, &state, &unexpired); err != nil {
		return "", ErrNotFound
	}
	if state == "accepted" {
		return id, nil
	}
	if state != "pending" || !unexpired || owner != prior {
		return "", ErrConflict
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=$1 AND owner_id=$2 AND state='active' AND role='administrator') AND NOT EXISTS(SELECT 1 FROM deployments WHERE organization_id=$1) AND NOT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE organization_id=$1)`, org, actor).Scan(&eligible); err != nil {
		return "", err
	}
	if !eligible {
		return "", ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET owner_id=$2 WHERE id=$1`, org, actor); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_transfers SET state='accepted' WHERE id=$1`, id); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_members SET state='revoked',updated_at=now() WHERE organization_id=$1 AND owner_id=$2`, org, actor); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_invitations SET state='revoked' WHERE organization_id=$1 AND state='pending'`, org); err != nil {
		return "", err
	}
	if err = audit(ctx, tx, org, actor, "transfer_accept", &actor, nil); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
func (s *Store) Mine(ctx context.Context, actor string) (json.RawMessage, error) {
	if !s.Enabled {
		return nil, ErrNotFound
	}
	var result []byte
	err := s.Pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('organizationId',id,'name',name,'role',role) ORDER BY created_at,id),'[]'::jsonb) FROM (SELECT o.id,o.name,o.created_at,CASE WHEN o.owner_id=$1 THEN 'owner' ELSE m.role END role FROM organizations o LEFT JOIN organization_members m ON m.organization_id=o.id AND m.owner_id=$1 AND m.state='active' WHERE o.owner_id=$1 OR m.owner_id=$1 ORDER BY o.created_at,o.id LIMIT 100) visible`, actor).Scan(&result)
	return result, err
}
func (s *Store) Incoming(ctx context.Context, actor string) (json.RawMessage, error) {
	if !s.Enabled {
		return nil, ErrNotFound
	}
	var result []byte
	err := s.Pool.QueryRow(ctx, `SELECT jsonb_build_object('invitations',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'organizationId',organization_id,'role',role,'expiresAt',expires_at)) FROM (SELECT i.id,i.organization_id,i.role,i.expires_at FROM organization_invitations i JOIN owners t ON t.id=i.target_id JOIN organizations o ON o.id=i.organization_id WHERE i.target_id=$1 AND i.state='pending' AND i.expires_at>clock_timestamp() AND t.issuer=i.target_issuer AND t.subject=i.target_subject AND o.owner_id=i.inviter_id ORDER BY i.created_at,i.id LIMIT 30) p),'[]'::jsonb),'transfers',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'organizationId',organization_id,'expiresAt',expires_at)) FROM (SELECT t.id,t.organization_id,t.expires_at FROM organization_transfers t JOIN organizations o ON o.id=t.organization_id JOIN organization_members m ON m.organization_id=t.organization_id AND m.owner_id=t.next_owner_id AND m.role='administrator' AND m.state='active' WHERE t.next_owner_id=$1 AND t.state='pending' AND t.expires_at>clock_timestamp() AND o.owner_id=t.previous_owner_id ORDER BY t.created_at,t.id LIMIT 30) p),'[]'::jsonb))`, actor).Scan(&result)
	return result, err
}
func (s *Store) Manage(ctx context.Context, actor, org string) (json.RawMessage, error) {
	if !s.Enabled || !uuid.MatchString(org) {
		return nil, ErrNotFound
	}
	var result []byte
	err := s.Pool.QueryRow(ctx, `SELECT jsonb_build_object('members',coalesce((SELECT jsonb_agg(jsonb_build_object('identityId',owner_id,'role',role,'state',state) ORDER BY owner_id) FROM (SELECT owner_id,role,state FROM organization_members WHERE organization_id=o.id ORDER BY (state='active') DESC,updated_at DESC,owner_id LIMIT 100) m),'[]'::jsonb),'membersTruncated',(SELECT count(*)>100 FROM organization_members WHERE organization_id=o.id),'invitations',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'targetId',target_id,'role',role,'state',CASE WHEN state='pending' AND expires_at<=clock_timestamp() THEN 'expired' ELSE state END,'expiresAt',expires_at) ORDER BY created_at DESC,id) FROM (SELECT id,target_id,role,state,expires_at,created_at FROM organization_invitations WHERE organization_id=o.id ORDER BY created_at DESC,id LIMIT 100) i),'[]'::jsonb),'audit',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id::text,'actorId',actor_id,'targetId',target_id,'action',action,'role',role,'createdAt',created_at) ORDER BY id DESC) FROM (SELECT id,actor_id,target_id,action,role,created_at FROM organization_access_events WHERE organization_id=o.id ORDER BY id DESC LIMIT 100) e),'[]'::jsonb)) FROM organizations o WHERE o.id=$1 AND o.owner_id=$2`, org, actor).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}
