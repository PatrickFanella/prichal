package orgaccess

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAuditPagesPreserveLargeIDsAndCurrentOwnership(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_access_events(id,organization_id,actor_id,action,created_at) OVERRIDING SYSTEM VALUE SELECT 9007199254740992+n,$1,$2,'rename','2026-10-01T00:00:00Z'::timestamptz FROM generate_series(1,157) n`, org, alice); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := s.Audit(ctx, alice, org, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(page.Items) > 50 {
			t.Fatal("unbounded page")
		}
		for _, event := range page.Items {
			if seen[event.ID] {
				t.Fatal("repeated event")
			}
			seen[event.ID] = true
			id, err := strconv.ParseInt(event.ID, 10, 64)
			if err != nil || id <= 9007199254740992 {
				t.Fatal("large ID lost precision")
			}
		}
		if !page.HasMore {
			if page.NextBefore != nil {
				t.Fatal("terminal cursor")
			}
			break
		}
		if page.NextBefore == nil {
			t.Fatal("missing cursor")
		}
		cursor = *page.NextBefore
	}
	if pages != 4 || len(seen) != 157 || count(t, s, "organization_access_events") != 157 {
		t.Fatalf("traversal: %d pages %d events", pages, len(seen))
	}
	first, err := s.Audit(ctx, alice, org, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Audit(ctx, alice, stranger, *first.NextBefore); !errors.Is(err, ErrCursor) {
		t.Fatalf("cross-org cursor: %v", err)
	}
	// An accepted administrator may rename but cannot inspect owner audit history.
	accept(t, s, "administrator")
	if _, err = s.Audit(ctx, bob, org, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("administrator audit access: %v", err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Audit(ctx, alice, org, *first.NextBefore); !errors.Is(err, ErrNotFound) {
		t.Fatalf("former owner paging: %v", err)
	}
	if _, err = s.Audit(ctx, bob, org, *first.NextBefore); err != nil {
		t.Fatalf("current owner paging: %v", err)
	}
	s.Enabled = false
	if _, err = s.Audit(ctx, bob, org, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled audit route")
	}
}
func TestAuditCursorAndQueryBounds(t *testing.T) {
	for _, raw := range []string{org + ":0", org + ":-1", org + ":01", org + ":+1", org + ":9223372036854775808", org + ":1:2", stranger + ":1"} {
		t.Run(raw, func(t *testing.T) {
			cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
			if _, err := auditBefore(org, cursor); !errors.Is(err, ErrCursor) {
				t.Fatal("invalid cursor accepted")
			}
		})
	}
	valid := auditCursor(org, "9223372036854775807")
	if id, err := auditBefore(org, valid); err != nil || id != 9223372036854775807 {
		t.Fatal("valid bounded cursor rejected")
	}
	for _, cursor := range []string{"!", valid + "=", valid + "\n", strings.Repeat("a", 129)} {
		if _, err := auditBefore(org, cursor); !errors.Is(err, ErrCursor) {
			t.Fatal("noncanonical cursor accepted")
		}
	}
	for _, raw := range []string{"before=", "before=a&before=b", "unknown=x", "before=%zz", "before=a;other=x", strings.Repeat("x", 257)} {
		if _, err := auditQuery(raw); !errors.Is(err, ErrCursor) {
			t.Fatalf("invalid query accepted: %s", raw)
		}
	}
}
func TestAuditHTTPIsReadOnlyScopedAndStrict(t *testing.T) {
	s := fixture(t)
	mux := http.NewServeMux()
	s.Register(sessions{alice})(mux)
	for _, tc := range []struct {
		query string
		want  int
	}{{"", 200}, {"before=", 400}, {"before=bad", 400}, {"unknown=x", 400}, {"before=x&before=y", 400}, {"before=%zz", 400}} {
		r := httptest.NewRequest("GET", "/api/team/organizations/"+org+"/audit", nil)
		r.URL.RawQuery = tc.query
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("audit response: %d %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/api/team/organizations/"+org+"/audit", nil)
	r.Header.Set("Authorization", "Bearer private")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CLI audit scope")
	}
	if count(t, s, "organization_access_events") != 0 {
		t.Fatal("reading added an audit")
	}
}

func TestAuditOrderAcrossIDDigitBoundaries(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_access_events(organization_id,actor_id,action) SELECT $1,$2,'rename' FROM generate_series(1,117)`, org, alice); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	expected := int64(117)
	for {
		page, err := s.Audit(ctx, alice, org, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			got, err := strconv.ParseInt(item.ID, 10, 64)
			if err != nil || got != expected {
				t.Fatalf("numeric ordering: got %s want %d", item.ID, expected)
			}
			expected--
		}
		if !page.HasMore {
			break
		}
		cursor = *page.NextBefore
	}
	if expected != 0 {
		t.Fatal("numeric pagination lost events")
	}
}
