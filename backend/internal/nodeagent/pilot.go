package nodeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PilotPolicy is host configuration, never configuration supplied by an API caller.
// The first pilot exposes reconciliation and inspection only; account creation stays closed.
type PilotPolicy struct {
	InstanceID string `json:"instanceId"`
	Hostname   string `json:"hostname"`
	EmailFrom  string `json:"emailFrom"`
	SMTPURL    string `json:"smtpUrl"`
}

const pilotReceipt = "pilot-policy.sha256"

var pilotHostname = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func (p PilotPolicy) validate() error {
	if !instanceID.MatchString(p.InstanceID) || !pilotHostname.MatchString(p.Hostname) || len(p.Hostname) > 253 || net.ParseIP(p.Hostname) != nil || p.Hostname == "pds.subcult.tv" {
		return errors.New("pilot requires a dedicated hostname and instance UUID")
	}
	for _, suffix := range []string{".localhost", ".local", ".test", ".invalid", ".example"} {
		if strings.HasSuffix(p.Hostname, suffix) {
			return errors.New("pilot requires a public hostname")
		}
	}
	address, err := mail.ParseAddress(p.EmailFrom)
	if err != nil || address.Address != p.EmailFrom || strings.ContainsAny(p.EmailFrom, "\r\n") {
		return errors.New("pilot requires a plain sender email address")
	}
	u, err := url.Parse(p.SMTPURL)
	if err != nil || u.Scheme != "smtps" || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Fragment != "" || u.RawQuery != "" || u.Path != "" || strings.ContainsAny(p.SMTPURL, "\r\n") {
		return errors.New("pilot requires authenticated SMTP over TLS without URL options")
	}
	password, ok := u.User.Password()
	if !ok || password == "" {
		return errors.New("pilot requires SMTP credentials")
	}
	if port := u.Port(); port != "" && port != "465" {
		return errors.New("pilot SMTP must use implicit TLS port 465")
	}
	return nil
}

// EnablePilot binds a fresh private state directory to an immutable operator policy.
// A changed policy or retained local namespace requires explicit recovery, never adoption.
func (d *Docker) EnablePilot(path string) error {
	if d.localIdentity || d.pilot != nil || !filepath.IsAbs(path) {
		return errors.New("pilot cannot share local identity configuration")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		return errors.New("private pilot policy file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("pilot policy unavailable")
	}
	var policy PilotPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid pilot policy")
	}
	if err := policy.validate(); err != nil {
		return err
	}
	canonical, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(canonical)
	expected := hex.EncodeToString(digest[:]) + "\n"
	receipt := filepath.Join(d.directory, pilotReceipt)
	check := func() error {
		info, err := os.Lstat(receipt)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("unsafe pilot policy receipt")
		}
		retained, err := os.ReadFile(receipt)
		if err != nil || string(retained) != expected {
			return errors.New("pilot policy differs from retained identity")
		}
		return nil
	}
	if err := check(); errors.Is(err, os.ErrNotExist) {
		entries, err := os.ReadDir(d.directory)
		if err != nil || len(entries) != 0 {
			return errors.New("pilot requires fresh agent state")
		}
		file, err := os.CreateTemp(d.directory, ".pilot-policy-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		if _, err = file.WriteString(expected); err != nil {
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
		if err = os.Link(file.Name(), receipt); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		directory, err := os.Open(d.directory)
		if err != nil {
			return err
		}
		err = directory.Sync()
		directory.Close()
		if err != nil {
			return err
		}
		if err = check(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	d.pilot = &policy
	d.maximumSlots = 1
	return nil
}

func (d *Docker) allowedInstance(id string) bool {
	if !instanceID.MatchString(id) {
		return false
	}
	if d.pilot != nil {
		return !d.localIdentity && id == d.pilot.InstanceID
	}
	_, err := os.Lstat(filepath.Join(d.directory, pilotReceipt))
	return errors.Is(err, os.ErrNotExist)
}

func (d *Docker) endpoint(id string) string {
	if d.pilot != nil {
		return "https://" + d.pilot.Hostname
	}
	return LocalEndpoint(id)
}

// ValidateLocalState prevents local handlers from operating on retained pilot state.
func (d *Docker) ValidateLocalState() error {
	_, err := os.Lstat(filepath.Join(d.directory, pilotReceipt))
	if !errors.Is(err, os.ErrNotExist) {
		return errors.New("local agent cannot use pilot state")
	}
	return nil
}

func (d *Docker) fenceFreshPilot(ctx context.Context, id string) error {
	if d.pilot == nil {
		return nil
	}
	if _, err := d.readCredentials(id); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	name := "pds-ass-tenant-" + id
	for _, path := range []string{"/volumes/" + name + "-data", "/volumes/" + name + "-blobs", "/networks/" + name + "-network", "/containers/pds-ass-edge-" + id + "/json"} {
		status, err := d.call(ctx, "GET", path, nil, nil)
		if status != http.StatusNotFound || err == nil {
			return errors.New("fresh pilot namespace contains retained resources")
		}
	}
	return nil
}

func (d *Docker) validatePilotNetwork(ctx context.Context, id string) error {
	if d.pilot == nil {
		return nil
	}
	var network struct {
		Labels   map[string]string
		Internal bool
		Driver   string
	}
	if _, err := d.call(ctx, "GET", "/networks/pds-ass-tenant-"+id+"-network", nil, &network); err != nil {
		return err
	}
	if network.Labels[label] != id || network.Internal || network.Driver != "bridge" {
		return errors.New("pilot network differs from outbound bridge policy")
	}
	return nil
}

// PilotHandler deliberately omits account, enrollment, export and lifecycle mutations.
// The agent still requires Authenticate and mutual TLS outside this handler.
func PilotHandler(runtime *Docker) http.Handler {
	next := Handler(runtime)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if runtime.pilot == nil || r.URL.RawPath != "" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		path := "/v1/instances/" + runtime.pilot.InstanceID
		if (r.Method == "GET" || r.Method == "PUT") && r.URL.Path == path {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}
