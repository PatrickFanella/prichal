package orgaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func listID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
func populateOrganizations(t *testing.T, s *Store, n int) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO organizations(id,name,owner_id,created_at) SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'organization-'||n,$1,'2026-01-01T00:00:00.123456Z'::timestamptz FROM generate_series(1,$2::int) n`, alice, n); err != nil {
		t.Fatal(err)
	}
}
func TestOrganizationPagesPreserveTimestampTies(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateOrganizations(t, s, 117)
	cursor := ""
	seen := map[string]bool{}
	pages := 0
	for {
		page, err := s.MinePage(ctx, alice, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(page.Items) > 50 {
			t.Fatal("unbounded page")
		}
		for _, item := range page.Items {
			if seen[item.OrganizationID] || item.Role != "owner" {
				t.Fatal("duplicate or incorrect role")
			}
			seen[item.OrganizationID] = true
		}
		if !page.HasMore {
			if page.NextAfter != nil {
				t.Fatal("terminal cursor")
			}
			break
		}
		if page.NextAfter == nil {
			t.Fatal("missing cursor")
		}
		cursor = *page.NextAfter
	}
	if pages != 3 || len(seen) != 118 {
		t.Fatalf("traversal %d %d", pages, len(seen))
	}
	first, err := s.MinePage(ctx, alice, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Items[49].OrganizationID != listID(50) {
		t.Fatal("UUID tie ordering")
	}
	if _, err = s.MinePage(ctx, bob, *first.NextAfter); !errors.Is(err, ErrCursor) {
		t.Fatalf("cross-identity cursor: %v", err)
	}
	empty, err := s.MinePage(ctx, bob, "")
	if err != nil || len(empty.Items) != 0 || empty.HasMore {
		t.Fatalf("foreign list: %+v %v", empty, err)
	}
	if count(t, s, "organization_access_events") != 0 {
		t.Fatal("list added audit work")
	}
	s.Enabled = false
	if _, err = s.MinePage(ctx, alice, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled list enabled")
	}
}
func TestOrganizationPagesRecheckRevocationRoleAndOwnership(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateOrganizations(t, s, 60)
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT id,$1,'viewer','active' FROM organizations WHERE id<>$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	first, err := s.MinePage(ctx, bob, "")
	if err != nil || !first.HasMore {
		t.Fatal("expected membership page")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organization_members SET state='revoked' WHERE organization_id=$1 AND owner_id=$2`, listID(55), bob); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organization_members SET role='administrator' WHERE organization_id=$1 AND owner_id=$2`, listID(56), bob); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, listID(54)); err != nil {
		t.Fatal(err)
	}
	next, err := s.MinePage(ctx, bob, *first.NextAfter)
	if err != nil || len(next.Items) != 9 {
		t.Fatalf("revoked page %+v %v", next, err)
	}
	for _, item := range next.Items {
		if item.OrganizationID == listID(55) {
			t.Fatal("revoked organization visible")
		}
		if item.OrganizationID == listID(56) && item.Role != "administrator" {
			t.Fatal("stale role")
		}
		if item.OrganizationID == listID(54) && item.Role != "owner" {
			t.Fatal("stale ownership")
		}
	}
	aliceFirst, err := s.MinePage(ctx, alice, "")
	if err != nil {
		t.Fatal(err)
	}
	aliceNext, err := s.MinePage(ctx, alice, *aliceFirst.NextAfter)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range aliceNext.Items {
		if item.OrganizationID == listID(54) {
			t.Fatal("former owner listed")
		}
	}
}
func TestOrganizationCursorQueryAndHTTPBounds(t *testing.T) {
	valid := organizationCursor{alice, time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC), org}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	cursor := base64.RawURLEncoding.EncodeToString(raw)
	if parsed, err := organizationAfter(alice, cursor); err != nil || !parsed.CreatedAt.Equal(valid.CreatedAt) {
		t.Fatal("timestamp precision")
	}
	for _, bad := range []string{"!", cursor + "\n", cursor + "=", strings.Repeat("a", 257), base64.RawURLEncoding.EncodeToString([]byte(`{"actorId":"` + alice + `","createdAt":"2026-01-01T00:00:00Z","id":"` + org + `","unknown":true}`))} {
		if _, err := organizationAfter(alice, bad); !errors.Is(err, ErrCursor) {
			t.Fatal("invalid cursor accepted")
		}
	}
	for _, bad := range []string{"after=", "before=x", "after=x&after=y", "after=%zz", strings.Repeat("x", 385)} {
		if _, err := organizationQuery(bad); !errors.Is(err, ErrCursor) {
			t.Fatal("invalid query accepted")
		}
	}
	s := fixture(t)
	mux := http.NewServeMux()
	s.Register(sessions{alice})(mux)
	for _, tc := range []struct {
		path, authorization string
		want                int
	}{{"/api/team/organizations/page", "", 200}, {"/api/team/organizations/page?after=", "", 400}, {"/api/team/organizations/page?unknown=x", "", 400}, {"/api/team/organizations/page", "Bearer private", 403}, {"/api/team/organizations", "", 200}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.authorization != "" {
			r.Header.Set("Authorization", tc.authorization)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("response %d %s", w.Code, w.Body.String())
		}
	}
}
