package orgaccess

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// MemberRecord exposes current management membership, without identity-provider details.
type MemberRecord struct {
	IdentityID string `json:"identityId"`
	Role       string `json:"role"`
	State      string `json:"state"`
}

// MemberPage is bounded current membership, including retained revocations.
type MemberPage struct {
	Items     []MemberRecord `json:"items"`
	HasMore   bool           `json:"hasMore"`
	NextAfter *string        `json:"nextAfter"`
}

// InvitationReceipt is recorded invitation metadata, not permission or an acceptance token.
type InvitationReceipt struct {
	ID        string    `json:"id"`
	TargetID  string    `json:"targetId"`
	Role      string    `json:"role"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// InvitationPage is a bounded owner-only receipt page.
type InvitationPage struct {
	Items     []InvitationReceipt `json:"items"`
	HasMore   bool                `json:"hasMore"`
	NextAfter *string             `json:"nextAfter"`
}

func memberCursor(actor, org, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(actor + ":" + org + ":members:" + id))
}
func memberAfter(actor, org, cursor string) (*string, error) {
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > 384 {
		return nil, ErrCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor {
		return nil, ErrCursor
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 4 || parts[0] != actor || parts[1] != org || parts[2] != "members" || !uuid.MatchString(parts[3]) {
		return nil, ErrCursor
	}
	return &parts[3], nil
}
func (s *Store) ownerRead(ctx context.Context, actor, org string) (pgx.Tx, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	var found string
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2`, org, actor).Scan(&found)
	if err != nil {
		_ = tx.Rollback(ctx) // Best effort cleanup; preserve the authorization/query error.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return tx, nil
}

// MembersPage rechecks current ownership and orders by immutable identity reference.
// Role/revocation updates cannot move a retained row across the continuation.
func (s *Store) MembersPage(ctx context.Context, actor, org, cursor string) (MemberPage, error) {
	page := MemberPage{Items: []MemberRecord{}}
	if !s.Enabled || !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return page, ErrNotFound
	}
	after, err := memberAfter(actor, org, cursor)
	if err != nil {
		return page, err
	}
	tx, err := s.ownerRead(ctx, actor, org)
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT owner_id::text,role,state FROM organization_members WHERE organization_id=$1 AND ($2::uuid IS NULL OR owner_id>$2) ORDER BY owner_id LIMIT 51`, org, after)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var item MemberRecord
		if err = rows.Scan(&item.IdentityID, &item.Role, &item.State); err != nil {
			rows.Close()
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > 50 {
		page.HasMore = true
		page.Items = page.Items[:50]
		next := memberCursor(actor, org, page.Items[49].IdentityID)
		page.NextAfter = &next
	}
	if err = tx.Commit(ctx); err != nil {
		return page, err
	}
	return page, nil
}

// InvitationReceiptsPage rechecks current ownership and computes expiry without mutation.
func (s *Store) InvitationReceiptsPage(ctx context.Context, actor, org, cursor string) (InvitationPage, error) {
	page := InvitationPage{Items: []InvitationReceipt{}}
	if !s.Enabled || !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return page, ErrNotFound
	}
	after, err := scopedAfter(actor, org, "invitation-receipts", cursor)
	if err != nil {
		return page, err
	}
	var at *time.Time
	var id *string
	if after != nil {
		at = &after.CreatedAt
		id = &after.ID
	}
	tx, err := s.ownerRead(ctx, actor, org)
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT i.id::text,i.target_id::text,i.role,CASE WHEN i.state='pending' AND i.expires_at<=statement_timestamp() THEN 'expired' ELSE i.state END,i.expires_at,i.created_at FROM organization_invitations i WHERE i.organization_id=$1 AND ($2::timestamptz IS NULL OR (i.created_at,i.id)>($2,$3::uuid)) ORDER BY i.created_at,i.id LIMIT 51`, org, at, id)
	if err != nil {
		return page, err
	}
	var latest time.Time
	for rows.Next() {
		var item InvitationReceipt
		var created time.Time
		if err = rows.Scan(&item.ID, &item.TargetID, &item.Role, &item.State, &item.ExpiresAt, &created); err != nil {
			rows.Close()
			return page, err
		}
		if len(page.Items) == 50 {
			page.HasMore = true
			break
		}
		page.Items = append(page.Items, item)
		latest = created
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	if page.HasMore {
		next, err := scopedNext(actor, org, "invitation-receipts", page.Items[49].ID, latest)
		if err != nil {
			return page, err
		}
		page.NextAfter = &next
	}
	if err = tx.Commit(ctx); err != nil {
		return page, err
	}
	return page, nil
}
