package fleet

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
)

// Renew is private operator authority, not an HTTP enrollment route. It retains
// the registered public key and endpoint and never creates a bootstrap token.
func (s Store) Renew(ctx context.Context, nodeID string) ([]byte, error) {
	if s.CA == nil || s.Key == nil || !s.CA.IsCA || time.Until(s.CA.NotAfter) < time.Minute {
		return nil, errors.New("node signing authority unavailable")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var endpoint string
	var retained, fingerprint []byte
	err = tx.QueryRow(ctx, `SELECT agent_endpoint,certificate_pem,public_key_hash FROM hosting_nodes WHERE id=$1 AND state='active' FOR UPDATE`, nodeID).Scan(&endpoint, &retained, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(retained)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 || !localEndpoint(endpoint) {
		return nil, ErrUnauthorized
	}
	old, err := x509.ParseCertificate(block.Bytes)
	if err != nil || old.Subject.CommonName != "pds-ass-node-"+nodeID || old.CheckSignatureFrom(s.CA) != nil {
		return nil, ErrUnauthorized
	}
	parsed, _ := url.Parse(endpoint)
	if old.IsCA || len(old.ExtKeyUsage) != 1 || old.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth || old.VerifyHostname(parsed.Hostname()) != nil {
		return nil, ErrUnauthorized
	}
	public, ok := old.PublicKey.(*ecdsa.PublicKey)
	if !ok || public.Curve.Params().Name != "P-256" {
		return nil, ErrUnauthorized
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(der)
	if !bytes.Equal(hash[:], fingerprint) {
		return nil, ErrUnauthorized
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	expiry := time.Now().Add(24 * time.Hour)
	if expiry.After(s.CA.NotAfter) {
		expiry = s.CA.NotAfter
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: old.Subject.CommonName},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{parsed.Hostname()}
	}
	der, err = x509.CreateCertificate(rand.Reader, template, s.CA, public, s.Key)
	if err != nil {
		return nil, err
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if _, err = tx.Exec(ctx, `UPDATE hosting_nodes SET certificate_pem=$2 WHERE id=$1`, nodeID, certificate); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return certificate, nil
}
