package nodeagent

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"
)

//go:embed member-exit-helper.cjs
var memberExitHelperScript string

// MemberMigrationWindow is never supplied or shortened by a remote caller.
const MemberMigrationWindow = 30 * 24 * time.Hour

type MemberExit struct {
	InstanceID      string `json:"instanceId"`
	Ticket          string `json:"ticket"`
	DID             string `json:"did"`
	State           string `json:"state"`
	RequestedAt     int64  `json:"requestedAt"`
	DeactivateAfter int64  `json:"deactivateAfter"`
}

func (m MemberExit) validMetadata(id, ticket string) bool {
	return instanceID.MatchString(id) && instanceID.MatchString(ticket) && m.InstanceID == id && m.Ticket == ticket && accountDID.MatchString(m.DID) && m.RequestedAt > 0 && m.RequestedAt <= math.MaxInt64-MemberMigrationWindow.Milliseconds() && m.DeactivateAfter == m.RequestedAt+MemberMigrationWindow.Milliseconds() && (m.State == "migration_window" || m.State == "deactivated")
}

// Valid rejects premature native completion and a different invitation identity.
func (m MemberExit) Valid(id, ticket string) bool {
	return m.validMetadata(id, ticket) && m.RequestedAt <= time.Now().Add(time.Minute).UnixMilli() && (m.State != "deactivated" || m.DeactivateAfter <= time.Now().UnixMilli())
}

type memberExitState struct {
	Version    int          `json:"version"`
	InstanceID string       `json:"instanceId"`
	Entries    []MemberExit `json:"entries"`
}
type nativeMemberExit struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	Deactivated bool   `json:"deactivated"`
}

