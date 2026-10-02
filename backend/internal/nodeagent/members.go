package nodeagent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

//go:embed member-helper.cjs
var memberHelperScript string

// MemberCeiling bounds member accounts per PDS until plan limits replace it.
const MemberCeiling = 25

// MemberInvitationLifetime is the native admission window for one member code.
const MemberInvitationLifetime = 72 * time.Hour

const maximumMemberEntries = 256

// MemberInvitation contains one single-use code or the public result of its use.
type MemberInvitation struct {
	InstanceID string `json:"instanceId"`
	Ticket     string `json:"ticket"`
	State      string `json:"state"`
	Code       string `json:"code,omitempty"`
	ExpiresAt  int64  `json:"expiresAt,omitempty"`
	DID        string `json:"did,omitempty"`
	Handle     string `json:"handle,omitempty"`
}

// Valid checks a node response against the requested identity and action.
func (m MemberInvitation) Valid(id, ticket string, withdraw bool) bool {
	if m.InstanceID != id || m.Ticket != ticket {
		return false
	}
	now := time.Now().UnixMilli()
	switch m.State {
	case "invitation_ready":
		return !withdraw && validInvitationCode(m.Code) && m.ExpiresAt > now && m.ExpiresAt <= now+MemberInvitationLifetime.Milliseconds()+5000 && m.DID == "" && m.Handle == ""
	case "accepted":
		return accountDID.MatchString(m.DID) && m.Handle != "" && len(m.Handle) <= 253 && m.Code == "" && m.ExpiresAt == 0
	case "expired":
		return m.Code == "" && m.DID == "" && m.Handle == "" && m.ExpiresAt > 0
	case "withdrawn", "ceiling_reached", "owner_required":
		return m.Code == "" && m.DID == "" && m.Handle == "" && m.ExpiresAt == 0
	}
	return false
}

