package support

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func seedJourney(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`INSERT INTO deployment_events(deployment_id,state,message,created_at) SELECT $1,'queued','PRIVATE_HOST_ERROR','2026-10-01T12:00:00Z' FROM generate_series(1,52)`,
		`INSERT INTO deployment_events(deployment_id,state,message,created_at) VALUES($1,'PRIVATE_UNKNOWN_STATE','PRIVATE_HOST_ERROR','2026-10-01T11:59:00Z')`,
		`WITH op AS(INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,message) VALUES($1,'22222222-2222-4222-8222-222222222222',gen_random_uuid(),'PRIVATE_INVITATION') RETURNING id) INSERT INTO enrollment_events(enrollment_id,state,message,created_at) SELECT id,'pending','PRIVATE_INVITATION','2026-10-01T12:00:00Z' FROM op`,
		`INSERT INTO handle_events(deployment_id,handle,state,message,created_at) VALUES($1,'private.example.net','checking','PRIVATE_DNS_ERROR','2026-10-01T12:00:00Z')`,
		`WITH op AS(INSERT INTO service_operations(deployment_id,idempotency_key,generation,kind,desired_state,prior_state,reason,actor,state) VALUES($1,gen_random_uuid(),1,'service','suspended','running','maintenance','PRIVATE_OPERATOR','applied') RETURNING id) INSERT INTO service_operation_events(operation_id,state,created_at) SELECT id,'applied','2026-10-01T12:00:00Z' FROM op`,
		`WITH op AS(INSERT INTO service_operations(deployment_id,idempotency_key,generation,kind,desired_state,prior_state,reason,actor,state) VALUES($1,gen_random_uuid(),2,'account','running','suspended','maintenance','PRIVATE_OPERATOR','applied') RETURNING id) INSERT INTO service_operation_events(operation_id,state,created_at) SELECT id,'applied','2026-10-01T12:00:00Z' FROM op`,
		`WITH op AS(INSERT INTO export_operations(deployment_id,owner_id,idempotency_key,account_did) VALUES($1,'22222222-2222-4222-8222-222222222222',gen_random_uuid(),'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa') RETURNING id) INSERT INTO export_events(export_id,state,created_at) SELECT id,'queued','2026-10-01T12:00:00Z' FROM op`,
		`WITH op AS(INSERT INTO hosting_exits(deployment_id,owner_id,idempotency_key,export_id,delay_seconds,retention_days,service_end_at,retention_review_at) SELECT $1,'22222222-2222-4222-8222-222222222222',gen_random_uuid(),id,30,7,now()+interval '30 seconds',now()+interval '7 days 30 seconds' FROM export_operations WHERE deployment_id=$1 RETURNING id) INSERT INTO hosting_exit_events(exit_id,state,created_at) SELECT id,'scheduled','2026-10-01T12:00:00Z' FROM op`,
	}
	for _, statement := range statements {
		if _, err := s.Pool.Exec(ctx, statement, instance); err != nil {
			t.Fatal(err)
		}
	}
}
func TestJourneyTiedPaginationScopeAndRedaction(t *testing.T) {
	s, _ := fixture(t)
	seedJourney(t, s)
	ctx := context.Background()
	cursor := ""
	seen := map[string]bool{}
	kinds := map[string]bool{}
	pages := 0
	for {
		page, err := s.Journey(ctx, bob, instance, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		data, _ := json.Marshal(page)
		if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "private.example") || strings.Contains(string(data), "did:plc") || strings.Contains(string(data), "idempotency") {
			t.Fatal("private history escaped")
		}
		if len(page.Items) > 50 {
			t.Fatal("unbounded page")
		}
		for _, event := range page.Items {
			if seen[event.Key] {
				t.Fatal("tie repeated event")
			}
			seen[event.Key] = true
			kinds[event.Category] = true
			if event.Message == "" || event.NextStep == "" {
				t.Fatal("guidance absent")
			}
		}
		if !page.HasMore {
			if page.NextBefore != nil {
				t.Fatal("unexpected final cursor")
			}
			break
		}
		if page.NextBefore == nil {
			t.Fatal("cursor absent")
		}
		cursor = *page.NextBefore
	}
	if len(seen) != 59 || len(kinds) != 7 || pages != 2 {
		t.Fatalf("incomplete history: events=%d categories=%d pages=%d", len(seen), len(kinds), pages)
	}
	if _, err := s.Journey(ctx, alice, instance, ""); err != ErrNotFound {
		t.Fatal("operator bypassed owner history")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, alice, organization); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Journey(ctx, bob, instance, ""); err != ErrNotFound {
		t.Fatal("prior owner retained scope")
	}
	page, err := s.Journey(ctx, alice, instance, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range page.Items {
		if event.Category == "enrollment" || event.Category == "export" || event.Category == "exit" {
			t.Fatal("prior owner private operations inherited")
		}
	}
	var inspections int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM support_inspections`).Scan(&inspections); err != nil || inspections != 0 {
		t.Fatal("read-only owner projection changed operator audit")
	}
}
func TestJourneyRejectsInvalidCursorAndQueries(t *testing.T) {
	s, actions := fixture(t)
	seedJourney(t, s)
	for _, raw := range []string{"!", strings.Repeat("a", 257), base64.RawURLEncoding.EncodeToString([]byte(instance + "|invalid|export|1")), base64.RawURLEncoding.EncodeToString([]byte(organization + "|2026-10-01T12:00:00Z|export|1")), base64.RawURLEncoding.EncodeToString([]byte(instance + "|2026-10-01T12:00:00Z|private|1")), base64.RawURLEncoding.EncodeToString([]byte(instance + "|2026-10-01T12:00:00Z|export|-1"))} {
		if _, err := s.Journey(context.Background(), bob, instance, raw); err != ErrCursor {
			t.Fatal("invalid cursor accepted")
		}
	}
	mux := http.NewServeMux()
	s.Register(sessionFixture{bob}, actions)(mux)
	for _, query := range []string{"?before=!", "?before=a&before=b", "?unknown=x", "?before=%"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/deployments/"+instance+"/journey"+query, nil))
		if response.Code != 400 {
			t.Fatalf("invalid query status %d", response.Code)
		}
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/deployments/"+instance+"/journey", nil))
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner route unavailable or cacheable")
	}
}
func TestJourneyGuidanceRetainsModerationAndExpiryBoundaries(t *testing.T) {
	for _, kind := range []string{"account", "service"} {
		message, next := journeyGuidance(kind, "applied", "running")
		if message == "" || !strings.Contains(next, "independent holds") {
			t.Fatal("resume erased moderation boundary")
		}
	}
	_, next := journeyGuidance("export", "ready", "")
	if !strings.Contains(next, "current download window") {
		t.Fatal("historical readiness promised current download")
	}
	_, next = journeyGuidance("exit", "review", "")
	if !strings.Contains(next, "deletion is not scheduled") {
		t.Fatal("review implied deletion")
	}
	message, _ := journeyGuidance("deployment", "PRIVATE_NEW_STATE", "")
	if strings.Contains(message, "PRIVATE") {
		t.Fatal("unknown state leaked through guidance")
	}
}
