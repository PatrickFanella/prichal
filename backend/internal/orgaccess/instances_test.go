package orgaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func populateInstances(t *testing.T, s *Store, n int) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,message,endpoint_url,created_at) SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,$1,'instance-'||n,'local','key-'||n,'test','ready','fixture-'||n||'.localhost','PRIVATE_MESSAGE','https://private.invalid','2026-01-01T00:00:00.123456Z'::timestamptz FROM generate_series(1,$2::int) n`, org, n); err != nil {
		t.Fatal(err)
	}
}
func TestInstancePagesPreserveTiesAndSafeViewerProjection(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateInstances(t, s, 117)
	accept(t, s, "viewer")
	before := count(t, s, "organization_access_events")
	cursor := ""
	seen := map[string]bool{}
	pages := 0
	for {
		page, err := s.InstancesPage(ctx, bob, org, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(page.Items) > 50 {
			t.Fatal("unbounded page")
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate instance")
			}
			seen[item.ID] = true
			if item.Kind != "test" || item.State != "ready" {
				t.Fatal("invalid status")
			}
		}
		raw, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "private.invalid") || strings.Contains(string(raw), "endpoint") {
			t.Fatal("private projection")
		}
		if !page.HasMore {
			if page.NextAfter != nil {
				t.Fatal("terminal cursor")
			}
			break
		}
		if page.NextAfter == nil || len(*page.NextAfter) > 384 {
			t.Fatal("invalid cursor bound")
		}
		cursor = *page.NextAfter
	}
	if pages != 3 || len(seen) != 117 || count(t, s, "organization_access_events") != before {
		t.Fatalf("traversal %d %d", pages, len(seen))
	}
	first, err := s.InstancesPage(ctx, bob, org, "")
	if err != nil || first.Items[49].ID != listID(50) {
		t.Fatal("UUID tie ordering")
	}
	if _, err = s.InstancesPage(ctx, alice, org, *first.NextAfter); !errors.Is(err, ErrCursor) {
		t.Fatal("cross-identity cursor")
	}
	if _, err = s.InstancesPage(ctx, bob, stranger, *first.NextAfter); !errors.Is(err, ErrCursor) {
		t.Fatal("cross-organization cursor")
	}
	if _, err = s.InstancesPage(ctx, stranger, org, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign shared status")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organization_members SET state='revoked' WHERE organization_id=$1 AND owner_id=$2`, org, bob); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstancesPage(ctx, bob, org, *first.NextAfter); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked viewer paged")
	}
	ownerPage, err := s.InstancesPage(ctx, alice, org, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstancesPage(ctx, alice, org, *ownerPage.NextAfter); !errors.Is(err, ErrNotFound) {
		t.Fatal("former owner paged")
	}
	if _, err = s.InstancesPage(ctx, bob, org, ""); err != nil {
		t.Fatal("current owner denied")
	}
}
func TestScopedInstanceCursorAndHTTPBounds(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)
	cursor, err := scopedNext(alice, org, "instances", key, at)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := scopedAfter(alice, org, "instances", cursor)
	if err != nil || !parsed.CreatedAt.Equal(at) {
		t.Fatal("timestamp precision")
	}
	wrongKind, err := scopedNext(alice, org, "invitations", key, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"!", cursor + "\n", cursor + "=", strings.Repeat("a", 385), wrongKind, base64.RawURLEncoding.EncodeToString([]byte(`{"actorId":"` + alice + `","organizationId":"` + org + `","kind":"instances","createdAt":"2026-01-01T00:00:00Z","id":"` + key + `","unknown":true}`))} {
		if _, err := scopedAfter(alice, org, "instances", bad); !errors.Is(err, ErrCursor) {
			t.Fatal("invalid scoped cursor accepted")
		}
	}
	s := fixture(t)
	mux := http.NewServeMux()
	s.Register(sessions{alice})(mux)
	path := "/api/team/organizations/" + org + "/instances/page"
	for _, tc := range []struct {
		query, authorization string
		want                 int
	}{{"", "", 200}, {"after=", "", 400}, {"after=x&after=y", "", 400}, {"after=%zz", "", 400}, {"unknown=x", "", 400}, {"", "Bearer private", 403}} {
		r := httptest.NewRequest("GET", path, nil)
		r.URL.RawQuery = tc.query
		if tc.authorization != "" {
			r.Header.Set("Authorization", tc.authorization)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("response %d %s", w.Code, w.Body.String())
		}
	}
	s.Enabled = false
	if _, err = s.InstancesPage(context.Background(), alice, org, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled instance list")
	}
}
