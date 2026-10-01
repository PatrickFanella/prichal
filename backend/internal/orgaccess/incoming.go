package orgaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// IncomingOffer is current identity-bound offer metadata, not granted permission.
type IncomingOffer struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Role           *string   `json:"role,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// IncomingOfferPage holds one bounded page of eligible invitation or transfer offers.
type IncomingOfferPage struct {
	Items     []IncomingOffer `json:"items"`
	HasMore   bool            `json:"hasMore"`
	NextAfter *string         `json:"nextAfter"`
}
type incomingCursor struct {
	ActorID   string    `json:"actorId"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func incomingAfter(actor, kind, cursor string) (*incomingCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > 320 {
		return nil, ErrCursor
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != cursor {
		return nil, ErrCursor
	}
	var value incomingCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.ActorID != actor || value.Kind != kind || !uuid.MatchString(value.ActorID) || !uuid.MatchString(value.ID) || value.CreatedAt.IsZero() {
		return nil, ErrCursor
	}
	return &value, nil
}

// IncomingInvitationsPage rechecks verified recipient identity and current inviter ownership.
func (s *Store) IncomingInvitationsPage(ctx context.Context, actor, cursor string) (IncomingOfferPage, error) {
	return s.incomingPage(ctx, actor, "invitations", cursor)
}

// IncomingTransfersPage rechecks current administrator membership and offering ownership.
// Acceptance separately rechecks empty-organization and billing eligibility.
func (s *Store) IncomingTransfersPage(ctx context.Context, actor, cursor string) (IncomingOfferPage, error) {
	return s.incomingPage(ctx, actor, "transfers", cursor)
}
func (s *Store) incomingPage(ctx context.Context, actor, kind, cursor string) (IncomingOfferPage, error) {
	page := IncomingOfferPage{Items: []IncomingOffer{}}
	if !s.Enabled || !uuid.MatchString(actor) {
		return page, ErrNotFound
	}
	after, err := incomingAfter(actor, kind, cursor)
	if err != nil {
		return page, err
	}
	var at *time.Time
	var id *string
	if after != nil {
		at = &after.CreatedAt
		id = &after.ID
	}
	var query string
	switch kind {
	case "invitations":
		query = `SELECT i.id::text,i.organization_id::text,i.role,i.expires_at,i.created_at FROM organization_invitations i JOIN owners t ON t.id=i.target_id JOIN organizations o ON o.id=i.organization_id WHERE i.target_id=$1 AND i.state='pending' AND i.expires_at>statement_timestamp() AND t.issuer=i.target_issuer AND t.subject=i.target_subject AND o.owner_id=i.inviter_id AND ($2::timestamptz IS NULL OR (i.created_at,i.id)>($2,$3::uuid)) ORDER BY i.created_at,i.id LIMIT 51`
	case "transfers":
		query = `SELECT t.id::text,t.organization_id::text,NULL::text,t.expires_at,t.created_at FROM organization_transfers t JOIN organizations o ON o.id=t.organization_id JOIN organization_members m ON m.organization_id=t.organization_id AND m.owner_id=t.next_owner_id AND m.role='administrator' AND m.state='active' WHERE t.next_owner_id=$1 AND t.state='pending' AND t.expires_at>statement_timestamp() AND o.owner_id=t.previous_owner_id AND ($2::timestamptz IS NULL OR (t.created_at,t.id)>($2,$3::uuid)) ORDER BY t.created_at,t.id LIMIT 51`
	default:
		return page, ErrCursor
	}
	rows, err := s.Pool.Query(ctx, query, actor, at, id)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	var latest time.Time
	for rows.Next() {
		var item IncomingOffer
		var created time.Time
		if err = rows.Scan(&item.ID, &item.OrganizationID, &item.Role, &item.ExpiresAt, &created); err != nil {
			return page, err
		}
		if len(page.Items) == 50 {
			page.HasMore = true
			break
		}
		page.Items = append(page.Items, item)
		latest = created
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if page.HasMore {
		raw, err := json.Marshal(incomingCursor{actor, kind, latest.UTC(), page.Items[49].ID})
		if err != nil {
			return page, err
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		page.NextAfter = &next
	}
	return page, nil
}
