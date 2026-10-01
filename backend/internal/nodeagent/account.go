package nodeagent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

//go:embed account-helper.cjs
var accountHelperScript string

// AccountResult contains public moderation state, never native credentials or email.
type AccountResult struct {
	ServiceResult
	DID         string `json:"did"`
	Deactivated bool   `json:"deactivated"`
}
type nativeAccount struct {
	DID         string `json:"did"`
	State       string `json:"state"`
	AppliedRef  string `json:"appliedRef"`
	Deactivated bool   `json:"deactivated"`
}
type accountLease struct {
	AccountResult
	Phase      string `json:"phase"`
	AppliedRef string `json:"appliedRef"`
}

var accountDID = regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`)

func (d *Docker) readAccountLease(id string) (accountLease, error) {
	var lease accountLease
	path := filepath.Join(d.directory, id+"-account.json")
	info, err := os.Lstat(path)
	if err != nil {
		return lease, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 2048 {
		return lease, errors.New("unsafe account lease")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return lease, err
	}
	if json.Unmarshal(data, &lease) != nil || lease.InstanceID != id || !lease.ServiceRequest.Valid() || (lease.Phase != "pending" && lease.Phase != "applied") || (lease.DID != "" && !accountDID.MatchString(lease.DID)) || (lease.AppliedRef != "" && !instanceID.MatchString(lease.AppliedRef)) {
		return lease, errors.New("invalid account lease")
	}
	if lease.Phase == "applied" && (!accountDID.MatchString(lease.DID) || (lease.State == "suspended" && lease.AppliedRef != lease.Ticket)) {
		return lease, errors.New("unconfirmed account lease")
	}
	return lease, nil
}
func (d *Docker) writeAccountLease(lease accountLease) error {
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(d.directory, ".account-")
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
	if err = os.Rename(file.Name(), filepath.Join(d.directory, lease.InstanceID+"-account.json")); err != nil {
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

// Account applies a native takedown to the sole retained account. It never clears user deactivation.
func (d *Docker) Account(ctx context.Context, id string, request ServiceRequest) (AccountResult, error) {
	result := AccountResult{ServiceResult: ServiceResult{InstanceID: id, ServiceRequest: request}}
	if !d.localIdentity || !instanceID.MatchString(id) || !request.Valid() {
		return result, errors.New("fixed local account request required")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	previous, err := d.readAccountLease(id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if previous.Generation > request.Generation || (previous.Generation == request.Generation && previous.ServiceRequest != request) {
		return result, errors.New("stale or conflicting account generation")
	}
	current, err := d.status(ctx, id)
	if err != nil || current.State != "running" || !current.Healthy {
		return result, errors.New("healthy unheld native service required")
	}
	// Inspect before replacing uncertain custody. A prior pending suspension may
	// already have reached native storage even though its checkpoint was lost.
	var observed nativeAccount
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, accountHelperScript, []string{"native_status"}, &observed); err != nil {
		return result, errors.New("native account inspection unavailable")
	}
	if !accountDID.MatchString(observed.DID) || (observed.State != "running" && observed.State != "suspended") || (previous.DID != "" && previous.DID != observed.DID) {
		return result, errors.New("native account identity differs")
	}
	if observed.State == "suspended" && (observed.AppliedRef == "" || !instanceID.MatchString(observed.AppliedRef) || (observed.AppliedRef != previous.AppliedRef && !(previous.Phase == "pending" && previous.State == "suspended" && observed.AppliedRef == previous.Ticket))) {
		return result, errors.New("unowned native moderation must remain intact")
	}
	if observed.State == "running" && observed.AppliedRef != "" {
		return result, errors.New("invalid native moderation state")
	}
	result.DID = observed.DID
	lease := accountLease{AccountResult: result, Phase: "pending", AppliedRef: observed.AppliedRef}
	if err = d.writeAccountLease(lease); err != nil {
		return result, err
	}
	var native nativeAccount
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, accountHelperScript, []string{"apply", request.State, request.Ticket, lease.AppliedRef}, &native); err != nil {
		return result, errors.New("native account transition could not be confirmed")
	}
	if !accountDID.MatchString(native.DID) || native.State != request.State || (previous.DID != "" && previous.DID != native.DID) || (request.State == "suspended" && native.AppliedRef != request.Ticket) || (request.State == "running" && native.AppliedRef != "") {
		return result, errors.New("native account transition differs from intent")
	}
	result.DID = native.DID
	result.Deactivated = native.Deactivated
	lease.AccountResult = result
	lease.Phase = "applied"
	if request.State == "suspended" {
		lease.AppliedRef = request.Ticket
	}
	return result, d.writeAccountLease(lease)
}
