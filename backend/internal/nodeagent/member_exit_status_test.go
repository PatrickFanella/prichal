package nodeagent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemberExitInspectionNeverCreatesOrAdvancesWindow(t *testing.T) {
	d := exitDocker(t)
	calls := 0
	native := nativeMemberExit{DID: exitDID, Handle: "changed.test"}
	inspect := func(did string) (nativeMemberExit, error) {
		calls++
		if did != exitDID {
			t.Fatal("foreign native target")
		}
		return native, nil
	}
	if _, err := d.inspectMemberExit(exitInstance, exitMember(), inspect); err == nil || calls != 0 {
		t.Fatal("inspection admitted absent journal")
	}
	path := filepath.Join(d.directory, exitInstance+"-member-exits.json")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection created journal")
	}
	// An overdue window is observed without deactivation or journal repair.
	start := time.Now().Add(-MemberMigrationWindow - time.Hour).UnixMilli()
	entry := MemberExit{InstanceID: exitInstance, Ticket: exitTicket, DID: exitDID, State: "migration_window", RequestedAt: start, DeactivateAfter: start + MemberMigrationWindow.Milliseconds()}
	state := memberExitState{Version: 1, InstanceID: exitInstance, Entries: []MemberExit{entry}}
	if err := d.writeMemberExits(state); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, inactive := range []bool{false, true} {
		native.Deactivated = inactive
		got, err := d.inspectMemberExit(exitInstance, exitMember(), inspect)
		if err != nil || got != entry {
			t.Fatalf("inspection advanced window: %+v %v", got, err)
		}
	}
	for _, bad := range []nativeMemberExit{{DID: "did:plc:cccccccccccccccccccccccc", Handle: "other.test"}, {DID: exitDID}} {
		native = bad
		if _, err := d.inspectMemberExit(exitInstance, exitMember(), inspect); err == nil {
			t.Fatal("unconfirmed native identity admitted")
		}
	}
	if _, err := d.inspectMemberExit(exitInstance, exitMember(), func(string) (nativeMemberExit, error) { return nativeMemberExit{}, errors.New("PRIVATE FAILURE") }); err == nil || err.Error() == "PRIVATE FAILURE" {
		t.Fatal("inspection failure not redacted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("inspection changed retained deadline bytes")
	}
	state.Entries[0].State = "deactivated"
	if err := d.writeMemberExits(state); err != nil {
		t.Fatal(err)
	}
	native = nativeMemberExit{DID: exitDID, Handle: "member.test"}
	if _, err := d.inspectMemberExit(exitInstance, exitMember(), inspect); err == nil {
		t.Fatal("active native account confirmed completed journal")
	}
	native.Deactivated = true
	if got, err := d.inspectMemberExit(exitInstance, exitMember(), inspect); err != nil || got.State != "deactivated" {
		t.Fatal("confirmed completion unavailable")
	}
	changed := exitMember()
	changed.DID = "did:plc:cccccccccccccccccccccccc"
	beforeCalls := calls
	if _, err := d.inspectMemberExit(exitInstance, changed, inspect); err == nil || calls != beforeCalls {
		t.Fatal("foreign retained identity dispatched")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.inspectMemberExit(exitInstance, exitMember(), inspect); err == nil || calls != beforeCalls {
		t.Fatal("unsafe journal dispatched")
	}
}
