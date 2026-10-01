package exports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func migrationFixture(t *testing.T, preferences bool) (*Store, string) {
	t.Helper()
	s := exportStoreFixture(t)
	ctx := context.Background()
	op, err := s.Submit(ctx, ownerID, deploymentID, exportID)
	if err != nil {
		t.Fatal(err)
	}
	data := portableFixture()
	if preferences {
		data.Preferences = []byte(`{"namespace":"app.bsky","preferences":[{"$type":"app.bsky.actor.defs.savedFeedsPref","privateValue":"PRIVATE_PREFERENCES"}]}`)
	}
	worker := &Worker{Store: s, Agent: &exportAgent{data: data}}
	if err = worker.Work(ctx, exportJob(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	return s, op.ID
}

func TestMigrationInventoryVersionAndPrivacy(t *testing.T) {
	for _, preferences := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1", true: "v2"}[preferences], func(t *testing.T) {
			s, id := migrationFixture(t, preferences)
			ctx := context.Background()
			var before string
			const inventory = `SELECT md5(json_build_array((SELECT json_agg(e ORDER BY id) FROM export_operations e),(SELECT json_agg(e ORDER BY id) FROM export_events e),(SELECT json_agg(j ORDER BY id) FROM river_job j))::text)`
			if err := s.Pool.QueryRow(ctx, inventory).Scan(&before); err != nil {
				t.Fatal(err)
			}
			result, err := s.PreflightMigration(ctx, ownerID, id)
			if err != nil {
				t.Fatal(err)
			}
			expected := "not_captured_v1"
			if preferences {
				expected = "captured_app_bsky_only"
			}
			if result.Preferences != expected || result.CapturedRecordCount != 2 || result.RepositoryBytes != int64(len("fixture signed CAR")) || result.CanTransfer || result.CanSwitchIdentity || result.SignatureVerification != "not_rechecked" || len(result.RemainingGates) != 7 {
				t.Fatalf("incorrect inventory: %+v", result)
			}
			encoded, _ := json.Marshal(result)
			for _, private := range []string{"PRIVATE_PREFERENCES", "fixture signed CAR", "did:key:", s.Artifacts.directory, "preferences.json"} {
				if strings.Contains(string(encoded), private) {
					t.Fatal("private archive content disclosed")
				}
			}
			var after string
			if err = s.Pool.QueryRow(ctx, inventory).Scan(&after); err != nil || before != after {
				t.Fatal("inventory changed management work")
			}
			if result, err = s.PreflightMigration(ctx, foreignID, id); !errors.Is(err, ErrNotFound) || result.ExportID != "" {
				t.Fatal("foreign inventory disclosed")
			}
		})
	}
}

func TestExportDisclosureRejectsChangesDuringArchiveRead(t *testing.T) {
	for _, method := range []string{"inventory", "download"} {
		for _, change := range []string{"owner", "expiry", "checkpoint", "hold", "cancel_context"} {
			t.Run(method+"/"+change, func(t *testing.T) {
				s, id := migrationFixture(t, false)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				read := func(id, deployment, did string) ([]byte, Manifest, error) {
					plain, manifest, err := s.Artifacts.Read(id, deployment, did)
					if err != nil {
						return plain, manifest, err
					}
					switch change {
					case "owner":
						_, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=(SELECT organization_id FROM deployments WHERE id=$2)`, foreignID, deployment)
					case "expiry":
						_, err = s.Pool.Exec(ctx, `UPDATE export_operations SET captured_at=now()-interval '25 hours',expires_at=now()-interval '1 hour' WHERE id=$1`, id)
					case "checkpoint":
						_, err = s.Pool.Exec(ctx, `UPDATE export_operations SET archive_sha256=repeat('a',64) WHERE id=$1`, id)
					case "hold":
						err = recovery.Hold(ctx, s.Pool, make([]byte, 32))
					case "cancel_context":
						cancel()
					}
					if err != nil {
						t.Fatal("Authority-change fixture failed:", err)
					}
					return plain, manifest, nil
				}
				if method == "inventory" {
					result, err := s.preflightMigration(ctx, ownerID, id, read)
					if err == nil || result.ExportID != "" {
						t.Fatal("stale authority/checkpoint published")
					}
				} else {
					plain, err := s.download(ctx, ownerID, id, read)
					if err == nil || plain != nil {
						t.Fatal("stale authority disclosed archive bytes")
					}
				}
			})
		}
	}
}

func TestMigrationRejectsTamperedOrUnavailableArchive(t *testing.T) {
	s, id := migrationFixture(t, false)
	ctx := context.Background()
	path := filepath.Join(s.Artifacts.directory, id+".sealed")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-1] ^= 1
	if err = os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := s.PreflightMigration(ctx, ownerID, id); !errors.Is(err, ErrUnavailable) || result.ExportID != "" {
		t.Fatal("tampered archive disclosed")
	}
	s.Artifacts = nil
	if _, err := s.PreflightMigration(ctx, ownerID, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing custody accepted")
	}
}

type migrationSession struct{ owner string }

func (s migrationSession) Authorize(w http.ResponseWriter, r *http.Request, mutation bool) (auth.Session, bool) {
	if mutation {
		panic("read route asked for mutation authority")
	}
	return auth.Session{OwnerID: s.owner}, true
}

func TestMigrationHTTPBoundaries(t *testing.T) {
	s, id := migrationFixture(t, true)
	for _, tc := range []struct {
		name, owner   string
		local, bearer bool
		status        int
	}{
		{"owned", ownerID, true, false, 200},
		{"foreign", foreignID, true, false, 404},
		{"production", ownerID, false, false, 503},
		{"cli", ownerID, true, true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			s.RegisterMigration(migrationSession{tc.owner}, tc.local)(mux)
			req := httptest.NewRequest("GET", "/api/exports/"+id+"/migration-preflight", nil)
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer private")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || strings.Contains(w.Body.String(), "PRIVATE_PREFERENCES") {
				t.Fatalf("boundary failed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
