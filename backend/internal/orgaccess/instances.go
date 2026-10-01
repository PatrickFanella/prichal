package orgaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SharedInstance is the fixed shared projection. It contains no native authority.
type SharedInstance struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Kind  string `json:"kind"`
}

// InstancePage carries bounded shared statuses, checked against current membership.
type InstancePage struct {
	Items     []SharedInstance `json:"items"`
	HasMore   bool             `json:"hasMore"`
	NextAfter *string          `json:"nextAfter"`
}

type scopedListCursor struct {
	ActorID        string    `json:"actorId"`
	OrganizationID string    `json:"organizationId"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"createdAt"`
	ID             string    `json:"id"`
}

func scopedAfter(actor, org, kind, cursor string) (*scopedListCursor, error) {
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
	var value scopedListCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.ActorID != actor || value.OrganizationID != org || value.Kind != kind || !uuid.MatchString(value.ActorID) || !uuid.MatchString(value.OrganizationID) || !uuid.MatchString(value.ID) || value.CreatedAt.IsZero() {
		return nil, ErrCursor
	}
	return &value, nil
}
func scopedNext(actor, org, kind, id string, at time.Time) (string, error) {
	raw, err := json.Marshal(scopedListCursor{actor, org, kind, at.UTC(), id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// InstancesPage is a read-only status page for the owner or a current member.
// A cursor is scoped to actor, organization and list kind, and grants no access.
func (s *Store) InstancesPage(ctx context.Context, actor, org, cursor string) (InstancePage, error) {
	page := InstancePage{Items: []SharedInstance{}}
	if !s.Enabled || !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return page, ErrNotFound
	}
	after, err := scopedAfter(actor, org, "instances", cursor)
	if err != nil {
		return page, err
	}
	var at *time.Time
	var id *string
	if after != nil {
		at = &after.CreatedAt
		id = &after.ID
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT o.id FROM organizations o LEFT JOIN organization_members m ON m.organization_id=o.id AND m.owner_id=$2 AND m.state='active' WHERE o.id=$1 AND (o.owner_id=$2 OR m.owner_id=$2)`, org, actor).Scan(&found); errors.Is(err, pgx.ErrNoRows) {
		return page, ErrNotFound
	} else if err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id::text,d.name,d.state,d.kind,d.created_at FROM deployments d WHERE d.organization_id=$1 AND ($2::timestamptz IS NULL OR (d.created_at,d.id)>($2,$3::uuid)) ORDER BY d.created_at,d.id LIMIT 51`, org, at, id)
	if err != nil {
		return page, err
	}
	var latest time.Time
	for rows.Next() {
		var item SharedInstance
		var createdAt time.Time
		if err = rows.Scan(&item.ID, &item.Name, &item.State, &item.Kind, &createdAt); err != nil {
			rows.Close()
			return page, err
		}
		if len(page.Items) == 50 {
			page.HasMore = true
			break
		}
		page.Items = append(page.Items, item)
		latest = createdAt
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	if page.HasMore {
		next, err := scopedNext(actor, org, "instances", page.Items[49].ID, latest)
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
