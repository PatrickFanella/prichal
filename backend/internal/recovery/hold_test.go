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

func TestRestoredHoldRevokesCopiedAuthenticationAtomically(t *testing.T) {
	pool := testdb.NewRecovery(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','held-auth','Held auth') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'disposable-csrf',now()+interval '1 hour')`, make([]byte, 32), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO login_attempts(state_hash,binding_hash,nonce,pkce_verifier,expires_at) VALUES($1,$1,'disposable-nonce','disposable-pkce',now()+interval '10 minutes')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO cli_credentials(token_hash,owner_id) VALUES($1,$2)`, make([]byte, 32), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cli_device_grants(device_hash,user_hash,requester_hash,owner_id,state) VALUES($1,$1,$1,$2,'approved')`, make([]byte, 32), owner); err != nil {
		t.Fatal(err)
	}

	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	var sessions, attempts, owners, credentials, grants int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM management_sessions),(SELECT count(*) FROM login_attempts),(SELECT count(*) FROM owners),(SELECT count(*) FROM cli_credentials),(SELECT count(*) FROM cli_device_grants)`).Scan(&sessions, &attempts, &owners, &credentials, &grants); err != nil || sessions != 0 || attempts != 0 || owners != 1 || credentials != 0 || grants != 0 {
		t.Fatal("hold did not revoke transient authentication while preserving ownership")
	}
	if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
		t.Fatal("authentication revocation released recovery hold")
	}
}
