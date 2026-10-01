package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalTLSIdentityPersistenceAndTrust(t *testing.T) {
	id := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	docker, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := docker.localCertificate(id)
	if err != nil {
		t.Fatal(err)
	}
	reconstructed, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	again, err := reconstructed.localCertificate(id)
	if err != nil || string(again) != string(certificate) {
		t.Fatalf("identity changed: %v", err)
	}
	identityPath := filepath.Join(directory, id+"-tls.pem")
	pair, err := tls.LoadX509KeyPair(identityPath, identityPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("public certificate invalid")
	}
	for _, entry := range []struct {
		name     string
		roots    *x509.CertPool
		hostname string
		success  bool
	}{
		{"fixture trust", roots, id + ".localhost", true},
		{"wrong hostname", roots, "another.localhost", false},
		{"missing fixture trust", x509.NewCertPool(), id + ".localhost", false},
	} {
		t.Run(entry.name, func(t *testing.T) {
			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: entry.roots, ServerName: entry.hostname}}}
			defer client.CloseIdleConnections()
			response, err := client.Get(server.URL)
			if response != nil {
				response.Body.Close()
			}
			if (err == nil) != entry.success {
				t.Fatalf("TLS result: %v", err)
			}
		})
	}
	if err = os.Chmod(identityPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.localCertificate(id); err == nil {
		t.Fatal("publicly readable private key accepted")
	}
	if err = os.Chmod(identityPath, 0600); err != nil {
		t.Fatal(err)
	}
	other := "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
	if err = os.Symlink(identityPath, filepath.Join(directory, other+"-tls.pem")); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.localCertificate(other); err == nil {
		t.Fatal("followed TLS identity symlink")
	}
	if docker.localIngressHealthy(context.Background(), other) {
		t.Fatal("unsafe TLS identity reported healthy")
	}
}
