package orgaccess

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrCursor identifies malformed or cross-organization pagination intent.
var ErrCursor = errors.New("invalid organization audit cursor")

// AuditEvent contains management access metadata only, without native content.
type AuditEvent struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actorId"`
	TargetID  *string   `json:"targetId"`
	Action    string    `json:"action"`
	Role      *string   `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// AuditPage is bounded recorded history, not a frozen multi-page snapshot.
type AuditPage struct {
	Items      []AuditEvent `json:"items"`
	HasMore    bool         `json:"hasMore"`
	NextBefore *string      `json:"nextBefore"`
}

func auditCursor(org, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(org + ":" + id))
}
func auditBefore(org, cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	if len(cursor) > 128 {
		return 0, ErrCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor {
		return 0, ErrCursor
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 || parts[0] != org || !uuid.MatchString(parts[0]) {
		return 0, ErrCursor
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != parts[1] {
		return 0, ErrCursor
	}
	return id, nil
}
func auditQuery(raw string) (string, error) {
	if len(raw) > 256 {
		return "", ErrCursor
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) > 1 {
		return "", ErrCursor
	}
	for key, value := range values {
		if key != "before" || len(value) != 1 || value[0] == "" || len(value[0]) > 128 {
			return "", ErrCursor
		}
	}
	return values.Get("before"), nil
}

// Audit reads one page after rechecking current ownership. Cursors confer no access.
func (s *Store) Audit(ctx context.Context, actor, org, cursor string) (AuditPage, error) {
	page := AuditPage{Items: []AuditEvent{}}
	if !s.Enabled || !uuid.MatchString(org) {
		return page, ErrNotFound
	}
	before, err := auditBefore(org, cursor)
	if err != nil {
		return page, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2`, org, actor).Scan(&found); errors.Is(err, pgx.ErrNoRows) {
		return page, ErrNotFound
	} else if err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, `SELECT e.id::text,e.actor_id::text,e.target_id::text,e.action,e.role,e.created_at FROM organization_access_events e WHERE e.organization_id=$1 AND ($2::bigint=0 OR e.id<$2) ORDER BY e.id DESC LIMIT 51`, org, before)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var event AuditEvent
		if err = rows.Scan(&event.ID, &event.ActorID, &event.TargetID, &event.Action, &event.Role, &event.CreatedAt); err != nil {
			rows.Close()
			return page, err
		}
		page.Items = append(page.Items, event)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > 50 {
		page.HasMore = true
		page.Items = page.Items[:50]
		next := auditCursor(org, page.Items[49].ID)
		page.NextBefore = &next
	}
	if err = tx.Commit(ctx); err != nil {
		return page, err
	}
	return page, nil
}