// memberEntry is private agent state. Codes never reach the admission guard.
type memberEntry struct {
	Ticket    string `json:"ticket"`
	State     string `json:"state"`
	Code      string `json:"code,omitempty"`
	IssuedAt  int64  `json:"issuedAt,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	DID       string `json:"did,omitempty"`
	Handle    string `json:"handle,omitempty"`
}
type memberState struct {
	InstanceID string        `json:"instanceId"`
	Entries    []memberEntry `json:"entries"`
}
type memberAdmissionEntry struct {
	Ticket    string `json:"ticket"`
	IssuedAt  int64  `json:"issuedAt"`
	ExpiresAt int64  `json:"expiresAt"`
}
type memberAdmissionView struct {
	InstanceID  string                 `json:"instanceId"`
	Ceiling     int                    `json:"ceiling"`
	Invitations []memberAdmissionEntry `json:"invitations"`
}
type nativeMemberCode struct {
	Known    bool   `json:"known"`
	Disabled bool   `json:"disabled"`
	DID      string `json:"did"`
	Handle   string `json:"handle"`
}
type nativeMembers struct {
	Accounts       int                         `json:"accounts"`
	Owner          string                      `json:"owner"`
	Usable         int                         `json:"usable"`
	Codes          map[string]nativeMemberCode `json:"codes"`
	Code           string                      `json:"code,omitempty"`
	OwnerRequired  bool                        `json:"ownerRequired,omitempty"`
	CeilingReached bool                        `json:"ceilingReached,omitempty"`
}

// SetMemberCeiling lowers the member ceiling for local qualification.
func (d *Docker) SetMemberCeiling(maximum int) error {
	if maximum < 1 || maximum > MemberCeiling {
		return errors.New("member ceiling must be between 1 and 25")
	}
	d.memberCeiling = maximum
	return nil
}
func (d *Docker) members() int {
	if d.memberCeiling == 0 {
		return MemberCeiling
	}
	return d.memberCeiling
}

func validMemberEntry(entry memberEntry) bool {
	if !instanceID.MatchString(entry.Ticket) || (entry.Code != "" && !validInvitationCode(entry.Code)) || (entry.DID != "" && !accountDID.MatchString(entry.DID)) || len(entry.Handle) > 253 {
		return false
	}
	switch entry.State {
	case "issuing":
		return entry.Code == "" && entry.DID == ""
	case "open":
		return entry.Code != "" && entry.DID == "" && entry.IssuedAt > 0 && entry.ExpiresAt > entry.IssuedAt && entry.ExpiresAt-entry.IssuedAt <= MemberInvitationLifetime.Milliseconds()
	case "accepted":
		return entry.DID != "" && entry.Handle != ""
	case "expired", "withdrawn", "ceiling_reached", "owner_required":
		return entry.DID == ""
	}
	return false
}

func (d *Docker) readMemberState(id string) (memberState, error) {
	state := memberState{InstanceID: id}
	path := filepath.Join(d.directory, id+"-members.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256<<10 {
		return state, errors.New("unsafe member state")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if json.Unmarshal(data, &state) != nil || state.InstanceID != id || len(state.Entries) > maximumMemberEntries {
		return state, errors.New("invalid member state")
	}
	seen := map[string]bool{}
	for _, entry := range state.Entries {
		if !validMemberEntry(entry) || seen[entry.Ticket] {
			return state, errors.New("invalid member entry")
		}
		seen[entry.Ticket] = true
	}
	return state, nil
}

func (d *Docker) writeMemberState(state memberState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(d.directory, ".members-")
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
	if err = os.Rename(file.Name(), filepath.Join(d.directory, state.InstanceID+"-members.json")); err != nil {
		return err
	}
	folder, err := os.Open(d.directory)
	if err != nil {
		return err
	}
	err = folder.Sync()
	folder.Close()
	return err
}

// publishMemberAdmission exposes only current tickets and their windows to the guard.
func (d *Docker) publishMemberAdmission(ctx context.Context, state memberState) error {
	view := memberAdmissionView{InstanceID: state.InstanceID, Ceiling: d.members(), Invitations: []memberAdmissionEntry{}}
	now := time.Now().UnixMilli()
	for _, entry := range state.Entries {
		if entry.State == "open" && entry.ExpiresAt > now {
			view.Invitations = append(view.Invitations, memberAdmissionEntry{entry.Ticket, entry.IssuedAt, entry.ExpiresAt})
		}
	}
	data, err := json.Marshal(view)
	if err != nil {
		return err
	}
	return d.publishGuardPolicy(ctx, state.InstanceID, "members.json", data)
}

func (d *Docker) memberHelper(ctx context.Context, id string, args []string) (nativeMembers, error) {
	var native nativeMembers
	if err := d.executeFixed(ctx, "pds-ass-tenant-"+id, memberHelperScript, args, &native); err != nil {
		return native, err
	}
	if native.Accounts < 0 || native.Accounts > MemberCeiling+1 || native.Usable < 0 || (native.Accounts > 0 && !accountDID.MatchString(native.Owner)) || (native.Code != "" && !validInvitationCode(native.Code)) {
		return native, errors.New("invalid native member inventory")
	}
	for code, value := range native.Codes {
		if !validInvitationCode(code) || (value.DID != "" && (!accountDID.MatchString(value.DID) || value.Handle == "" || len(value.Handle) > 253)) {
			return native, errors.New("invalid native member code")
		}
	}
	return native, nil
}

// settle disables the named open codes and records whether each was used first.
// A disabled code cannot create an account afterwards.
func (d *Docker) settle(ctx context.Context, id string, state *memberState, tickets map[string]string) error {
	codes := []string{}
	for index, entry := range state.Entries {
		if next, ok := tickets[entry.Ticket]; ok && entry.State == "open" {
			codes = append(codes, entry.Code)
			state.Entries[index].State = next
		}
	}
	if len(codes) == 0 {
		return nil
	}
	native, err := d.memberHelper(ctx, id, append([]string{"disable"}, codes...))
	if err != nil {
		return err
	}
	for index, entry := range state.Entries {
		if result, ok := native.Codes[entry.Code]; ok && entry.Code != "" {
			if !result.Known || !result.Disabled {
				return errors.New("native member code was not disabled")
			}
			if result.DID != "" {
				state.Entries[index].State, state.Entries[index].DID, state.Entries[index].Handle = "accepted", result.DID, result.Handle
			}
			state.Entries[index].Code = ""
		}
	}
	return d.writeMemberState(*state)
}

func (d *Docker) memberRuntime(ctx context.Context, id, ticket string) error {
	if !d.localIdentity || !instanceID.MatchString(id) || !instanceID.MatchString(ticket) {
		return errors.New("isolated instance and member ticket required")
	}
	current, err := d.status(ctx, id)
	if err != nil {
		return errors.New("guarded local PDS policy could not be confirmed")
	}
	if current.State == "suspended" || !current.Healthy {
		return errors.New("healthy unheld local PDS required")
	}
	return nil
}

func memberResult(id string, entry memberEntry) MemberInvitation {
	result := MemberInvitation{InstanceID: id, Ticket: entry.Ticket, State: entry.State, DID: entry.DID, Handle: entry.Handle}
	switch entry.State {
	case "open":
		result.State, result.Code, result.ExpiresAt = "invitation_ready", entry.Code, entry.ExpiresAt
	case "expired":
		result.ExpiresAt = entry.ExpiresAt
	}
	return result
}

// MemberInvitation issues or reports one single-use member code. Replays keep
// the original code and expiry; the native ceiling counts accounts and usable codes.
func (d *Docker) MemberInvitation(ctx context.Context, id, ticket string) (MemberInvitation, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if err := d.memberRuntime(ctx, id, ticket); err != nil {
		return MemberInvitation{}, err
	}
	state, err := d.readMemberState(id)
	if err != nil {
		return MemberInvitation{}, err
	}
	now := time.Now().UnixMilli()
	expired := map[string]string{}
	for _, entry := range state.Entries {
		if entry.State == "open" && entry.ExpiresAt <= now {
			expired[entry.Ticket] = "expired"
		}
	}
	if err = d.settle(ctx, id, &state, expired); err != nil {
		return MemberInvitation{}, err
	}
	index := -1
	for i, entry := range state.Entries {
		if entry.Ticket == ticket {
			index = i
		}
	}
	if index >= 0 && state.Entries[index].State == "open" {
		native, err := d.memberHelper(ctx, id, []string{"inventory", state.Entries[index].Code})
		if err != nil {
			return MemberInvitation{}, err
		}
		if used := native.Codes[state.Entries[index].Code]; used.DID != "" {
			state.Entries[index].State, state.Entries[index].Code, state.Entries[index].DID, state.Entries[index].Handle = "accepted", "", used.DID, used.Handle
			if err = d.writeMemberState(state); err != nil {
				return MemberInvitation{}, err
			}
		} else if !used.Known || used.Disabled {
			return MemberInvitation{}, errors.New("retained member code differs from native state")
		}
	}
	if index >= 0 && state.Entries[index].State != "issuing" {
		if err = d.publishMemberAdmission(ctx, state); err != nil {
			return MemberInvitation{}, err
		}
		return memberResult(id, state.Entries[index]), nil
	}
	if index < 0 {
		if len(state.Entries) >= maximumMemberEntries {
			pruned := false
			for i, entry := range state.Entries {
				if entry.State != "open" && entry.State != "issuing" && entry.State != "accepted" {
					state.Entries = append(state.Entries[:i], state.Entries[i+1:]...)
					pruned = true
					break
				}
			}
			if !pruned {
				return MemberInvitation{}, errors.New("member state bound reached")
			}
		}
		// Record intent before the native mutation. A lost response leaves an
		// untracked code, which the next issuance disables before counting.
		state.Entries = append(state.Entries, memberEntry{Ticket: ticket, State: "issuing"})
		index = len(state.Entries) - 1
		if err = d.writeMemberState(state); err != nil {
			return MemberInvitation{}, err
		}
	}
	args := []string{"issue", strconv.Itoa(d.members())}
	for _, entry := range state.Entries {
		if entry.State == "open" {
			args = append(args, entry.Code)
		}
	}
	native, err := d.memberHelper(ctx, id, args)
	if err != nil {
		return MemberInvitation{}, err
	}
	switch {
	case native.OwnerRequired:
		state.Entries[index].State = "owner_required"
	case native.CeilingReached:
		state.Entries[index].State = "ceiling_reached"
	case native.Code != "":
		issued := time.Now().UnixMilli()
		state.Entries[index] = memberEntry{Ticket: ticket, State: "open", Code: native.Code, IssuedAt: issued, ExpiresAt: issued + MemberInvitationLifetime.Milliseconds()}
	default:
		return MemberInvitation{}, errors.New("native member response invalid")
	}
	if err = d.writeMemberState(state); err != nil {
		return MemberInvitation{}, err
	}
	if err = d.publishMemberAdmission(ctx, state); err != nil {
		return MemberInvitation{}, err
	}
	return memberResult(id, state.Entries[index]), nil
}

// WithdrawMemberInvitation disables one unused code. A code used before the
// disable is reported as an accepted member; no account is changed.
func (d *Docker) WithdrawMemberInvitation(ctx context.Context, id, ticket string) (MemberInvitation, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if err := d.memberRuntime(ctx, id, ticket); err != nil {
		return MemberInvitation{}, err
	}
	state, err := d.readMemberState(id)
	if err != nil {
		return MemberInvitation{}, err
	}
	index := -1
	for i, entry := range state.Entries {
		if entry.Ticket == ticket {
			index = i
		}
	}
	if index < 0 {
		return MemberInvitation{}, errors.New("retained member ticket required")
	}
	switch state.Entries[index].State {
	case "open":
		if err = d.settle(ctx, id, &state, map[string]string{ticket: "withdrawn"}); err != nil {
			return MemberInvitation{}, err
		}
	case "issuing":
		// The code from a lost issuance response is unknown. Disable every
		// untracked usable code; tracked open codes stay valid.
		tracked := []string{"sweep"}
		for _, entry := range state.Entries {
			if entry.State == "open" {
				tracked = append(tracked, entry.Code)
			}
		}
		if _, err = d.memberHelper(ctx, id, tracked); err != nil {
			return MemberInvitation{}, err
		}
		state.Entries[index].State = "withdrawn"
		if err = d.writeMemberState(state); err != nil {
			return MemberInvitation{}, err
		}
	}
	if err = d.publishMemberAdmission(ctx, state); err != nil {
		return MemberInvitation{}, err
	}
	return memberResult(id, state.Entries[index]), nil
}
