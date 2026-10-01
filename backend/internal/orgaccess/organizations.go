package orgaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// OrganizationSummary is the permitted management projection, not native custody.
type OrganizationSummary struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Role           string `json:"role"`
}

// OrganizationPage carries one bounded page and an identity-bound continuation.
type OrganizationPage struct {
	Items     []OrganizationSummary `json:"items"`
	HasMore   bool                  `json:"hasMore"`
	NextAfter *string               `json:"nextAfter"`
}

type organizationCursor struct {
	ActorID   string    `json:"actorId"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func organizationAfter(actor, cursor string) (*organizationCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > 256 {
		return nil, ErrCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor {
		return nil, ErrCursor
	}
	var value organizationCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.ActorID != actor || !uuid.MatchString(value.ActorID) || !uuid.MatchString(value.ID) || value.CreatedAt.IsZero() {
		return nil, ErrCursor
	}
	return &value, nil
}
func organizationQuery(raw string) (string, error) { return listQuery(raw, "after", 256) }

// MinePage rechecks current ownership/membership on every keyset page.
// Revoked organizations disappear without granting access through an old cursor.
func (s *Store) MinePage(ctx context.Context, actor, cursor string) (OrganizationPage, error) {
	page := OrganizationPage{Items: []OrganizationSummary{}}
	if !s.Enabled || !uuid.MatchString(actor) {
		return page, ErrNotFound
	}
	after, err := organizationAfter(actor, cursor)
	if err != nil {
		return page, err
	}
	var at *time.Time
	var id *string
	if after != nil {
		at = &after.CreatedAt
		id = &after.ID
	}
	rows, err := s.Pool.Query(ctx, `SELECT o.id::text,o.name,CASE WHEN o.owner_id=$1 THEN 'owner' ELSE m.role END,o.created_at FROM organizations o LEFT JOIN organization_members m ON m.organization_id=o.id AND m.owner_id=$1 AND m.state='active' WHERE (o.owner_id=$1 OR m.owner_id=$1) AND ($2::timestamptz IS NULL OR (o.created_at,o.id)>($2,$3::uuid)) ORDER BY o.created_at,o.id LIMIT 51`, actor, at, id)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	var latest time.Time
	for rows.Next() {
		var item OrganizationSummary
		var createdAt time.Time
		if err = rows.Scan(&item.OrganizationID, &item.Name, &item.Role, &createdAt); err != nil {
			return page, err
		}
		if len(page.Items) == 50 {
			page.HasMore = true
			break
		}
		page.Items = append(page.Items, item)
		latest = createdAt
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if page.HasMore {
		raw, err := json.Marshal(organizationCursor{actor, latest.UTC(), page.Items[49].OrganizationID})
		if err != nil {
			return page, err
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		page.NextAfter = &next
	}
	return page, nil
}
