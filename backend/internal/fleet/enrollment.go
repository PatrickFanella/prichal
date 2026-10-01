// Package fleet owns node identity and placement in management PostgreSQL.
package fleet

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUnauthorized covers unknown, expired and consumed enrollment credentials.
var ErrUnauthorized = errors.New("node enrollment credential rejected")

// Store's signer belongs only in the private enrollment service, never the public API.
type Store struct {
	Pool *pgxpool.Pool
	CA   *x509.Certificate
	Key  crypto.Signer
}

// Bootstrap is returned once to the operator and must be written to a private file.
type Bootstrap struct {
	NodeID   string `json:"nodeId"`
	Token    string `json:"token"`
	Endpoint string `json:"endpoint"`
}

func localEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

// Issue creates a bounded, local qualification node and a 15-minute one-time credential.
func (s Store) Issue(ctx context.Context, endpoint string, slots int) (Bootstrap, error) {
	if !localEndpoint(endpoint) || slots < 1 || slots > 1000 {
		return Bootstrap{}, errors.New("invalid local node endpoint or capacity")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Bootstrap{}, err
	}
	token := hex.EncodeToString(random[:])
	hash := sha256.Sum256([]byte(token))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	defer tx.Rollback(ctx)
	bootstrap := Bootstrap{Token: token, Endpoint: endpoint}
	if err = tx.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots) VALUES('local',$1,$2,$2) RETURNING id`, endpoint, slots).Scan(&bootstrap.NodeID); err != nil {
		return Bootstrap{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO node_bootstrap_tokens(node_id,token_hash,expires_at) VALUES($1,$2,now()+interval '15 minutes')`, bootstrap.NodeID, hash[:]); err != nil {
		return Bootstrap{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Bootstrap{}, err
	}
	return bootstrap, nil
}

// Enroll validates the CSR and consumes the bootstrap credential with node activation.
func (s Store) Enroll(ctx context.Context, nodeID, token string, csrPEM []byte) ([]byte, error) {
	if len(token) != 64 {
		return nil, ErrUnauthorized
	}
	block, rest := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("invalid node CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		return nil, errors.New("invalid node CSR signature")
	}
	publicKey, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve.Params().Name != "P-256" {
		return nil, errors.New("node CSR requires ECDSA P-256")
	}
	if s.CA == nil || s.Key == nil || !s.CA.IsCA || time.Now().After(s.CA.NotAfter) {
		return nil, errors.New("node signing authority unavailable")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	hash := sha256.Sum256([]byte(token))
	var endpoint string
	err = tx.QueryRow(ctx, `SELECT n.agent_endpoint FROM hosting_nodes n JOIN node_bootstrap_tokens b ON b.node_id=n.id WHERE n.id=$1 AND b.token_hash=$2 AND b.expires_at>now() AND b.consumed_at IS NULL AND n.state='pending' FOR UPDATE OF n,b`, nodeID, hash[:]).Scan(&endpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	expiry := time.Now().Add(24 * time.Hour)
	if expiry.After(s.CA.NotAfter) {
		expiry = s.CA.NotAfter
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "pds-ass-node-" + nodeID}, NotBefore: time.Now().Add(-time.Minute), NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	parsed, _ := url.Parse(endpoint)
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{parsed.Hostname()}
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, s.CA, csr.PublicKey, s.Key)
	if err != nil {
		return nil, err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return nil, err
	}
	fingerprint := sha256.Sum256(publicDER)
	if _, err = tx.Exec(ctx, `UPDATE hosting_nodes SET state='active',public_key_hash=$2,certificate_pem=$3,enrolled_at=now() WHERE id=$1`, nodeID, fingerprint[:], pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE node_bootstrap_tokens SET consumed_at=now() WHERE node_id=$1`, nodeID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), nil
}
