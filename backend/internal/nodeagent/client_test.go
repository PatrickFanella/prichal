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

type clientRuntime struct {
	calls, withdrawals int
	withdrawalResult   *Enrollment
	exitResult         *MemberExit
	exits              int
	exitInspections    int
}

func (r *clientRuntime) MemberExitStatus(_ context.Context, id, ticket string) (MemberExit, error) {
	r.exitInspections++
	if r.exitResult != nil {
		return *r.exitResult, nil
	}
	now := time.Now()
	return MemberExit{InstanceID: id, Ticket: ticket, DID: exitDID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()}, nil
}

func (r *clientRuntime) MemberExit(_ context.Context, id, ticket string) (MemberExit, error) {
	r.exits++
	if r.exitResult != nil {
		return *r.exitResult, nil
	}
	now := time.Now()
	return MemberExit{InstanceID: id, Ticket: ticket, DID: exitDID, State: "migration_window", RequestedAt: now.UnixMilli(), DeactivateAfter: now.Add(MemberMigrationWindow).UnixMilli()}, nil
}

func (r *clientRuntime) WithdrawEnrollment(_ context.Context, id, ticket string) (Enrollment, error) {
	r.withdrawals++
	if r.withdrawalResult != nil {
		return *r.withdrawalResult, nil
	}
	return Enrollment{InstanceID: id, Ticket: ticket, State: "withdrawn"}, nil
}

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
	result, exitErr := client.MemberExit(context.Background(), peer, nodeID, nodeID)
	if exitErr != nil || result.State != "migration_window" {
		t.Fatalf("authenticated member exit: %+v %v", result, exitErr)
	}
	if inspected, err := client.MemberExitStatus(context.Background(), peer, nodeID, nodeID); err != nil || inspected.State != "migration_window" || runtime.exits != 1 || runtime.exitInspections != 1 {
		t.Fatalf("read-only authenticated member exit: %+v %v", inspected, err)
	}
	for _, mutate := range []func(*MemberExit){
		func(m *MemberExit) { m.Ticket = exitTicket },
		func(m *MemberExit) { m.InstanceID = exitTicket },
		func(m *MemberExit) { m.DID = "invalid" },
		func(m *MemberExit) { m.DeactivateAfter-- },
		func(m *MemberExit) { m.State = "deactivated" },
	} {
		bad := result
		mutate(&bad)
		runtime.exitResult = &bad
		if _, err := client.MemberExit(context.Background(), peer, nodeID, nodeID); err == nil {
			t.Fatal("unsafe member exit response accepted")
		}
		if _, err := client.MemberExitStatus(context.Background(), peer, nodeID, nodeID); err == nil {
			t.Fatal("unsafe member exit inspection accepted")
		}
	}
	runtime.exitResult = nil
	exitCalls := runtime.exits
	withdrawn, err := client.WithdrawEnrollment(context.Background(), peer, nodeID, nodeID)
	if err != nil || withdrawn.State != "withdrawn" {
		t.Fatalf("authenticated withdrawal: %+v %v", withdrawn, err)
	}
	for _, invalid := range []Enrollment{
		{InstanceID: nodeID, Ticket: nodeID, State: "invitation_ready", Code: "fixture", ExpiresAt: time.Now().Add(time.Minute).UnixMilli()},
		{InstanceID: nodeID, Ticket: nodeID, State: "withdrawn", Code: "fixture"},
		{InstanceID: nodeID, Ticket: nodeID, State: "withdrawn", ExpiresAt: 1},
		{InstanceID: nodeID, Ticket: nodeID, State: "withdrawn", DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"},
		{InstanceID: nodeID, Ticket: nodeID, State: "verified", DID: "did:plc:AAAAAAAAAAAAAAAAAAAAAAAA", Handle: "fixture.test", EmailConfirmed: true},
		{InstanceID: nodeID, Ticket: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", State: "withdrawn"},
	} {
		runtime.withdrawalResult = &invalid
		if _, err = client.WithdrawEnrollment(context.Background(), peer, nodeID, nodeID); err == nil {
			t.Fatal("invalid withdrawal response accepted")
		}
	}
	runtime.withdrawalResult = nil
	withdrawalCalls := runtime.withdrawals
	if _, err = client.Ensure(context.Background(), peer, nodeID); err != nil {
		t.Fatal(err)
	}
	if usage, usageErr := client.Usage(context.Background(), peer, nodeID); usageErr != nil || usage.Accounts != 1 {
		t.Fatal("registered usage failed")
	}
	if _, err = client.Account(context.Background(), peer, nodeID, ServiceRequest{Ticket: nodeID, Generation: 1, State: "suspended"}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Service(context.Background(), peer, nodeID, ServiceRequest{Ticket: nodeID, Generation: 1, State: "suspended"}); err != nil {
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
			if _, err := client.MemberExit(context.Background(), entry.peer, nodeID, nodeID); err == nil {
				t.Fatal("unregistered exit peer accepted")
			}
			if _, err := client.MemberExitStatus(context.Background(), entry.peer, nodeID, nodeID); err == nil {
				t.Fatal("unregistered exit inspection peer accepted")
			}
			if _, err := client.WithdrawEnrollment(context.Background(), entry.peer, nodeID, nodeID); err == nil {
				t.Fatal("unregistered withdrawal peer accepted")
			}
			if _, err := client.Ensure(context.Background(), entry.peer, nodeID); err == nil {
				t.Fatal("unregistered peer accepted")
			}
		})
	}
	if runtime.exits != exitCalls {
		t.Fatal("unregistered peer reached member exit runtime")
	}
	if runtime.calls != 1 {
		t.Fatalf("invalid peer reached runtime: %d", runtime.calls)
	}
	if runtime.withdrawals != withdrawalCalls {
		t.Fatal("unregistered peer reached withdrawal runtime")
	}
	// A running worker must read renewed files and must not fall back after corruption.
	workerPath := filepath.Join(directory, "worker")
	if err = os.WriteFile(workerPath, []byte("invalid replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Capacity(context.Background(), peer); err == nil {
		t.Fatal("startup certificate snapshot used after invalid replacement")
	}
	if err = os.WriteFile(workerPath, workerPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Capacity(context.Background(), peer); err != nil {
		t.Fatal("retained worker did not recover after certificate publication", err)
	}
}

func (r *clientRuntime) Usage(_ context.Context, id string) (Usage, error) {
	return Usage{InstanceID: id, DataBytes: 1000, BlobBytes: 68, DataFiles: 20, BlobFiles: 1, Accounts: 1, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}, nil
}

func (r *clientRuntime) Service(_ context.Context, id string, request ServiceRequest) (ServiceResult, error) {
	return ServiceResult{InstanceID: id, ServiceRequest: request}, nil
}

func (r *clientRuntime) Account(_ context.Context, id string, request ServiceRequest) (AccountResult, error) {
	return AccountResult{ServiceResult: ServiceResult{InstanceID: id, ServiceRequest: request}, DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"}, nil
}
