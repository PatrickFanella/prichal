package managededge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const instance = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
const invitation = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const actor = "did:plc:abcdefghijklmnopqrstuvwx"

func fixture(t *testing.T) (Inventory, string, time.Time) {
	t.Helper()
	directory := t.TempDir()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{instance + ".localhost"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(directory, instance+".crt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	host, _ := managednamespace.Endpoint(instance)
	handle, _ := managednamespace.Handle(invitation)
	return Inventory{Version: 1, Endpoints: []Endpoint{{DeploymentID: instance, Hostname: host, ForwardPort: 35000, CertificateSHA256: hex.EncodeToString(hash[:])}}, Handles: []Handle{{InvitationID: invitation, DeploymentID: instance, Hostname: handle, DID: actor}}}, directory, now
}
func TestExactHostCandidatePinsUpstreamAndSeparatesHandleProof(t *testing.T) {
	inventory, directory, now := fixture(t)
	candidate, err := Render(inventory, directory, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"http://pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv", "https://172.20.0.1:35000", "tls_server_name " + instance + ".localhost", "tls_trust_pool file /etc/caddy/certs/prichal-managed/" + instance + ".crt", "http://at-aaaaaaaaaaaa4aaaaaaaaaaaaaaaaaaa.subcult.tv", "path /.well-known/atproto-did", `respond "` + actor + `" 200`, "method GET HEAD"} {
		if !strings.Contains(candidate, required) {
			t.Fatalf("missing %q", required)
		}
	}
	for _, forbidden := range []string{"*.", "tls_insecure_skip_verify", "import authelia", "on_demand", "ca.key", "worker.key"} {
		if strings.Contains(candidate, forbidden) {
			t.Fatalf("unsafe generated directive: %s", forbidden)
		}
	}
	if inventory.Endpoints[0].DeploymentID != instance {
		t.Fatal("inventory mutated")
	}
}
func TestInventoryCannotChooseOtherHostsPathsOrDuplicateAuthority(t *testing.T) {
	for name, mutate := range map[string]func(*Inventory){
		"management host":     func(i *Inventory) { i.Endpoints[0].Hostname = "prichal.subcult.tv" },
		"nested hostname":     func(i *Inventory) { i.Endpoints[0].Hostname = "alice.pds.subcult.tv" },
		"config injection":    func(i *Inventory) { i.Endpoints[0].Hostname = "evil\n}" },
		"path injection":      func(i *Inventory) { i.Endpoints[0].DeploymentID = "../../secret" },
		"unreserved port":     func(i *Inventory) { i.Endpoints[0].ForwardPort = 34945 },
		"certificate receipt": func(i *Inventory) { i.Endpoints[0].CertificateSHA256 = strings.Repeat("0", 64) },
		"duplicate endpoint":  func(i *Inventory) { i.Endpoints = append(i.Endpoints, i.Endpoints[0]) },
		"unknown placement":   func(i *Inventory) { i.Handles[0].DeploymentID = invitation },
		"arbitrary handle":    func(i *Inventory) { i.Handles[0].Hostname = "admin.subcult.tv" },
		"DID injection":       func(i *Inventory) { i.Handles[0].DID = "did:plc:abc\n}" },
		"duplicate handle":    func(i *Inventory) { i.Handles = append(i.Handles, i.Handles[0]) },
		"conflicting DID": func(i *Inventory) {
			h := i.Handles[0]
			h.InvitationID = "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"
			h.Hostname, _ = managednamespace.Handle(h.InvitationID)
			i.Handles = append(i.Handles, h)
		},
	} {
		t.Run(name, func(t *testing.T) {
			i, d, n := fixture(t)
			mutate(&i)
			if _, err := Render(i, d, n); err == nil {
				t.Fatal("invalid inventory rendered")
			}
		})
	}
}
func TestCertificateExpirySymlinkAndPrivateMaterialRefused(t *testing.T) {
	i, d, n := fixture(t)
	if _, err := Render(i, d, n.Add(2*time.Hour)); err == nil {
		t.Fatal("expired certificate accepted")
	}
	if _, err := Render(i, d, n.Add(-2*time.Hour)); err == nil {
		t.Fatal("future certificate accepted")
	}
	path := filepath.Join(d, instance+".crt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	renamed := path + ".retained"
	if err := os.Rename(path, renamed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(renamed, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(i, d, n); err == nil {
		t.Fatal("certificate symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("synthetic private material")})...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	i.Endpoints[0].CertificateSHA256 = hex.EncodeToString(sum[:])
	if _, err := Render(i, d, n); err == nil {
		t.Fatal("private PEM material accepted")
	}
}
func TestDecodeRefusesExtraAuthorityAndUnboundedInput(t *testing.T) {
	for _, value := range []string{`{"version":1,"command":"reload"}`, `{} {}`, `{"version":true}`, strings.Repeat(" ", 1<<20+1)} {
		if _, err := Decode(strings.NewReader(value)); err == nil {
			t.Fatal("invalid manifest decoded")
		}
	}
	if _, err := Decode(strings.NewReader(`{"version":1,"endpoints":[],"handles":[]}`)); err != nil {
		t.Fatal(err)
	}
}
