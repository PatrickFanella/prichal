package support

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrCursor = errors.New("invalid journey cursor")

const reviewMessage = "A hosting event requires review."

var categories = map[string]bool{"deployment": true, "enrollment": true, "domain": true, "service": true, "account": true, "export": true, "exit": true}

type JourneyEvent struct {
	Key       string    `json:"key"`
	Category  string    `json:"category"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
	Message   string    `json:"message"`
	NextStep  string    `json:"nextStep"`
}
type JourneyPage struct {
	Items      []JourneyEvent `json:"items"`
	HasMore    bool           `json:"hasMore"`
	NextBefore *string        `json:"nextBefore"`
}
type journeyCursor struct {
	at       time.Time
	category string
	id       int64
}

func decodeCursor(raw, deployment string) (journeyCursor, error) {
	if raw == "" {
		return journeyCursor{}, nil
	}
	if len(raw) > 256 {
		return journeyCursor{}, ErrCursor
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return journeyCursor{}, ErrCursor
	}
	fields := strings.Split(string(data), "|")
	if len(fields) != 4 || fields[0] != deployment || !categories[fields[2]] {
		return journeyCursor{}, ErrCursor
	}
	at, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil || at.IsZero() {
		return journeyCursor{}, ErrCursor
	}
	id, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || id <= 0 {
		return journeyCursor{}, ErrCursor
	}
	return journeyCursor{at, fields[2], id}, nil
}
func encodeCursor(deployment string, c journeyCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(deployment + "|" + c.at.UTC().Format(time.RFC3339Nano) + "|" + c.category + "|" + strconv.FormatInt(c.id, 10)))
}

// Journey reads a bounded owner projection of durable audit events. Raw messages,
// identities, request keys, domain values and credentials are never selected.
func (s *Store) Journey(ctx context.Context, owner, deployment, before string) (JourneyPage, error) {
	page := JourneyPage{Items: []JourneyEvent{}}
	if !uuid.MatchString(deployment) {
		return page, ErrNotFound
	}
	cursor, err := decodeCursor(before, deployment)
	if err != nil {
		return page, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2`, deployment, owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return page, ErrNotFound
	}
	if err != nil {
		return page, err
	}
	var at any
	if before != "" {
		at = cursor.at
	}
	rows, err := tx.Query(ctx, `WITH history AS (
 SELECT e.id,'deployment'::text category,e.state,e.created_at,''::text desired FROM deployment_events e WHERE e.deployment_id=$1
 UNION ALL SELECT e.id,'enrollment',e.state,e.created_at,'' FROM enrollment_events e JOIN enrollment_operations o ON o.id=e.enrollment_id WHERE o.deployment_id=$1 AND o.owner_id=$2
 UNION ALL SELECT e.id,'domain',e.state,e.created_at,'' FROM handle_events e WHERE e.deployment_id=$1
 UNION ALL SELECT e.id,o.kind,e.state,e.created_at,o.desired_state FROM service_operation_events e JOIN service_operations o ON o.id=e.operation_id WHERE o.deployment_id=$1
 UNION ALL SELECT e.id,'export',e.state,e.created_at,'' FROM export_events e JOIN export_operations o ON o.id=e.export_id WHERE o.deployment_id=$1 AND o.owner_id=$2
 UNION ALL SELECT e.id,'exit',e.state,e.created_at,'' FROM hosting_exit_events e JOIN hosting_exits o ON o.id=e.exit_id WHERE o.deployment_id=$1 AND o.owner_id=$2
 ) SELECT id,category,state,created_at,desired FROM history WHERE ($3::timestamptz IS NULL OR (created_at,category COLLATE "C",id)<($3::timestamptz,$4::text COLLATE "C",$5::bigint)) ORDER BY created_at DESC,category COLLATE "C" DESC,id DESC LIMIT 51`, deployment, owner, at, cursor.category, cursor.id)
	if err != nil {
		return page, err
	}
	var last journeyCursor
	for rows.Next() {
		var event JourneyEvent
		var id int64
		var desired string
		if err = rows.Scan(&id, &event.Category, &event.State, &event.CreatedAt, &desired); err != nil {
			rows.Close()
			return page, err
		}
		if len(page.Items) == 50 {
			page.HasMore = true
			break
		}
		event.Key = event.Category + ":" + strconv.FormatInt(id, 10)
		event.Message, event.NextStep = journeyGuidance(event.Category, event.State, desired)
		if event.Message == reviewMessage {
			event.State = "review"
		}
		page.Items = append(page.Items, event)
		last = journeyCursor{event.CreatedAt, event.Category, id}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	if page.HasMore {
		value := encodeCursor(deployment, last)
		page.NextBefore = &value
	}
	if err = tx.Commit(ctx); err != nil {
		return page, err
	}
	return page, nil
}

