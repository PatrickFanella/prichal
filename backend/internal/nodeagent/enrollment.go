package nodeagent

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

//go:embed enrollment-helper.cjs
var enrollmentHelperScript string

// Enrollment contains only an expiring owner invitation and public account status.
type Enrollment struct {
	InstanceID     string `json:"instanceId"`
	Ticket         string `json:"ticket"`
	State          string `json:"state"`
	Code           string `json:"code,omitempty"`
	ExpiresAt      int64  `json:"expiresAt,omitempty"`
	DID            string `json:"did,omitempty"`
	Handle         string `json:"handle,omitempty"`
	EmailConfirmed bool   `json:"emailConfirmed"`
}
type enrollmentLease struct {
	InstanceID string `json:"instanceId"`
	Ticket     string `json:"ticket"`
	State      string `json:"state"`
	IssuedAt   int64  `json:"issuedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	Code       string `json:"code,omitempty"`
}
type nativeEnrollment struct {
	Count          int    `json:"count"`
	Code           string `json:"code,omitempty"`
	DID            string `json:"did,omitempty"`
	Handle         string `json:"handle,omitempty"`
	EmailConfirmed bool   `json:"emailConfirmed,omitempty"`
}

func (d *Docker) readEnrollmentLease(id string) (enrollmentLease, error) {
	var lease enrollmentLease
	path := filepath.Join(d.directory, id+"-enrollment", "lease.json")
	info, err := os.Lstat(path)
	if err != nil {
		return lease, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return lease, errors.New("unsafe enrollment lease")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return lease, err
	}
	if json.Unmarshal(data, &lease) != nil || lease.InstanceID != id || !instanceID.MatchString(lease.Ticket) {
		return lease, errors.New("invalid enrollment lease")
	}
	switch lease.State {
	case "closed", "preparing", "open", "complete", "expired", "withdrawing", "withdrawn":
	default:
		return lease, errors.New("invalid enrollment state")
	}
	return lease, nil
}
func (d *Docker) publishEnrollmentLease(ctx context.Context, lease enrollmentLease) error {
	directory := filepath.Join(d.directory, lease.InstanceID+"-enrollment")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("private enrollment directory required")
	}
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".lease-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporary.Name(), filepath.Join(directory, "lease.json")); err != nil {
		return err
	}
	folder, err := os.Open(directory)
	if err != nil {
		return err
	}
	err = folder.Sync()
	folder.Close()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	// The publication is confirmed inside the guard, not inferred from a host write.
	const script = `const fs=require('node:fs'),crypto=require('node:crypto');process.stdout.write(JSON.stringify({hash:crypto.createHash('sha256').update(fs.readFileSync('/enrollment/lease.json')).digest('hex')}))`
	for attempt := 0; attempt < 10; attempt++ {
		var observed struct {
			Hash string `json:"hash"`
		}
		if err = d.executeFixed(ctx, "pds-ass-enrollment-"+lease.InstanceID, script, nil, &observed); err == nil && observed.Hash == hex.EncodeToString(digest[:]) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("enrollment policy publication could not be confirmed")
}

// Enroll is local-only. It never accepts passwords or arbitrary native administrator actions.
func (d *Docker) Enroll(ctx context.Context, id, ticket string) (Enrollment, error) {
	result := Enrollment{InstanceID: id, Ticket: ticket}
	if !d.localIdentity || !instanceID.MatchString(id) || !instanceID.MatchString(ticket) {
		return result, errors.New("isolated instance and enrollment ticket required")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	lease, err := d.readEnrollmentLease(id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if lease.Ticket == ticket && (lease.State == "withdrawing" || lease.State == "withdrawn") {
		return result, errors.New("withdrawn enrollment requires a new recovery ticket")
	}
	current, err := d.status(ctx, id)
	if err != nil {
		return result, errors.New("guarded local PDS policy could not be confirmed")
	}
	if current.State == "suspended" {
		return result, errors.New("service is suspended")
	}
	if !current.Healthy {
		if lease.Ticket != ticket || lease.State != "preparing" {
			return result, errors.New("healthy guarded local PDS required")
		}
		if _, err = d.call(ctx, "POST", "/containers/pds-ass-tenant-"+id+"/start", nil, nil); err != nil {
			return result, err
		}
		if err = d.waitEnrollmentRuntime(ctx, id); err != nil {
			return result, err
		}
	}
	var native nativeEnrollment
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, enrollmentHelperScript, []string{"inventory"}, &native); err != nil {
		return result, err
	}
	if native.Count == 1 {
		if !regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`).MatchString(native.DID) || len(native.Handle) > 253 {
			return result, errors.New("invalid native account status")
		}
		closed := enrollmentLease{InstanceID: id, Ticket: ticket, State: "complete"}
		if err = d.publishEnrollmentLease(ctx, closed); err != nil {
			return result, err
		}
		result.State = "account_created"
		result.DID = native.DID
		result.Handle = native.Handle
		result.EmailConfirmed = native.EmailConfirmed
		if native.EmailConfirmed {
			result.State = "verified"
		}
		return result, nil
	}
	if native.Count != 0 || lease.State == "complete" {
		return result, errors.New("first-account inventory conflicts with retained enrollment")
	}
	if lease.Ticket == ticket && lease.ExpiresAt > 0 {
		if lease.ExpiresAt <= time.Now().UnixMilli() {
			lease.State = "expired"
			lease.Code = ""
			if err = d.publishEnrollmentLease(ctx, lease); err != nil {
				return result, err
			}
			result.State = "expired"
			result.ExpiresAt = lease.ExpiresAt
			return result, nil
		}
		if lease.State == "open" && lease.ExpiresAt-lease.IssuedAt <= int64(15*time.Minute/time.Millisecond) && lease.IssuedAt <= time.Now().UnixMilli() && validInvitationCode(lease.Code) {
			result.State = "invitation_ready"
			result.Code = lease.Code
			result.ExpiresAt = lease.ExpiresAt
			return result, nil
		}
		return result, errors.New("retained enrollment conflicts with ticket")
	}
	// Close admission, then drain the native process before replacement invitation
	// issuance. An earlier accepted signup must finish or terminate before recounting.
	lease = enrollmentLease{InstanceID: id, Ticket: ticket, State: "preparing"}
	if err = d.publishEnrollmentLease(ctx, lease); err != nil {
		return result, err
	}
	if _, err = d.call(ctx, "POST", "/containers/pds-ass-tenant-"+id+"/stop?t=10", nil, nil); err != nil {
		return result, err
	}
	if _, err = d.call(ctx, "POST", "/containers/pds-ass-tenant-"+id+"/start", nil, nil); err != nil {
		return result, err
	}
	if err = d.waitEnrollmentRuntime(ctx, id); err != nil {
		return result, err
	}
	native = nativeEnrollment{}
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, enrollmentHelperScript, []string{"invitation"}, &native); err != nil {
		return result, err
	}
	if native.Count == 1 {
		// A signup completed during the drain. Never issue another invitation.
		closed := enrollmentLease{InstanceID: id, Ticket: ticket, State: "complete"}
		if err = d.publishEnrollmentLease(ctx, closed); err != nil {
			return result, err
		}
		result.State = "account_created"
		result.DID = native.DID
		result.Handle = native.Handle
		result.EmailConfirmed = native.EmailConfirmed
		if native.EmailConfirmed {
			result.State = "verified"
		}
		return result, nil
	}
	if native.Count != 0 || !validInvitationCode(native.Code) {
		return result, errors.New("native invitation response invalid")
	}
	lease.State = "open"
	lease.Code = native.Code
	lease.IssuedAt = time.Now().UnixMilli()
	lease.ExpiresAt = lease.IssuedAt + int64(15*time.Minute/time.Millisecond)
	if err = d.publishEnrollmentLease(ctx, lease); err != nil {
		return result, err
	}
	result.State = "invitation_ready"
	result.Code = lease.Code
	result.ExpiresAt = lease.ExpiresAt
	return result, nil
}
func validInvitationCode(code string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`).MatchString(code)
}

// WithdrawEnrollment closes only the retained first-account ticket. It never
// deletes an account, suspends service, or replaces an invitation.
func (d *Docker) WithdrawEnrollment(ctx context.Context, id, ticket string) (Enrollment, error) {
	result := Enrollment{InstanceID: id, Ticket: ticket}
	if !d.localIdentity || !instanceID.MatchString(id) || !instanceID.MatchString(ticket) {
		return result, errors.New("isolated instance and enrollment ticket required")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	lease, err := d.readEnrollmentLease(id)
	if err != nil || lease.Ticket != ticket {
		return result, errors.New("retained enrollment ticket required")
	}
	current, err := d.status(ctx, id)
	if err != nil || current.State == "suspended" || (!current.Healthy && lease.State != "withdrawing") {
		return result, errors.New("guarded local PDS policy could not be confirmed")
	}
	// A retry after an interrupted drain must finish the drain before inventory.
	if lease.State != "withdrawing" {
		var native nativeEnrollment
		if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, enrollmentHelperScript, []string{"inventory"}, &native); err != nil {
			return result, err
		}
		if native.Count == 1 {
			return enrollmentAccount(result, native)
		}
		if native.Count != 0 || lease.State == "complete" {
			return result, errors.New("first-account inventory conflicts with retained enrollment")
		}
		if lease.State == "withdrawn" {
			result.State = "withdrawn"
			return result, nil
		}
	}
	lease = enrollmentLease{InstanceID: id, Ticket: ticket, State: "withdrawing"}
	if err = d.publishEnrollmentLease(ctx, lease); err != nil {
		return result, err
	}
	// Drain ingress too: a previously authorized proxy request must not survive
	// the native restart and arrive after the final account inventory.
	for _, name := range []string{"pds-ass-edge-" + id, "pds-ass-tenant-" + id} {
		if _, err = d.call(ctx, "POST", "/containers/"+name+"/stop?t=10", nil, nil); err != nil {
			return result, err
		}
	}
	for _, name := range []string{"pds-ass-tenant-" + id, "pds-ass-edge-" + id} {
		if _, err = d.call(ctx, "POST", "/containers/"+name+"/start", nil, nil); err != nil {
			return result, err
		}
	}
	if err = d.waitEnrollmentRuntime(ctx, id); err != nil {
		return result, err
	}
	var native nativeEnrollment
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, enrollmentHelperScript, []string{"withdraw"}, &native); err != nil {
		return result, err
	}
	if native.Count == 1 {
		account, err := enrollmentAccount(result, native)
		if err != nil {
			return result, err
		}
		lease.State = "complete"
		if err = d.publishEnrollmentLease(ctx, lease); err != nil {
			return result, err
		}
		return account, nil
	}
	if native.Count != 0 || native.Code != "" || native.DID != "" || native.Handle != "" || native.EmailConfirmed {
		return result, errors.New("invalid withdrawn account inventory")
	}
	lease.State = "withdrawn"
	if err = d.publishEnrollmentLease(ctx, lease); err != nil {
		return result, err
	}
	result.State = "withdrawn"
	return result, nil
}

func enrollmentAccount(result Enrollment, native nativeEnrollment) (Enrollment, error) {
	if native.Count != 1 || !accountDID.MatchString(native.DID) || native.Handle == "" || len(native.Handle) > 253 || native.Code != "" {
		return result, errors.New("invalid native account status")
	}
	result.State, result.DID, result.Handle, result.EmailConfirmed = "account_created", native.DID, native.Handle, native.EmailConfirmed
	if native.EmailConfirmed {
		result.State = "verified"
	}
	return result, nil
}

func (d *Docker) waitEnrollmentRuntime(ctx context.Context, id string) error {
	for attempt := 0; attempt < 50; attempt++ {
		status, err := d.status(ctx, id)
		if err == nil && status.Healthy {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errors.New("native PDS did not recover after enrollment drain")
}