func (d *Docker) readMemberExits(id string) (memberExitState, error) {
	state := memberExitState{Version: 1, InstanceID: id, Entries: []MemberExit{}}
	path := filepath.Join(d.directory, id+"-member-exits.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		return state, errors.New("unsafe member exit state")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	var decoded *memberExitState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || decoder.Decode(new(any)) != io.EOF || decoded == nil || decoded.Entries == nil || decoded.Version != 1 || decoded.InstanceID != id || len(decoded.Entries) > MemberCeiling {
		return state, errors.New("invalid member exit state")
	}
	state = *decoded
	tickets, dids := map[string]bool{}, map[string]bool{}
	for _, entry := range state.Entries {
		if !entry.validMetadata(id, entry.Ticket) || tickets[entry.Ticket] || dids[entry.DID] {
			return state, errors.New("invalid member exit entry")
		}
		tickets[entry.Ticket] = true
		dids[entry.DID] = true
	}
	return state, nil
}
func (d *Docker) writeMemberExits(state memberExitState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(d.directory, ".member-exits-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(d.directory, state.InstanceID+"-member-exits.json")); err != nil {
		return err
	}
	directory, err := os.Open(d.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// MemberExit binds a retained accepted invitation to a fixed, durable window.
// Replays inspect/advance that same request; no caller supplies a DID or time.
func (d *Docker) MemberExit(ctx context.Context, id, ticket string) (MemberExit, error) {
	return d.memberExitAt(ctx, id, ticket, time.Now())
}

// MemberExitStatus inspects an existing window without creating or advancing it.
// Native status may confirm completion, but inspection never repairs the journal.
func (d *Docker) MemberExitStatus(ctx context.Context, id, ticket string) (MemberExit, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.pilot != nil {
		return MemberExit{}, errors.New("member exit inspection requires isolated local identity")
	}
	if err := d.memberRuntime(ctx, id, ticket); err != nil {
		return MemberExit{}, err
	}
	members, err := d.readMemberState(id)
	if err != nil {
		return MemberExit{}, err
	}
	for _, member := range members.Entries {
		if member.Ticket == ticket && member.State == "accepted" && accountDID.MatchString(member.DID) {
			return d.inspectMemberExit(id, member, func(did string) (nativeMemberExit, error) {
				var result nativeMemberExit
				err := d.executeFixed(ctx, "pds-ass-tenant-"+id, memberExitHelperScript, []string{"native_status", did}, &result)
				return result, err
			})
		}
	}
	return MemberExit{}, errors.New("retained accepted member required")
}

func (d *Docker) inspectMemberExit(id string, member memberEntry, inspect func(string) (nativeMemberExit, error)) (MemberExit, error) {
	if !instanceID.MatchString(id) || !instanceID.MatchString(member.Ticket) || member.State != "accepted" || !accountDID.MatchString(member.DID) {
		return MemberExit{}, errors.New("invalid member exit inspection")
	}
	state, err := d.readMemberExits(id)
	if err != nil {
		return MemberExit{}, err
	}
	for _, entry := range state.Entries {
		if entry.Ticket != member.Ticket {
			continue
		}
		if entry.DID != member.DID || !entry.Valid(id, member.Ticket) {
			return MemberExit{}, errors.New("retained member exit identity or window differs")
		}
		native, err := inspect(member.DID)
		if err != nil || native.DID != member.DID || native.Handle == "" || len(native.Handle) > 253 || (entry.State == "deactivated" && !native.Deactivated) {
			return MemberExit{}, errors.New("native member exit status differs")
		}
		return entry, nil
	}
	return MemberExit{}, errors.New("retained member exit required")
}

func (d *Docker) memberExitAt(ctx context.Context, id, ticket string, now time.Time) (MemberExit, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.pilot != nil {
		return MemberExit{}, errors.New("member exit requires isolated local identity")
	}
	if err := d.memberRuntime(ctx, id, ticket); err != nil {
		return MemberExit{}, err
	}
	members, err := d.readMemberState(id)
	if err != nil {
		return MemberExit{}, err
	}
	for _, entry := range members.Entries {
		if entry.Ticket == ticket && entry.State == "accepted" && accountDID.MatchString(entry.DID) {
			return d.reconcileMemberExit(id, entry, now, func(action, did string) (nativeMemberExit, error) {
				var result nativeMemberExit
				err := d.executeFixed(ctx, "pds-ass-tenant-"+id, memberExitHelperScript, []string{action, did}, &result)
				return result, err
			})
		}
	}
	return MemberExit{}, errors.New("retained accepted member required")
}
func (d *Docker) reconcileMemberExit(id string, member memberEntry, now time.Time, call func(string, string) (nativeMemberExit, error)) (MemberExit, error) {
	if !instanceID.MatchString(id) || !instanceID.MatchString(member.Ticket) || member.State != "accepted" || !accountDID.MatchString(member.DID) || now.UnixMilli() <= 0 || now.UnixMilli() > math.MaxInt64-MemberMigrationWindow.Milliseconds() {
		return MemberExit{}, errors.New("invalid member exit intent")
	}
	state, err := d.readMemberExits(id)
	if err != nil {
		return MemberExit{}, err
	}
	index := -1
	for i, entry := range state.Entries {
		if entry.Ticket == member.Ticket {
			index = i
			if entry.DID != member.DID {
				return MemberExit{}, errors.New("retained member identity changed")
			}
		} else if entry.DID == member.DID {
			return MemberExit{}, errors.New("member exit already belongs to another invitation")
		}
	}
	confirm := func(action string) error {
		result, err := call(action, member.DID)
		if err != nil || result.DID != member.DID || result.Handle == "" || len(result.Handle) > 253 || (action == "deactivate" && !result.Deactivated) {
			return errors.New("native member exit could not be confirmed")
		}
		return nil
	}
	if index < 0 {
		if len(state.Entries) >= MemberCeiling {
			return MemberExit{}, errors.New("member exit state bound reached")
		}
		if err = confirm("native_status"); err != nil {
			return MemberExit{}, err
		}
		state.Entries = append(state.Entries, MemberExit{InstanceID: id, Ticket: member.Ticket, DID: member.DID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()})
		index = len(state.Entries) - 1
		if err = d.writeMemberExits(state); err != nil {
			return MemberExit{}, err
		}
	}
	result := state.Entries[index]
	if result.State == "deactivated" || now.UnixMilli() < result.DeactivateAfter {
		return result, nil
	}
	if err = confirm("deactivate"); err != nil {
		return MemberExit{}, err
	}
	state.Entries[index].State = "deactivated"
	if err = d.writeMemberExits(state); err != nil {
		return MemberExit{}, err
	}
	return state.Entries[index], nil
}
