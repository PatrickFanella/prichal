package nodeagent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// rewriteLocalIdentity replaces the retained certificate while keeping its private key.
func rewriteLocalIdentity(t *testing.T, path string, mutate func(*x509.Certificate)) *ecdsa.PrivateKey {
	t.Helper()
	pair, err := tls.LoadX509KeyPair(path, path)
	if err != nil {
		t.Fatal(err)
	}
	key := pair.PrivateKey.(*ecdsa.PrivateKey)
	template, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	mutate(template)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	data := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestLocalTLSIdentityLifetimeAndExpiredRenewal(t *testing.T) {
	id := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	docker, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	first, err := docker.localCertificate(id)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(first)
	issued, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if lifetime := issued.NotAfter.Sub(issued.NotBefore); lifetime < LocalTLSLifetime || lifetime > LocalTLSLifetime+time.Hour {
		t.Fatalf("local TLS lifetime %s", lifetime)
	}
	path := filepath.Join(directory, id+"-tls.pem")

	// The October 30 staging identity: issued for seven days and now past NotAfter.
	key := rewriteLocalIdentity(t, path, func(c *x509.Certificate) {
		c.NotBefore = time.Now().Add(-8 * 24 * time.Hour)
		c.NotAfter = time.Now().Add(-24 * time.Hour)
	})
	if _, err = docker.readLocalCertificate(id); err == nil {
		t.Fatal("expired identity served")
	}
	renewed, err := docker.localCertificate(id)
	if err != nil {
		t.Fatalf("expired identity not renewed: %v", err)
	}
	if bytes.Equal(renewed, first) {
		t.Fatal("renewal returned the previous certificate")
	}
	pair, err := tls.LoadX509KeyPair(path, path)
	if err != nil {
		t.Fatal(err)
	}
	if !key.Equal(pair.PrivateKey) {
		t.Fatal("renewal replaced the retained key")
	}
	if info, err := os.Lstat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("renewed identity mode: %v", err)
	}
	again, err := docker.localCertificate(id)
	if err != nil || !bytes.Equal(again, renewed) {
		t.Fatalf("current identity reissued: %v", err)
	}

	// A CA-signed ingress leaf (the public pilot) is renewed by its operator, never self-signed here.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "pilot CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: id + ".localhost"}, DNSNames: []string{id + ".localhost"}, NotBefore: time.Now().Add(-8 * 24 * time.Hour), NotAfter: time.Now().Add(-time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pilot := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...)
	if err = os.WriteFile(path, pilot, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.localCertificate(id); err == nil {
		t.Fatal("expired CA-signed ingress leaf was self-signed")
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, pilot) {
		t.Fatal("expired CA-signed ingress leaf was replaced")
	}

	for name, mutate := range map[string]func(*x509.Certificate){
		"expired other hostname": func(c *x509.Certificate) {
			c.DNSNames = []string{"another.localhost"}
			c.NotBefore = time.Now().Add(-8 * 24 * time.Hour)
			c.NotAfter = time.Now().Add(-24 * time.Hour)
		},
		"overlong lifetime": func(c *x509.Certificate) {
			c.NotAfter = c.NotBefore.Add(LocalTLSLifetime + 48*time.Hour)
		},
		"not yet valid": func(c *x509.Certificate) {
			c.NotBefore = time.Now().Add(time.Hour)
			c.NotAfter = time.Now().Add(48 * time.Hour)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rewriteLocalIdentity(t, path, mutate)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = docker.localCertificate(id); err == nil {
				t.Fatal("defective identity accepted or reissued")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("defective identity was replaced")
			}
		})
	}
}
