package managedrouting

import (
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// WriteBundle creates an exclusive private bundle; it never replaces an existing inventory or certificate.
func WriteBundle(directory string, bundle Bundle) error {
	if bundle.Inventory.ExpiresAt == nil || !time.Now().Before(*bundle.Inventory.ExpiresAt) || bundle.Inventory.ExpiresAt.After(time.Now().Add(ReadbackLifetime+time.Minute)) {
		return ErrUnqualified
	}
	if bundle.Inventory.Version != 1 || bundle.Inventory.Generation < 1 || len(bundle.Inventory.Endpoints) == 0 || len(bundle.Inventory.Endpoints) > 100 || len(bundle.Inventory.Handles) != 0 || len(bundle.Certificates) != len(bundle.Inventory.Endpoints) {
		return ErrUnqualified
	}
	seen, ports := map[string]bool{}, map[int]bool{}
	for _, endpoint := range bundle.Inventory.Endpoints {
		host, err := managednamespace.Endpoint(endpoint.DeploymentID)
		if err != nil || host != endpoint.Hostname || seen[endpoint.DeploymentID] || ports[endpoint.ForwardPort] || endpoint.ForwardPort < 35000 || endpoint.ForwardPort > 35999 {
			return ErrUnqualified
		}
		seen[endpoint.DeploymentID] = true
		ports[endpoint.ForwardPort] = true
		if err := managededge.ValidateCertificate(endpoint.DeploymentID, bundle.Certificates[endpoint.DeploymentID], endpoint.CertificateSHA256, time.Now()); err != nil {
			return err
		}
	}
	data, err := inventoryBytes(bundle.Inventory)
	if err != nil {
		return err
	}
	forwarding, err := forwardingBytes(bundle, data, time.Now())
	if err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return errors.New("bundle directory must be new")
	}
	certificates := filepath.Join(directory, "certificates")
	if err := os.Mkdir(certificates, 0700); err != nil {
		return err
	}
	ids := make([]string, 0, len(bundle.Certificates))
	for id := range bundle.Certificates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	write := func(path string, data []byte) error {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := file.Write(data); err != nil {
			return err
		}
		return file.Sync()
	}
	for _, id := range ids {
		if err := write(filepath.Join(certificates, id+".crt"), bundle.Certificates[id]); err != nil {
			return err
		}
	}
	if err := write(filepath.Join(directory, "forwarding.json"), forwarding); err != nil {
		return err
	}
	// Publish the inventory last; a partial failed directory is retained for inspection.
	return write(filepath.Join(directory, "inventory.json"), data)
}
