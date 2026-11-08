package exports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExportWithoutVerifiedAccountIsADefiniteRefusal(t *testing.T) {
	s := exportStoreFixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE enrollment_operations SET state='account_created' WHERE deployment_id=$1`, deploymentID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, ownerID, deploymentID, exportID); !errors.Is(err, ErrNoVerifiedAccount) {
		t.Fatal("export without a verified account was not a definite refusal", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM export_operations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("refused export created an operation", count, err)
	}
	w := httptest.NewRecorder()
	failure(w, ErrNoVerifiedAccount)
	var body map[string]string
	if w.Code != 409 || json.Unmarshal(w.Body.Bytes(), &body) != nil || !strings.Contains(body["error"], "verify the first account") {
		t.Fatalf("refusal response %d %s", w.Code, w.Body.String())
	}
}
