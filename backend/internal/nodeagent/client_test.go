package nodeagent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type clientRuntime struct{ calls int }

func (r *clientRuntime) Ensure(_ context.Context, id string) (Instance, error) {
	r.calls++
	return Instance{ID: id, Endpoint: LocalEndpoint(id), State: "running", Healthy: true}, nil
}
func (r *clientRuntime) Capacity(context.Context) (Capacity, error) {
	return Capacity{MaximumSlots: 2, CPUs: 8, MemoryBytes: 8 << 30, InstanceIDs: []string{}}, nil
}
func TestMutualTLSRegistryPin(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "agent-ca"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	issue := func(cn string, usage x509.ExtKeyUsage) (tls.Certificate, []byte, []byte) {
		t.Helper()
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
		if err != nil {
			t.Fatal(err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		return pair, certPEM, keyPEM
	}
	serverCert, _, _ := issue("pds-ass-node-"+nodeID, x509.ExtKeyUsageServerAuth)
	_, workerPEM, workerKey := issue("pds-ass-worker", x509.ExtKeyUsageClientAuth)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	runtime := &clientRuntime{}
	server := httptest.NewUnstartedServer(Authenticate(Handler(runtime)))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}
	server.StartTLS()
	defer server.Close()
	directory := t.TempDir()
	for _, entry := range []struct {
		name string
		data []byte
	}{{"ca", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})}, {"worker", workerPEM}, {"key", workerKey}} {
		if err = os.WriteFile(filepath.Join(directory, entry.name), entry.data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	client, err := NewClient(filepath.Join(directory, "ca"), filepath.Join(directory, "worker"), filepath.Join(directory, "key"))
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(serverCert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(public)
	peer := Peer{NodeID: nodeID, Endpoint: server.URL, PublicKeyHash: pin[:]}
	if _, err = client.Ensure(context.Background(), peer, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Capacity(context.Background(), peer); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name string
		peer Peer
	}{{"wrong key", Peer{NodeID: nodeID, Endpoint: server.URL, PublicKeyHash: make([]byte, 32)}}, {"wrong node", Peer{NodeID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Endpoint: server.URL, PublicKeyHash: pin[:]}}} {
		t.Run(entry.name, func(t *testing.T) {
			if _, err := client.Ensure(context.Background(), entry.peer, nodeID); err == nil {
				t.Fatal("unregistered peer accepted")
			}
		})
	}
	if runtime.calls != 1 {
		t.Fatalf("invalid peer reached runtime: %d", runtime.calls)
	}
}
