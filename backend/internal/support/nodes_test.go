package support

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeInventoryAuthorityFreshnessAndReservations(t *testing.T) {
	s, actions := fixture(t)
	ctx := context.Background()
	var node string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at,measured_cpus,measured_memory_bytes,last_verified_at) VALUES('local','https://127.0.0.1:9444',1,1,'active',$1,now(),3,4294967296,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, instance, node); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		view, err := s.Nodes(ctx, alice)
		if err != nil || strings.Contains(string(view), "PRIVATE") || strings.Contains(string(view), "certificate") || strings.Contains(string(view), "endpoint") {
			t.Fatal("private node data escaped", err)
		}
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		if json.Unmarshal(view, &payload) != nil || len(payload.Items) != 1 {
			t.Fatal("inventory missing")
		}
		return payload.Items[0]
	}
	view := read()
	if view["fresh"] != true || view["reserved"] != float64(1) || view["available"] != float64(0) {
		t.Fatal("reserved capacity exposed as free")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE hosting_nodes SET last_verified_at=now()-interval '2 minutes' WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	view = read()
	if view["fresh"] != false || view["available"] != float64(0) {
		t.Fatal("stale node admitted")
	}
	if _, err := s.Nodes(ctx, bob); err != ErrNotFound {
		t.Fatal("ordinary owner read global inventory")
	}
	mux := http.NewServeMux()
	s.Register(sessionFixture{owner: alice}, actions)(mux)
	request := httptest.NewRequest("GET", "/api/operator/nodes", nil)
	request.Header.Set("Authorization", "Bearer private-fixture")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("CLI credential accepted for inventory")
	}
}
