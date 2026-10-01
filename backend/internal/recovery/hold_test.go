package recovery

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestRecoveryFailsClosedAndHoldCannotTargetSource(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := Check(ctx, pool); err == nil {
		t.Fatal("missing ledger accepted")
	}
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Hold(ctx, pool, make([]byte, 32)); err == nil {
		t.Fatal("source database accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
		t.Fatalf("hold ignored: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now()`); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
