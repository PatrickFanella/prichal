package fleet

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestEnrollment(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Pool: pool, CA: ca, Key: key}
	bootstrap, err := store.Issue(ctx, "https://127.0.0.1:9091", 2)
	if err != nil {
		t.Fatal(err)
	}
	nodeKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "caller-cannot-select-worker-role"}}, nodeKey)
	if err != nil {
		t.Fatal(err)
	}
	csr := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	if _, err = store.Enroll(ctx, bootstrap.NodeID, "unknown", csr); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err = store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, []byte("invalid csr")); err == nil {
		t.Fatal("invalid CSR accepted")
	}
	certPEM, err := store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, csr)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "pds-ass-node-"+bootstrap.NodeID || cert.NotAfter.After(ca.NotAfter) || len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatal("incorrect certificate role or expiry")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, csr); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("replayed enrollment: %v", err)
	}
	renewedPEM, err := store.Renew(ctx, bootstrap.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	renewedBlock, _ := pem.Decode(renewedPEM)
	renewed, err := x509.ParseCertificate(renewedBlock.Bytes)
	if err != nil || !renewed.PublicKey.(*ecdsa.PublicKey).Equal(nodeKey.Public()) || renewed.SerialNumber.Cmp(cert.SerialNumber) == 0 || renewed.NotAfter.After(ca.NotAfter) {
		t.Fatal("renewal changed identity or exceeded authority")
	}
	if _, err = renewed.Verify(x509.VerifyOptions{Roots: roots, DNSName: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, csr); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("renewal reopened bootstrap")
	}
	if _, err = store.Renew(ctx, "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unknown node renewed")
	}
	if _, err = pool.Exec(ctx, `UPDATE hosting_nodes SET public_key_hash=decode(repeat('00',32),'hex') WHERE id=$1`, bootstrap.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Renew(ctx, bootstrap.NodeID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("registry key drift accepted")
	}
	expired, err := store.Issue(ctx, "https://127.0.0.1:9093", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE node_bootstrap_tokens SET expires_at=now()-interval '1 minute' WHERE node_id=$1`, expired.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Enroll(ctx, expired.NodeID, expired.Token, csr); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired enrollment: %v", err)
	}
}
