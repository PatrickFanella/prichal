package nodeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPublicCertificateRefusesPrivateMaterialAndUnregisteredPaths(t *testing.T) {
	id := "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewDocker("/unused", directory)
	if err != nil {
		t.Fatal(err)
	}
	data, err := runtime.localCertificate(id)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	value := IngressCertificate{InstanceID: id, Endpoint: LocalEndpoint(id), Certificate: string(data), SHA256: hex.EncodeToString(sum[:])}
	if err := value.Validate(id); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"private data\n", "-----BEGIN PRIVATE KEY-----\nnot-public\n-----END PRIVATE KEY-----\n"} {
		bad := value
		bad.Certificate = prefix + bad.Certificate
		sum := sha256.Sum256([]byte(bad.Certificate))
		bad.SHA256 = hex.EncodeToString(sum[:])
		if err := bad.Validate(id); err == nil {
			t.Fatal("noncertificate data accepted")
		}
	}
	provider := &clientRuntime{certificateResult: &value}
	for _, path := range []string{"/v1/instances/invalid/certificate", "/v1/instances/" + id + "/certificate?path=/private/key", "/v1/instances/" + id + "/certificate/extra"} {
		w := httptest.NewRecorder()
		Handler(provider).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 || strings.Contains(w.Body.String(), "BEGIN CERTIFICATE") {
			t.Fatalf("unexpected path granted certificate: %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	Authenticate(Handler(provider)).ServeHTTP(w, httptest.NewRequest("GET", "/v1/instances/"+id+"/certificate", nil))
	if w.Code != 401 {
		t.Fatal("unauthenticated read accepted")
	}
	// Missing native resources must not create credentials or TLS identity during inspection.
	emptyDirectory := t.TempDir()
	if err := os.Chmod(emptyDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	empty, err := NewDocker("/unused", emptyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := empty.IngressCertificate(context.Background(), id); err == nil {
		t.Fatal("missing runtime supplied ingress identity")
	}
}
