package auth

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestOwnerPausePreservesExistingSignin(t *testing.T) {
	f := setup(t, nil)
	existing := f.signIn(t, browser(t), "bob")
	store := OwnerAdmissionStore{Pool: f.pool}
	ctx := context.Background()
	if result, err := store.Set(ctx, "fixture-operator", "local_qualification", false); err != nil || result.Open {
		t.Fatal("owner pause failed", err)
	}
	client := browser(t)
	callback := f.begin(t, client, "alice")
	res := response(t, client, "GET", callback, "", "", "", 503)
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "New workspaces are paused. Existing owners can sign in.") {
		t.Fatal("missing pause guidance")
	}
	for _, cookie := range res.Cookies() {
		if cookie.Name == f.service.settings.cookie("session") {
			t.Fatal("paused signup issued a session")
		}
	}
	var owners, sessions int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM owners`).Scan(&owners); err != nil || owners != 1 {
		t.Fatal("paused signup created an owner")
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM management_sessions`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatal("paused signup changed existing sessions")
	}
	again := f.signIn(t, browser(t), "bob")
	if again.OwnerID != existing.OwnerID {
		t.Fatal("existing owner identity changed during pause")
	}
	if _, err = store.Set(ctx, "fixture-operator", "local_qualification", true); err != nil {
		t.Fatal(err)
	}
	created := f.signIn(t, browser(t), "alice")
	if created.OwnerID == existing.OwnerID {
		t.Fatal("new subject used existing owner")
	}
}

func TestOwnerAdmissionAtomicAuditAndMissingPolicy(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	store := OwnerAdmissionStore{Pool: f.pool}
	if _, err := store.Set(ctx, "fixture", "local_qualification", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(ctx, "fixture", "local_qualification", false); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM owner_admission_events`).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("repeat pause duplicated audit")
	}
	for _, tc := range []struct{ actor, reason string }{{"", "maintenance"}, {"fixture\n", "maintenance"}, {"fixture", "unsupported"}} {
		if _, err := store.Set(ctx, tc.actor, tc.reason, true); err == nil {
			t.Fatal("invalid policy change accepted")
		}
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE owner_admission_events ADD CONSTRAINT fixture_audit_failure CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(ctx, "fixture", "maintenance", true); err == nil {
		t.Fatal("audit failure accepted")
	}
	if state, err := store.Status(ctx); err != nil || state.Open {
		t.Fatal("failed audit reopened owners")
	}
	if _, err := f.pool.Exec(ctx, `ALTER TABLE owner_admission_events DROP CONSTRAINT fixture_audit_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(ctx, "fixture", "maintenance", true); err != nil {
		t.Fatal(err)
	}
	existing := f.signIn(t, browser(t), "bob")
	if _, err := f.pool.Exec(ctx, `DELETE FROM owner_admission`); err != nil {
		t.Fatal(err)
	}
	// Missing policy still permits only the existing owner path.
	if again := f.signIn(t, browser(t), "bob"); again.OwnerID != existing.OwnerID {
		t.Fatal("missing policy stopped existing sign-in")
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = ownerForSession(ctx, tx, f.issuerServer.URL, "alice", "Creator"); err == nil {
		t.Fatal("missing policy admitted a new owner")
	}
}

func TestOwnerClosureWaitsForAdmittedCreation(t *testing.T) {
	f := setup(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := ownerForSession(ctx, tx, "fixed-issuer", "fixed-subject", "Creator")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := (OwnerAdmissionStore{Pool: f.pool}).Set(ctx, "fixture", "maintenance", false)
		done <- err
	}()
	// Observe the actual database lock wait rather than a timing-only assertion.
	for {
		var blocked bool
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM owner_admission%FOR UPDATE%')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-done:
			t.Fatal("closure returned before admitted commit", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	tx, err = f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	same, err := ownerForSession(ctx, tx, "fixed-issuer", "fixed-subject", "Updated name")
	if err != nil || same != id {
		t.Fatal("existing owner stopped after closure", err)
	}
	if _, err = ownerForSession(ctx, tx, "other-issuer", "fixed-subject", "Creator"); !errors.Is(err, ErrOwnerAdmissionClosed) {
		t.Fatal("other issuer bypassed closure", err)
	}
}

func TestRecoveryHeldOwnerCreationAndPolicyFailClosed(t *testing.T) {
	pool := testdb.NewRecovery(t)
	f := setup(t, pool)
	ctx := context.Background()
	f.signIn(t, browser(t), "bob")
	var retainedName string
	if err := pool.QueryRow(ctx, `SELECT display_name FROM owners`).Scan(&retainedName); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := (OwnerAdmissionStore{Pool: pool}).Set(ctx, "fixture", "maintenance", false); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held policy changed", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = ownerForSession(ctx, tx, f.issuerServer.URL, "alice", "Creator"); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held owner created", err)
	}
	if _, err = ownerForSession(ctx, tx, f.issuerServer.URL, "bob", "Changed name"); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held existing-owner session admitted", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	client := browser(t)
	callback := f.begin(t, client, "bob")
	response(t, client, "GET", callback, "", "", "", 503)
	var sessions, owners int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM management_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("held callback minted a session")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM owners`).Scan(&owners); err != nil || owners != 1 {
		t.Fatal("held callback changed owner custody")
	}
	var afterName string
	if err = pool.QueryRow(ctx, `SELECT display_name FROM owners`).Scan(&afterName); err != nil || afterName != retainedName {
		t.Fatal("held callback changed owner metadata")
	}
}
