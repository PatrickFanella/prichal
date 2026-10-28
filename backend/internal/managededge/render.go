// Package managededge renders exact-host candidates from a private operator inventory.
// It neither publishes routes nor verifies account ownership or public readiness.
package managededge

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

type Inventory struct {
	Version    int        `json:"version"`
	Generation int64      `json:"generation,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	Endpoints  []Endpoint `json:"endpoints"`
	Handles    []Handle   `json:"handles"`
}
type Endpoint struct {
	DeploymentID      string `json:"deploymentId"`
	Hostname          string `json:"hostname"`
	ForwardPort       int    `json:"forwardPort"`
	CertificateSHA256 string `json:"certificateSha256"`
}
type Handle struct {
	InvitationID string `json:"invitationId"`
	DeploymentID string `json:"deploymentId"`
	Hostname     string `json:"hostname"`
	DID          string `json:"did"`
}

var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var plcDID = regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`)

const InventoryLifetime = 15 * time.Minute

const certificateMount = "/etc/caddy/certs/prichal-managed"

func Decode(reader io.Reader) (Inventory, error) {
	// Read a bounded manifest before decoding so trailing whitespace cannot bypass the limit.
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return Inventory{}, errors.New("managed edge inventory exceeds 1 MiB or is unreadable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var inventory Inventory
	if decoder.Decode(&inventory) != nil || decoder.Decode(new(any)) != io.EOF {
		return Inventory{}, errors.New("invalid managed edge inventory")
	}
	return inventory, nil
}

// Render verifies each public certificate against its inventory digest and private ingress name.
// The caller must supply an operator-authorized inventory with previously verified handle bindings.
func Render(inventory Inventory, certificateDirectory string, now time.Time) (string, error) {
	return render(inventory, certificateDirectory, now, certificateMount)
}

// RenderRevision uses an immutable, UUID-scoped public trust directory for publication/rollback.
func RenderRevision(inventory Inventory, certificateDirectory string, now time.Time, revision string) (string, error) {
	if _, err := managednamespace.Endpoint(revision); err != nil || inventory.Generation <= 0 {
		return "", errors.New("publication requires a canonical revision and authoritative inventory")
	}
	return render(inventory, certificateDirectory, now, certificateMount+"/"+revision)
}

func render(inventory Inventory, certificateDirectory string, now time.Time, mount string) (string, error) {
	if inventory.Generation < 0 || inventory.Generation > 0 && (inventory.ExpiresAt == nil || !now.Before(*inventory.ExpiresAt) || inventory.ExpiresAt.After(now.Add(InventoryLifetime+time.Minute))) {
		return "", errors.New("authoritative inventory is missing a current bounded deadline")
	}
	if inventory.Version != 1 || len(inventory.Endpoints) == 0 || len(inventory.Endpoints) > 100 || len(inventory.Handles) > 2500 || !filepath.IsAbs(certificateDirectory) {
		return "", errors.New("unsupported managed edge inventory scope")
	}
	info, err := os.Lstat(certificateDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("certificate directory must be a real directory")
	}
	endpoints := append([]Endpoint(nil), inventory.Endpoints...)
	handles := append([]Handle(nil), inventory.Handles...)
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].DeploymentID < endpoints[j].DeploymentID })
	sort.Slice(handles, func(i, j int) bool { return handles[i].InvitationID < handles[j].InvitationID })
	ids, hosts, ports, dids := map[string]bool{}, map[string]bool{}, map[int]bool{}, map[string]bool{}
	for _, endpoint := range endpoints {
		hostname, err := managednamespace.Endpoint(endpoint.DeploymentID)
		if err != nil || endpoint.Hostname != hostname || hosts[hostname] || ids[endpoint.DeploymentID] || endpoint.ForwardPort < 35000 || endpoint.ForwardPort > 35999 || ports[endpoint.ForwardPort] || !digest.MatchString(endpoint.CertificateSHA256) {
			return "", errors.New("conflicting or invalid managed endpoint")
		}
		ids[endpoint.DeploymentID] = true
		hosts[hostname] = true
		ports[endpoint.ForwardPort] = true
		path := filepath.Join(certificateDirectory, endpoint.DeploymentID+".crt")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
			return "", errors.New("public ingress certificate must be a bounded regular file")
		}
		file, err := os.Open(path)
		if err != nil {
			return "", errors.New("public ingress certificate unavailable")
		}
		opened, statErr := file.Stat()
		if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			file.Close()
			return "", errors.New("public ingress certificate changed during inspection")
		}
		data, err := io.ReadAll(io.LimitReader(file, 16385))
		file.Close()
		if err != nil || len(data) > 16384 {
			return "", errors.New("public ingress certificate unavailable or oversized")
		}
		if err := ValidateCertificate(endpoint.DeploymentID, data, endpoint.CertificateSHA256, now); err != nil {
			return "", err
		}
	}
	for _, handle := range handles {
		hostname, err := managednamespace.Handle(handle.InvitationID)
		if err != nil || hostname != handle.Hostname || hosts[hostname] || !ids[handle.DeploymentID] || !plcDID.MatchString(handle.DID) || dids[handle.DID] {
			return "", errors.New("conflicting or invalid verified handle binding")
		}
		hosts[hostname] = true
		dids[handle.DID] = true
	}
	var out strings.Builder
	out.WriteString("# Managed Prichal candidate; exact inventory hosts only. Cloudflare terminates public TLS.\n")
	for _, endpoint := range endpoints {
		fmt.Fprintf(&out, `# Public certificate SHA256: %s
http://%s {
 reverse_proxy https://172.20.0.1:%d {
  header_up Host %s
  header_up X-Forwarded-Proto https
  transport http {
   tls_trust_pool file %s/%s.crt
   tls_server_name %s.localhost
  }
 }
}
`, endpoint.CertificateSHA256, endpoint.Hostname, endpoint.ForwardPort, endpoint.Hostname, mount, endpoint.DeploymentID, endpoint.DeploymentID)
	}
	for _, handle := range handles {
		fmt.Fprintf(&out, `http://%s {
 @proof {
  method GET HEAD
  path /.well-known/atproto-did
 }
 handle @proof {
  header Content-Type text/plain
  header Cache-Control "public, max-age=60"
  respond "%s" 200
 }
 handle {
  respond "Unknown handle path" 404
 }
}
`, handle.Hostname, handle.DID)
	}
	return out.String(), nil
}

// ValidateCertificate accepts only the bounded public leaf for the retained private ingress name.
func ValidateCertificate(id string, data []byte, expectedSHA256 string, now time.Time) error {
	if _, err := managednamespace.Endpoint(id); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 16384 || !digest.MatchString(expectedSHA256) {
		return errors.New("invalid public certificate scope")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expectedSHA256 {
		return errors.New("public ingress certificate differs from inventory")
	}
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN CERTIFICATE-----")) {
		return errors.New("only public certificate PEM is permitted")
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return errors.New("only one public certificate is permitted")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.IsCA || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || cert.VerifyHostname(id+".localhost") != nil {
		return errors.New("public ingress certificate identity or validity differs")
	}
	return nil
}
