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
)

func populateOffers(t *testing.T, s *Store, kind string, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organizations(id,owner_id,name,created_at) SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,$1,'offer-org-'||n,'2026-01-01T00:00:00.123456Z' FROM generate_series(1,$2::int) n`, alice, n); err != nil {
		t.Fatal(err)
	}
	if kind == "invitations" {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_invitations(id,organization_id,inviter_id,target_id,target_issuer,target_subject,role,idempotency_key,expires_at,created_at) SELECT id,id,$1,$2,'http://localhost:5556',$2::uuid::text,'viewer',id,now()+interval '1 hour','2026-01-01T00:00:00.123456Z' FROM organizations WHERE id<>$3`, alice, bob, org); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT id,$1,'administrator','active' FROM organizations WHERE id<>$2`, bob, org); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_transfers(id,organization_id,previous_owner_id,next_owner_id,idempotency_key,expires_at,created_at) SELECT id,id,$1,$2,id,now()+interval '1 hour','2026-01-01T00:00:00.123456Z' FROM organizations WHERE id<>$3`, alice, bob, org); err != nil {
			t.Fatal(err)
		}
	}
}
func TestIncomingOfferTraversalAndCurrentEligibility(t *testing.T) {
	for _, kind := range []string{"invitations", "transfers"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			populateOffers(t, s, kind, 117)
			before := count(t, s, "organization_access_events")
			cursor := ""
			seen := map[string]bool{}
			pages := 0
			for {
				page, err := s.incomingPage(ctx, bob, kind, cursor)
				if err != nil {
					t.Fatal(err)
				}
				pages++
				raw, err := json.Marshal(page)
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Items) > 50 || strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "issuer") || strings.Contains(string(raw), "subject") || strings.Contains(string(raw), "idempotency") {
					t.Fatal("unsafe/unbounded projection")
				}
				for _, item := range page.Items {
					if seen[item.ID] {
						t.Fatal("duplicate")
					}
					seen[item.ID] = true
					if kind == "invitations" && (item.Role == nil || *item.Role != "viewer") || kind == "transfers" && item.Role != nil {
						t.Fatal("wrong offer kind")
					}
				}
				if !page.HasMore {
					if page.NextAfter != nil {
						t.Fatal("terminal cursor")
					}
					break
				}
				if page.NextAfter == nil || len(*page.NextAfter) > 320 {
					t.Fatal("missing cursor")
				}
				cursor = *page.NextAfter
			}
			if pages != 3 || len(seen) != 117 || count(t, s, "organization_access_events") != before {
				t.Fatal("lost/mutated offers")
			}
			legacy, err := s.Incoming(ctx, bob)
			if err != nil {
				t.Fatal(err)
			}
			var lists map[string][]IncomingOffer
			if err = json.Unmarshal(legacy, &lists); err != nil || len(lists[kind]) != 30 {
				t.Fatal("legacy compatibility")
			}
			first, err := s.incomingPage(ctx, bob, kind, "")
			if err != nil || first.Items[49].ID != listID(50) {
				t.Fatal("timestamp UUID ordering")
			}
			if kind == "invitations" {
				if _, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET expires_at=now()-interval '1 second' WHERE id=$1`, listID(51)); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET state='revoked' WHERE id=$1`, listID(52)); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET target_subject='changed' WHERE id=$1`, listID(53)); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, stranger, listID(54)); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET state='accepted' WHERE id=$1`, listID(55)); err != nil {
					t.Fatal(err)
				}
			} else {
				queries := []string{`UPDATE organization_transfers SET expires_at=now()-interval '1 second' WHERE id=$1`, `UPDATE organization_transfers SET state='revoked' WHERE id=$1`, `UPDATE organization_members SET role='viewer' WHERE organization_id=$1`, `UPDATE organizations SET owner_id='33333333-3333-4333-8333-333333333333' WHERE id=$1`, `UPDATE organization_members SET state='revoked' WHERE organization_id=$1`}
				for i, q := range queries {
					if _, err = s.Pool.Exec(ctx, q, listID(51+i)); err != nil {
						t.Fatal(err)
					}
				}
			}
			next, err := s.incomingPage(ctx, bob, kind, *first.NextAfter)
			if err != nil || len(next.Items) != 50 || next.Items[0].ID != listID(56) {
				t.Fatal("stale offer eligibility")
			}
			foreign, err := s.incomingPage(ctx, stranger, kind, "")
			if err != nil || len(foreign.Items) != 0 {
				t.Fatal("foreign recipient offers")
			}
		})
	}
}
func TestIncomingCursorAndHTTPBoundaries(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateOffers(t, s, "invitations", 51)
	first, err := s.IncomingInvitationsPage(ctx, bob, "")
	if err != nil {
		t.Fatal(err)
	}
	cursor := *first.NextAfter
	if _, err = s.IncomingInvitationsPage(ctx, alice, cursor); !errors.Is(err, ErrCursor) {
		t.Fatal("cross-identity")
	}
	if _, err = s.IncomingTransfersPage(ctx, bob, cursor); !errors.Is(err, ErrCursor) {
		t.Fatal("cross-list")
	}
	parsed, err := incomingAfter(bob, "invitations", cursor)
	if err != nil || parsed.CreatedAt.Nanosecond() != 123456000 {
		t.Fatal("timestamp precision")
	}
	for _, bad := range []string{"!", cursor + "\n", cursor + "=", strings.Repeat("a", 321), base64.RawURLEncoding.EncodeToString([]byte(`{"actorId":"` + bob + `","kind":"invitations","createdAt":"2026-01-01T00:00:00Z","id":"` + key + `","extra":true}`))} {
		if _, err = s.IncomingInvitationsPage(ctx, bob, bad); !errors.Is(err, ErrCursor) {
			t.Fatal("invalid cursor")
		}
	}
	mux := http.NewServeMux()
	s.Register(sessions{bob})(mux)
	for _, kind := range []string{"invitations", "transfers"} {
		t.Run(kind, func(t *testing.T) {
			path := "/api/team/incoming/" + kind + "/page"
			for _, tc := range []struct {
				q, header string
				want      int
			}{{"", "", 200}, {"after=", "", 400}, {"after=x&after=y", "", 400}, {"other=x", "", 400}, {"after=%zz", "", 400}, {"", "Bearer private", 403}} {
				r := httptest.NewRequest("GET", path, nil)
				r.URL.RawQuery = tc.q
				if tc.header != "" {
					r.Header.Set("Authorization", tc.header)
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("HTTP boundary")
				}
			}
		})
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE owners SET subject='changed' WHERE id=$1`, bob); err != nil {
		t.Fatal(err)
	}
	changed, err := s.IncomingInvitationsPage(ctx, bob, "")
	if err != nil || len(changed.Items) != 0 {
		t.Fatal("changed verified identity")
	}
	s.Enabled = false
	if _, err = s.IncomingTransfersPage(ctx, bob, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled offers")
	}
}
