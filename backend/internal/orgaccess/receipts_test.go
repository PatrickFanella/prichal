package orgaccess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func populateReceipts(t *testing.T, s *Store, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'http://localhost:5556','fixture-'||n,'PRIVATE_EMAIL' FROM generate_series(1,$1::int) n`, n); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT $1,('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'viewer','revoked' FROM generate_series(1,$2::int) n`, org, n); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_invitations(id,organization_id,inviter_id,target_id,target_issuer,target_subject,role,idempotency_key,state,expires_at,created_at) SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,$1,$2,('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'PRIVATE_ISSUER','PRIVATE_SUBJECT','viewer',('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,'pending','2020-01-01T00:00:00Z','2026-01-01T00:00:00.123456Z' FROM generate_series(1,$3::int) n`, org, alice, n); err != nil {
		t.Fatal(err)
	}
}
func TestOwnerReceiptTraversalAndCurrentStates(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateReceipts(t, s, 117)
	before := count(t, s, "organization_access_events")
	for _, kind := range []string{"members", "invitations"} {
		t.Run(kind, func(t *testing.T) {
			cursor := ""
			seen := map[string]bool{}
			pages := 0
			for {
				var hasMore bool
				var next *string
				var ids []string
				var raw []byte
				var err error
				if kind == "members" {
					page, e := s.MembersPage(ctx, alice, org, cursor)
					err = e
					hasMore = page.HasMore
					next = page.NextAfter
					raw, e = json.Marshal(page)
					if e != nil {
						t.Fatal(e)
					}
					for _, v := range page.Items {
						ids = append(ids, v.IdentityID)
						if v.State != "revoked" {
							t.Fatal("wrong current membership")
						}
					}
				} else {
					page, e := s.InvitationReceiptsPage(ctx, alice, org, cursor)
					err = e
					hasMore = page.HasMore
					next = page.NextAfter
					raw, e = json.Marshal(page)
					if e != nil {
						t.Fatal(e)
					}
					for _, v := range page.Items {
						ids = append(ids, v.ID)
						if v.State != "expired" {
							t.Fatal("expiry not projected")
						}
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				pages++
				if len(ids) > 50 || strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "idempotency") {
					t.Fatal("unsafe/unbounded projection")
				}
				for _, id := range ids {
					if seen[id] {
						t.Fatal("duplicate")
					}
					seen[id] = true
				}
				if !hasMore {
					if next != nil {
						t.Fatal("terminal cursor")
					}
					break
				}
				if next == nil || len(*next) > 384 {
					t.Fatal("missing continuation")
				}
				cursor = *next
			}
			if pages != 3 || len(seen) != 117 {
				t.Fatalf("incomplete %d/%d", pages, len(seen))
			}
		})
	}
	var pending int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM organization_invitations WHERE state='pending'`).Scan(&pending); err != nil || pending != 117 || count(t, s, "organization_access_events") != before {
		t.Fatal("read mutated expiry/audit")
	}
	first, err := s.MembersPage(ctx, alice, org, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organization_members SET state='active',role='administrator',updated_at=now() WHERE owner_id=$1`, listID(51)); err != nil {
		t.Fatal(err)
	}
	next, err := s.MembersPage(ctx, alice, org, *first.NextAfter)
	if err != nil || next.Items[0].IdentityID != listID(51) || next.Items[0].State != "active" || next.Items[0].Role != "administrator" {
		t.Fatal("current state/order lost")
	}
}
func TestOwnerReceiptsScopeAndHTTP(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	populateReceipts(t, s, 51)
	member, err := s.MembersPage(ctx, alice, org, "")
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := s.InvitationReceiptsPage(ctx, alice, org, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind   string
		read   func(context.Context, string, string, string) (any, error)
		cursor string
	}{
		{"members/page", func(c context.Context, a, o, k string) (any, error) { return s.MembersPage(c, a, o, k) }, *member.NextAfter},
		{"invitations/page", func(c context.Context, a, o, k string) (any, error) { return s.InvitationReceiptsPage(c, a, o, k) }, *invitation.NextAfter},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			for _, actor := range []string{bob, stranger} {
				if _, err := tc.read(ctx, actor, org, ""); !errors.Is(err, ErrNotFound) {
					t.Fatal("foreign read")
				}
				if _, err := tc.read(ctx, actor, org, tc.cursor); !errors.Is(err, ErrCursor) {
					t.Fatal("cross-identity")
				}
			}
			if _, err := tc.read(ctx, alice, stranger, tc.cursor); !errors.Is(err, ErrCursor) {
				t.Fatal("cross-organization")
			}
			for _, bad := range []string{"!", tc.cursor + "=", tc.cursor + "\n", strings.Repeat("a", 385)} {
				if _, err := tc.read(ctx, alice, org, bad); !errors.Is(err, ErrCursor) {
					t.Fatal("bad cursor")
				}
			}
			mux := http.NewServeMux()
			s.Register(sessions{alice})(mux)
			path := "/api/team/organizations/" + org + "/" + tc.kind
			for _, q := range []string{"after=", "after=x&after=y", "other=x", "after=%zz"} {
				r := httptest.NewRequest("GET", path, nil)
				r.URL.RawQuery = q
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("query/cache boundary")
				}
			}
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Authorization", "Bearer private")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatal("CLI allowed")
			}
		})
	}
	if _, err = s.MembersPage(ctx, alice, org, *invitation.NextAfter); !errors.Is(err, ErrCursor) {
		t.Fatal("receipt/member cursor reuse")
	}
	if _, err = s.InvitationReceiptsPage(ctx, alice, org, *member.NextAfter); !errors.Is(err, ErrCursor) {
		t.Fatal("member/receipt cursor reuse")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MembersPage(ctx, alice, org, *member.NextAfter); !errors.Is(err, ErrNotFound) {
		t.Fatal("former owner member read")
	}
	if _, err = s.InvitationReceiptsPage(ctx, alice, org, *invitation.NextAfter); !errors.Is(err, ErrNotFound) {
		t.Fatal("former owner receipt read")
	}
	if _, err = s.MembersPage(ctx, bob, org, ""); err != nil {
		t.Fatal(err)
	}
	s.Enabled = false
	if _, err = s.InvitationReceiptsPage(ctx, bob, org, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled read")
	}
}

func TestCurrentMembersCannotReadOwnerReceipts(t *testing.T) {
	for _, role := range []string{"viewer", "administrator"} {
		t.Run(role, func(t *testing.T) {
			s := fixture(t)
			accept(t, s, role)
			if _, err := s.MembersPage(context.Background(), bob, org, ""); !errors.Is(err, ErrNotFound) {
				t.Fatal("member read owner membership")
			}
			if _, err := s.InvitationReceiptsPage(context.Background(), bob, org, ""); !errors.Is(err, ErrNotFound) {
				t.Fatal("member read owner receipts")
			}
		})
	}
}
