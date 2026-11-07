package nodeagent

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// LocalTLSLifetime bounds a new private ingress identity. Trust is the exact
// certificate pin in the managed inventory, so the lifetime only sets how often
// the pin must be republished; it is not a rotation control.
const LocalTLSLifetime = 2 * 365 * 24 * time.Hour

var errLocalTLSExpired = errors.New("local TLS identity expired")

// localCertificate retains a fixture-only TLS identity. It never installs system trust.
// An expired identity is reissued with its retained key; every other defect is refused.
func (d *Docker) localCertificate(id string) ([]byte, error) {
	if !instanceID.MatchString(id) {
		return nil, errors.New("invalid TLS instance")
	}
	path := filepath.Join(d.directory, id+"-tls.pem")
	read := func() ([]byte, error) { return d.readLocalCertificate(id) }
	cert, key, err := d.readLocalIdentity(id)
	if err == nil {
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), nil
	}
	renew := errors.Is(err, errLocalTLSExpired)
	if !renew && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !renew {
		if key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return nil, err
		}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: id + ".localhost"}, DNSNames: []string{id + ".localhost"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(LocalTLSLifetime), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	data := bytes.Join([][]byte{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})}, nil)
	temporary, err := os.CreateTemp(d.directory, ".local-tls-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(data); err != nil {
		temporary.Close()
		return nil, err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return nil, err
	}
	if err = temporary.Close(); err != nil {
		return nil, err
	}
	if renew {
		// Replace only the expired file; a running edge keeps its mounted inode until restart.
		err = os.Rename(temporary.Name(), path)
	} else if err = os.Link(temporary.Name(), path); errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	directory, err := os.Open(d.directory)
	if err != nil {
		return nil, err
	}
	err = directory.Sync()
	directory.Close()
	if err != nil {
		return nil, err
	}
	return read()
}

func (d *Docker) readLocalCertificate(id string) ([]byte, error) {
	cert, _, err := d.readLocalIdentity(id)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), nil
}

// readLocalIdentity returns errLocalTLSExpired, with the retained key, only for an
// otherwise valid identity past NotAfter.
func (d *Docker) readLocalIdentity(id string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	path := filepath.Join(d.directory, id+"-tls.pem")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, nil, errors.New("unsafe local TLS identity")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, nil, errors.New("invalid local TLS identity")
	}
	key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("invalid local TLS identity")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	if cert.VerifyHostname(id+".localhost") != nil || now.Before(cert.NotBefore) || cert.NotAfter.Sub(cert.NotBefore) > LocalTLSLifetime+24*time.Hour {
		return nil, nil, errors.New("local TLS identity expired or mismatched")
	}
	if !now.Before(cert.NotAfter) {
		// Only an identity this agent issued (self-signed by its retained key) may be reissued.
		// A CA-signed pilot ingress leaf belongs to its operator renewal and stays refused.
		if cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) != nil {
			return nil, nil, errors.New("local TLS identity expired or mismatched")
		}
		// The caller may reissue with this retained key; the expired certificate is never served.
		return nil, key, errLocalTLSExpired
	}
	return cert, key, nil
}
