package support

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

func TestReleaseProjectionIsScopedAndExpires(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	var node string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state) VALUES('local','https://localhost:9091',1,1,'active') RETURNING id`).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, instance, node); err != nil {
		t.Fatal(err)
	}
	imageID := "sha256:" + strings.Repeat("a", 64)
	if _, err := s.Pool.Exec(ctx, `INSERT INTO instance_runtime_releases(deployment_id,node_id,image_ref,image_id) VALUES($1,$2,$3,$4)`, instance, node, nodeagent.PDSImage, imageID); err != nil {
		t.Fatal(err)
	}
	store := &deployment.Store{Pool: s.Pool}
	op, err := store.Get(ctx, bob, instance)
	if err != nil || op.Release.State != "observed" || op.Release.Runtime == nil || op.Release.Runtime.ImageID != imageID {
		t.Fatalf("owner projection: %+v %v", op.Release, err)
	}
	if _, err = store.Get(ctx, alice, instance); !errors.Is(err, deployment.ErrNotFound) {
		t.Fatal("foreign owner read release")
	}
	view, err := s.Inspect(ctx, alice, "inspect", instance)
	if err != nil || !strings.Contains(string(view), nodeagent.PDSImage) || strings.Contains(string(view), node) {
		t.Fatalf("operator projection %s %v", view, err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE instance_runtime_releases SET checked_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	op, err = store.Get(ctx, bob, instance)
	if err != nil || op.Release.State != "stale" {
		t.Fatal("stale release claimed current")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE instance_runtime_releases SET image_ref=NULL,image_id=NULL,checked_at=now()`); err != nil {
		t.Fatal(err)
	}
	op, err = store.Get(ctx, bob, instance)
	if err != nil || op.Release.State != "unknown" || op.Release.Runtime != nil || op.Release.CheckedAt == nil {
		t.Fatal("failed inspection retained current release")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE instance_runtime_releases SET image_ref=$1,image_id=$2`, nodeagent.PDSImage, imageID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `DELETE FROM instance_placements`); err != nil {
		t.Fatal(err)
	}
	op, err = store.Get(ctx, bob, instance)
	if err != nil || op.Release.State != "unknown" || op.Release.CheckedAt != nil {
		t.Fatal("removed placement retained release")
	}
}

func TestUpstreamRouteRequiresOperatorBrowser(t *testing.T) {
	s, actions := fixture(t)
	// Denied callers must not start an outbound lookup.
	for _, tc := range []struct {
		owner, authorization string
		want                 int
	}{{bob, "", 404}, {otherIssuer, "", 404}, {alice, "Bearer fixture", 403}} {
		mux := http.NewServeMux()
		s.Register(sessionFixture{tc.owner}, actions)(mux)
		r := httptest.NewRequest("GET", "/api/operator/pds-upstream-release", nil)
		if tc.authorization != "" {
			r.Header.Set("Authorization", tc.authorization)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("authority %d", w.Code)
		}
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["tag"] != nil {
			t.Fatal("denied release leaked")
		}
	}
}
