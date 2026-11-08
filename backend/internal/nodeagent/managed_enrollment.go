package nodeagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ManagedMail is host configuration for managed PDS verification mail, never API input.
type ManagedMail struct {
	EmailFrom string `json:"emailFrom"`
	SMTPURL   string `json:"smtpUrl"`
}

// EnableManagedMail gives generated managed instances an authenticated SMTPS sender.
// A retained managed runtime created without mail no longer matches policy and must be
// set aside (stopped, renamed) before reconciliation recreates it on its retained volumes.
func (d *Docker) EnableManagedMail(path string) error {
	if !d.managedEndpoints || d.managedMail != nil || !filepath.IsAbs(path) {
		return errors.New("managed mail requires managed endpoints and an absolute policy path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		return errors.New("private managed mail policy file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("managed mail policy unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy ManagedMail
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid managed mail policy")
	}
	if err := validateMail("managed mail", policy.EmailFrom, policy.SMTPURL); err != nil {
		return err
	}
	d.managedMail = &policy
	return nil
}

// EnableManagedEnrollment opens first-account leases for generated managed instances only.
// Each managed instance then runs the first-account enrollment guard; signup stays closed
// outside a lease. A retained closed edge must be set aside before reconciliation.
func (d *Docker) EnableManagedEnrollment() error {
	if !d.managedEndpoints || d.managedMail == nil {
		return errors.New("managed enrollment requires managed endpoints and managed mail")
	}
	d.managedEnrollment = true
	return nil
}

// ManagedEnrollment reports the immutable startup opt-in to the request fence.
func (d *Docker) ManagedEnrollment() bool { return d.managedEnrollment }