func journeyGuidance(category, state, desired string) (string, string) {
	if category == "service" || category == "account" {
		label := "Hosting service"
		if category == "account" {
			label = "Native account access"
		}
		switch state {
		case "pending":
			return label + " change requested.", "Wait for the confirmed outcome."
		case "failed":
			return label + " change requires review.", "Contact support. Your data remains retained."
		case "applied":
			if desired == "suspended" {
				return label + " suspended.", "Review the current hosting and moderation status before requesting support."
			}
			if desired == "running" {
				return label + " resumed.", "Check current status; independent holds may still apply."
			}
		}
	}
	messages := map[string]map[string][2]string{
		"deployment": {"queued": {"Deployment requested.", "Wait for provisioning."}, "configuring": {"Server configuration started.", "Wait for endpoint checks."}, "checking": {"Endpoint verification started.", "Wait for a confirmed deployment result."}, "ready": {"Deployment became ready.", "Review current status and first-account enrollment."}, "failed": {"Deployment needs attention.", "Review current status and contact support before starting another request."}},
		"enrollment": {"pending": {"First-account invitation requested.", "Wait for the invitation."}, "invitation_ready": {"First-account invitation prepared.", "Open enrollment to review its current expiry and complete account setup."}, "account_created": {"Native account created.", "Wait for identity verification."}, "verified": {"Native account verified.", "Review the current domain and account status."}, "expired": {"First-account invitation expired.", "Open enrollment for supported invitation recovery."}, "failed": {"Enrollment needs review.", "Contact support before retrying."}, "superseded": {"First-account invitation replaced.", "Use the current enrollment invitation."}},
		"domain":     {"pending_dns": {"Domain proof is awaiting DNS.", "Review the current domain instructions."}, "checking": {"Domain proof check started.", "Wait for the check result."}, "pending_identity": {"DNS proof found; identity confirmation pending.", "Follow the current domain instructions."}, "verified": {"Domain proof verified.", "Review current identity and domain status."}, "conflict": {"Domain proof conflicts.", "Review domain instructions and contact support."}, "unavailable": {"Domain proof check unavailable.", "Review current status before retrying."}, "invalid": {"Domain proof is invalid.", "Review the current DNS instructions."}},
		"export":     {"queued": {"Portable export requested.", "Wait for a verified capture."}, "ready": {"Portable export prepared.", "Open exports to check its current download window and save a private copy."}, "failed": {"Portable export needs review.", "Contact support. Source data remains retained."}},
		"exit":       {"scheduled": {"Hosting exit scheduled.", "Review the current service-end and retention dates."}, "stopping": {"Hosting exit is stopping service.", "Wait for the confirmed outcome; data remains retained."}, "ended": {"Hosting service ended.", "Review retained exports and the supported reversal window."}, "resuming": {"Hosting exit reversal requested.", "Wait for confirmed service resumption."}, "reversed": {"Hosting exit reversed.", "Check current status; independent holds may still apply."}, "review": {"Hosting exit requires support review.", "Contact support. Data remains retained; deletion is not scheduled."}},
	}
	if value, ok := messages[category][state]; ok {
		return value[0], value[1]
	}
	return reviewMessage, "Review current status or contact support."
}
