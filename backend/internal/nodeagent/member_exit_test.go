package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const exitInstance = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const exitTicket = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
const exitDID = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"

func exitDocker(t *testing.T) *Docker {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func exitMember() memberEntry {
	return memberEntry{Ticket: exitTicket, State: "accepted", DID: exitDID, Handle: "member.test"}
}

func TestMemberExitWindowPersistsAndReconcilesUncertainNativeCompletion(t *testing.T) {
	d := exitDocker(t)
	member := exitMember()
	now := time.Now().UTC().Truncate(time.Millisecond)
	calls, writes := 0, 0
	inactive, lose := false, true
	call := func(action, did string) (nativeMemberExit, error) {
		calls++
		if did != exitDID {
			t.Fatal("caller changed native target")
		}
		if action == "deactivate" {
			state, err := d.readMemberExits(exitInstance)
			if err != nil || len(state.Entries) != 1 || state.Entries[0].DeactivateAfter != now.Add(MemberMigrationWindow).UnixMilli() {
				t.Fatal("native mutation preceded durable deadline")
			}
			if !inactive {
				writes++
				inactive = true
			}
			if lose {
				lose = false
				return nativeMemberExit{}, errors.New("uncertain native response")
			}
		} else if action != "native_status" {
			t.Fatal("arbitrary native action")
		}
		return nativeMemberExit{DID: did, Handle: "member.test", Deactivated: inactive}, nil
	}
	first, err := d.reconcileMemberExit(exitInstance, member, now, call)
	if err != nil || first.State != "migration_window" || first.DeactivateAfter-first.RequestedAt != MemberMigrationWindow.Milliseconds() || calls != 1 || writes != 0 {
		t.Fatalf("initial window: %+v %v", first, err)
	}
	info, _ := os.Stat(filepath.Join(d.directory, exitInstance+"-member-exits.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("exit state not private")
	}
	restarted, err := NewDocker("/unused.sock", d.directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, before := range []time.Time{now.Add(-time.Hour), now.Add(MemberMigrationWindow - time.Millisecond)} {
		result, err := restarted.reconcileMemberExit(exitInstance, member, before, call)
		if err != nil || result != first || calls != 1 {
			t.Fatal("retry changed deadline or touched native account early")
		}
	}
	failed, err := restarted.reconcileMemberExit(exitInstance, member, now.Add(MemberMigrationWindow), call)
	if err == nil || failed.State != "" || writes != 1 {
		t.Fatal("uncertain native response claimed completion")
	}
	state, _ := restarted.readMemberExits(exitInstance)
	if state.Entries[0] != first {
		t.Fatal("uncertain completion lost original window")
	}
	result, err := restarted.reconcileMemberExit(exitInstance, member, now.Add(MemberMigrationWindow+time.Second), call)
	if err != nil || result.State != "deactivated" || result.DeactivateAfter != first.DeactivateAfter || writes != 1 {
		t.Fatalf("completion replay: %+v %v", result, err)
	}
	before := calls
	if _, err = restarted.reconcileMemberExit(exitInstance, member, now.Add(MemberMigrationWindow+time.Hour), call); err != nil || calls != before {
		t.Fatal("completed retry repeated native execution")
	}
}

func TestMemberExitRejectsChangedBindingCorruptStateAndUnsafeFiles(t *testing.T) {
	d := exitDocker(t)
	member := exitMember()
	now := time.Now()
	calls := 0
	call := func(action, did string) (nativeMemberExit, error) {
		calls++
		return nativeMemberExit{DID: did, Handle: "member.test"}, nil
	}
	if _, err := d.reconcileMemberExit(exitInstance, member, now, call); err != nil {
		t.Fatal(err)
	}
	before := calls
	changed := member
	changed.DID = "did:plc:cccccccccccccccccccccccc"
	if _, err := d.reconcileMemberExit(exitInstance, changed, now, call); err == nil {
		t.Fatal("changed identity accepted")
	}
	changed = member
	changed.Ticket = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	if _, err := d.reconcileMemberExit(exitInstance, changed, now, call); err == nil {
		t.Fatal("identity moved to another ticket")
	}
	if calls != before {
		t.Fatal("foreign binding reached native helper")
	}
	path := filepath.Join(d.directory, exitInstance+"-member-exits.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"null", "{}", "[]", string(original) + "{}", strings.Replace(string(original), "\"version\":1", "\"version\":2", 1), strings.Replace(string(original), "\"entries\":[", "\"unexpected\":true,\"entries\":[", 1)} {
		if err = os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = d.reconcileMemberExit(exitInstance, member, now, call); err == nil {
			t.Fatal("corrupt exit state silently reset")
		}
	}
	if err = os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = d.reconcileMemberExit(exitInstance, member, now, call); err == nil {
		t.Fatal("public exit state accepted")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(d.directory, "retained")
	if err = os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err = d.reconcileMemberExit(exitInstance, member, now, call); err == nil {
		t.Fatal("symlink exit state accepted")
	}
	if calls != before {
		t.Fatal("unsafe state reached native helper")
	}
}

func TestMemberExitRequiresConfirmedMemberAndRejectsShortenedDeadline(t *testing.T) {
	d := exitDocker(t)
	member := exitMember()
	now := time.Now()
	foreign := func(string, string) (nativeMemberExit, error) {
		return nativeMemberExit{DID: "did:plc:cccccccccccccccccccccccc", Handle: "foreign.test"}, nil
	}
	if _, err := d.reconcileMemberExit(exitInstance, member, now, foreign); err == nil {
		t.Fatal("foreign native identity accepted")
	}
	if _, err := os.Stat(filepath.Join(d.directory, exitInstance+"-member-exits.json")); !os.IsNotExist(err) {
		t.Fatal("unconfirmed member created a removal")
	}
	result := MemberExit{InstanceID: exitInstance, Ticket: exitTicket, DID: exitDID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()}
	if !result.Valid(exitInstance, exitTicket) {
		t.Fatal("valid window refused")
	}
	for _, bad := range []MemberExit{
		{InstanceID: result.InstanceID, Ticket: result.Ticket, DID: result.DID, State: "deactivated", RequestedAt: result.RequestedAt, DeactivateAfter: result.DeactivateAfter},
		{InstanceID: result.InstanceID, Ticket: result.Ticket, DID: result.DID, State: result.State, RequestedAt: result.RequestedAt, DeactivateAfter: result.DeactivateAfter - 1},
	} {
		if bad.Valid(exitInstance, exitTicket) {
			t.Fatal("premature or shortened exit accepted")
		}
	}
	if _, err := d.MemberExit(context.Background(), exitInstance, exitTicket); err == nil {
		t.Fatal("exit accepted without explicit local runtime")
	}
	d.EnableLocalIdentityFixtures()
	if _, err := d.MemberExit(context.Background(), "invalid", exitTicket); err == nil {
		t.Fatal("invalid instance accepted")
	}
}

type exitRouteRuntime struct{ calls, inspections int }

type legacyExitRuntime struct{ calls int }

func (*legacyExitRuntime) Ensure(context.Context, string) (Instance, error) { return Instance{}, nil }
func (r *legacyExitRuntime) MemberExit(context.Context, string, string) (MemberExit, error) {
	r.calls++
	return MemberExit{}, nil
}

func TestMemberExitInspectionNeverFallsBackToRemoval(t *testing.T) {
	runtime := &legacyExitRuntime{}
	w := httptest.NewRecorder()
	Handler(runtime).ServeHTTP(w, httptest.NewRequest("POST", "/v1/instances/"+exitInstance+"/members/exit/status", strings.NewReader(fmt.Sprintf("{\"ticket\":%q}", exitTicket))))
	if w.Code != 503 || runtime.calls != 0 {
		t.Fatal("unsupported inspection fell back to removal")
	}
}

func (r *exitRouteRuntime) MemberExitStatus(_ context.Context, id, ticket string) (MemberExit, error) {
	r.inspections++
	now := time.Now()
	return MemberExit{InstanceID: id, Ticket: ticket, DID: exitDID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()}, nil
}

func (*exitRouteRuntime) Ensure(context.Context, string) (Instance, error) { return Instance{}, nil }
func (r *exitRouteRuntime) MemberExit(_ context.Context, id, ticket string) (MemberExit, error) {
	r.calls++
	now := time.Now()
	return MemberExit{InstanceID: id, Ticket: ticket, DID: exitDID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()}, nil
}
func TestMemberExitRouteAcceptsOnlyRetainedTicket(t *testing.T) {
	for _, suffix := range []string{"", "/status"} {
		t.Run(suffix, func(t *testing.T) {
			runtime := &exitRouteRuntime{}
			handler := Handler(runtime)
			base := "/v1/instances/" + exitInstance + "/members/exit" + suffix
			for _, entry := range []struct {
				path, body string
				status     int
			}{
				{base, fmt.Sprintf("{\"ticket\":%q}", exitTicket), 200},
				{base, fmt.Sprintf("{\"ticket\":%q,\"did\":%q}", exitTicket, exitDID), 400},
				{base, fmt.Sprintf("{\"ticket\":%q,\"deactivateAfter\":1}", exitTicket), 400},
				{base, fmt.Sprintf("{\"ticket\":%q}{}", exitTicket), 400},
				{base + "?immediate=true", fmt.Sprintf("{\"ticket\":%q}", exitTicket), 404},
				{"/v1/instances/invalid/members/exit", fmt.Sprintf("{\"ticket\":%q}", exitTicket), 404},
			} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("POST", entry.path, strings.NewReader(entry.body)))
				if w.Code != entry.status {
					t.Fatalf("route %s returned %d", entry.path, w.Code)
				}
			}
			if runtime.calls+runtime.inspections != 1 || (suffix != "" && runtime.calls != 0) {
				t.Fatal("unallowlisted request dispatched")
			}
			request := httptest.NewRequest("POST", base, strings.NewReader(fmt.Sprintf("{\"ticket\":%q}", exitTicket)))
			w := httptest.NewRecorder()
			Authenticate(handler).ServeHTTP(w, request)
			if w.Code != 401 {
				t.Fatal("uncertified worker admitted")
			}
		})
	}
}

func TestMemberExitJournalBoundAndMalformedEntries(t *testing.T) {
	d := exitDocker(t)
	now := time.Now().UnixMilli()
	state := memberExitState{Version: 1, InstanceID: exitInstance, Entries: []MemberExit{}}
	for i := 0; i < MemberCeiling; i++ {
		state.Entries = append(state.Entries, MemberExit{InstanceID: exitInstance, Ticket: fmt.Sprintf("%08d-bbbb-bbbb-bbbb-bbbbbbbbbbbb", i), DID: "did:plc:" + strings.Repeat("a", 23) + string(rune('b'+i)), State: "migration_window", RequestedAt: now, DeactivateAfter: now + MemberMigrationWindow.Milliseconds()})
	}
	if err := d.writeMemberExits(state); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := d.reconcileMemberExit(exitInstance, exitMember(), time.Now(), func(string, string) (nativeMemberExit, error) { called = true; return nativeMemberExit{}, nil }); err == nil || called {
		t.Fatal("bounded journal created extra removal")
	}
	state.Entries[0].DeactivateAfter--
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(d.directory, exitInstance+"-member-exits.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.readMemberExits(exitInstance); err == nil {
		t.Fatal("shortened retained deadline accepted")
	}
}

func TestMemberExitNativeSuccessWithJournalFailureDoesNotClaimCompletion(t *testing.T) {
	d := exitDocker(t)
	now := time.Now()
	path := filepath.Join(d.directory, exitInstance+"-member-exits.json")
	first, err := d.reconcileMemberExit(exitInstance, exitMember(), now, func(_, did string) (nativeMemberExit, error) {
		return nativeMemberExit{DID: did, Handle: "member.test"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := d.reconcileMemberExit(exitInstance, exitMember(), now.Add(MemberMigrationWindow), func(_, did string) (nativeMemberExit, error) {
		// Simulate a filesystem failure after native success, before receipt replacement.
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		return nativeMemberExit{DID: did, Handle: "member.test", Deactivated: true}, nil
	})
	if err == nil || result.State != "" {
		t.Fatal("unpersisted native completion claimed success")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, retained, 0600); err != nil {
		t.Fatal(err)
	}
	result, err = d.reconcileMemberExit(exitInstance, exitMember(), now.Add(MemberMigrationWindow+time.Second), func(_, did string) (nativeMemberExit, error) {
		return nativeMemberExit{DID: did, Handle: "member.test", Deactivated: true}, nil
	})
	if err != nil || result.State != "deactivated" || result.DeactivateAfter != first.DeactivateAfter {
		t.Fatalf("receipt recovery: %+v %v", result, err)
	}
}
