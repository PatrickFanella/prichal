package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

const managedIdentitySuffix = "-managed-identity.json"

// ManagedProvisioningOnly closes unsupported mutation routes for this staging opt-in.
func (d *Docker) ManagedProvisioningOnly() bool { return d.managedEndpoints }

type managedIdentity struct {
	Version    int    `json:"version"`
	InstanceID string `json:"instanceId"`
	Hostname   string `json:"hostname"`
}

// EnableManagedEndpoints permits fresh generated identities alongside retained private fixtures.
// It must be called before serving requests. It does not adopt or rename any namespace.
func (d *Docker) EnableManagedEndpoints() error {
	if d.localIdentity || d.pilot != nil {
		return errors.New("managed endpoints cannot share pilot or local protocol identity")
	}
	if _, err := os.Lstat(filepath.Join(d.directory, pilotReceipt)); !errors.Is(err, os.ErrNotExist) {
		return errors.New("managed endpoints cannot use retained pilot state")
	}
	d.managedEndpoints = true
	if err := d.validateManagedState(); err != nil {
		d.managedEndpoints = false
		return err
	}
	return nil
}

// ManagedIdentityRetained fences generic mutation routes even if a receipt is malformed.
func (d *Docker) ManagedIdentityRetained(id string) bool {
	if !instanceID.MatchString(id) {
		return false
	}
	_, err := os.Lstat(filepath.Join(d.directory, id+managedIdentitySuffix))
	return !errors.Is(err, os.ErrNotExist)
}

func (d *Docker) readManagedIdentity(id string) (managedIdentity, error) {
	host, err := managednamespace.Endpoint(id)
	if err != nil {
		return managedIdentity{}, err
	}
	path := filepath.Join(d.directory, id+managedIdentitySuffix)
	info, err := os.Lstat(path)
	if err != nil {
		return managedIdentity{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1024 {
		return managedIdentity{}, errors.New("unsafe managed identity receipt")
	}
	file, err := os.Open(path)
	if err != nil {
		return managedIdentity{}, errors.New("managed identity receipt unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return managedIdentity{}, errors.New("managed identity receipt changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(data) > 1024 {
		return managedIdentity{}, errors.New("managed identity receipt unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value managedIdentity
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Version != 1 || value.InstanceID != id || value.Hostname != host {
		return managedIdentity{}, errors.New("managed identity differs from generated policy")
	}
	return value, nil
}

func (d *Docker) managedRuntime(id string) bool {
	if !d.managedEndpoints || d.localIdentity || d.pilot != nil {
		return false
	}
	_, err := d.readManagedIdentity(id)
	return err == nil
}

func (d *Docker) publicRuntime(id string) bool { return d.pilot != nil || d.managedRuntime(id) }

func (d *Docker) tenantNetwork(id string) string {
	if d.managedRuntime(id) {
		return "pds-ass-managed-tenant-" + id + "-network"
	}
	return "pds-ass-tenant-" + id + "-network"
}
func (d *Docker) ingressDirectory(id string) string {
	if d.managedRuntime(id) {
		return filepath.Join(d.directory, id+"-managed-edge")
	}
	return filepath.Join(d.directory, id+"-edge")
}

func (d *Docker) validateManagedState() error {
	entries, err := os.ReadDir(d.directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), managedIdentitySuffix) {
			continue
		}
		if !d.managedEndpoints {
			return errors.New("retained managed identity requires managed runtime opt-in")
		}
		if _, err := d.readManagedIdentity(strings.TrimSuffix(entry.Name(), managedIdentitySuffix)); err != nil {
			return err
		}
	}
	return nil
}

// EnsureManaged accepts only a UUID; the public name, images and paths are fixed policy.
func (d *Docker) EnsureManaged(ctx context.Context, id string) (Instance, error) {
	host, err := managednamespace.Endpoint(id)
	if err != nil || !d.managedEndpoints || d.localIdentity || d.pilot != nil {
		return Instance{}, errors.New("managed reconciliation is outside agent policy")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if _, err := d.readManagedIdentity(id); errors.Is(err, os.ErrNotExist) {
		// A missing receipt must never turn retained private credentials/storage into public identity.
		entries, err := os.ReadDir(d.directory)
		if err != nil {
			return Instance{}, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), id) {
				return Instance{}, errors.New("managed identity requires a fresh native namespace")
			}
		}
		if err := d.fenceFreshPublicNamespace(ctx, id); err != nil {
			return Instance{}, err
		}
		data, err := json.Marshal(managedIdentity{Version: 1, InstanceID: id, Hostname: host})
		if err != nil {
			return Instance{}, err
		}
		file, err := os.CreateTemp(d.directory, ".managed-identity-")
		if err != nil {
			return Instance{}, err
		}
		defer os.Remove(file.Name())
		if _, err := file.Write(append(data, '\n')); err != nil {
			file.Close()
			return Instance{}, err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return Instance{}, err
		}
		if err := file.Close(); err != nil {
			return Instance{}, err
		}
		if err := os.Link(file.Name(), filepath.Join(d.directory, id+managedIdentitySuffix)); err != nil && !errors.Is(err, os.ErrExist) {
			return Instance{}, err
		}
		directory, err := os.Open(d.directory)
		if err != nil {
			return Instance{}, err
		}
		err = directory.Sync()
		directory.Close()
		if err != nil {
			return Instance{}, err
		}
	} else if err != nil {
		return Instance{}, err
	}
	if _, err := d.readManagedIdentity(id); err != nil {
		return Instance{}, err
	}
	return d.ensure(ctx, id)
}

func (d *Docker) fenceFreshPublicNamespace(ctx context.Context, id string) error {
	name := "pds-ass-tenant-" + id
	for _, path := range []string{"/containers/" + name + "/json", "/containers/pds-ass-edge-" + id + "/json", "/containers/pds-ass-enrollment-" + id + "/json", "/volumes/" + name + "-data", "/volumes/" + name + "-blobs", "/networks/" + name + "-network", "/networks/pds-ass-managed-tenant-" + id + "-network"} {
		status, err := d.call(ctx, "GET", path, nil, nil)
		if status != http.StatusNotFound || err == nil {
			return errors.New("fresh public namespace contains retained resources or is unavailable")
		}
	}
	return nil
}
