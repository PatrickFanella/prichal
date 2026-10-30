package managedrouting

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

// Forward identifies the current authenticated native target, without host credentials or commands.
type Forward struct {
	DeploymentID        string    `json:"deploymentId"`
	NodeID              string    `json:"nodeId"`
	NodeKeySHA256       string    `json:"nodeKeySha256"`
	NativeEndpoint      string    `json:"nativeEndpoint"`
	ObservationSequence int64     `json:"observationSequence"`
	ObservedAt          time.Time `json:"observedAt"`
}

type forwardingEndpoint struct {
	Forward
	Hostname          string `json:"hostname"`
	ForwardPort       int    `json:"forwardPort"`
	CertificateSHA256 string `json:"certificateSha256"`
}

type forwardingManifest struct {
	Version         int                  `json:"version"`
	InventorySHA256 string               `json:"inventorySha256"`
	Generation      int64                `json:"generation"`
	ExpiresAt       time.Time            `json:"expiresAt"`
	Endpoints       []forwardingEndpoint `json:"endpoints"`
}

func inventoryBytes(inventory managededge.Inventory) ([]byte, error) {
	data, err := json.MarshalIndent(inventory, "", "  ")
	return append(data, '\n'), err
}

func forwardingBytes(bundle Bundle, inventory []byte, now time.Time) ([]byte, error) {
	if bundle.Inventory.ExpiresAt == nil || len(bundle.Forwards) != len(bundle.Inventory.Endpoints) {
		return nil, ErrUnqualified
	}
	byID := map[string]Forward{}
	targets := map[string]bool{}
	for _, forward := range bundle.Forwards {
		if _, err := managednamespace.Endpoint(forward.NodeID); err != nil {
			return nil, ErrUnqualified
		}
		pin, err := hex.DecodeString(forward.NodeKeySHA256)
		if err != nil || len(pin) != 32 || hex.EncodeToString(pin) != forward.NodeKeySHA256 || forward.NativeEndpoint == "" || forward.NativeEndpoint != nodeagent.LocalEndpoint(forward.DeploymentID) || forward.ObservationSequence < 1 || forward.ObservationSequence > bundle.Inventory.Generation || forward.ObservedAt.After(now.Add(time.Minute)) || !now.Before(forward.ObservedAt.Add(ReadbackLifetime)) || bundle.Inventory.ExpiresAt.After(forward.ObservedAt.Add(ReadbackLifetime)) {
			return nil, ErrUnqualified
		}
		// Distinct deployments on one node cannot share the derived loopback listener.
		endpoint, err := url.Parse(forward.NativeEndpoint)
		if err != nil {
			return nil, ErrUnqualified
		}
		target := forward.NodeID + ":" + endpoint.Port()
		if _, exists := byID[forward.DeploymentID]; exists || targets[target] {
			return nil, ErrUnqualified
		}
		byID[forward.DeploymentID], targets[target] = forward, true
	}
	sum := sha256.Sum256(inventory)
	manifest := forwardingManifest{Version: 1, InventorySHA256: hex.EncodeToString(sum[:]), Generation: bundle.Inventory.Generation, ExpiresAt: *bundle.Inventory.ExpiresAt, Endpoints: []forwardingEndpoint{}}
	for _, endpoint := range bundle.Inventory.Endpoints {
		forward, exists := byID[endpoint.DeploymentID]
		if !exists {
			return nil, ErrUnqualified
		}
		block, _ := pem.Decode(bundle.Certificates[endpoint.DeploymentID])
		if block == nil {
			return nil, ErrUnqualified
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || bundle.Inventory.ExpiresAt.After(certificate.NotAfter) {
			return nil, ErrUnqualified
		}
		manifest.Endpoints = append(manifest.Endpoints, forwardingEndpoint{Forward: forward, Hostname: endpoint.Hostname, ForwardPort: endpoint.ForwardPort, CertificateSHA256: endpoint.CertificateSHA256})
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	return append(data, '\n'), err
}
