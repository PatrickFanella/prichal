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

// localCertificate retains a fixture-only TLS identity. It never installs system trust.
func (d *Docker) localCertificate(id string) ([]byte, error) {
	if !instanceID.MatchString(id) {
		return nil, errors.New("invalid TLS instance")
	}
	path := filepath.Join(d.directory, id+"-tls.pem")
	read := func() ([]byte, error) { return d.readLocalCertificate(id) }
	if cert, err := read(); err == nil {
		return cert, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: id + ".localhost"}, DNSNames: []string{id + ".localhost"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(7 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
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
	if err = os.Link(temporary.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
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
	path := filepath.Join(d.directory, id+"-tls.pem")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, errors.New("unsafe local TLS identity")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, errors.New("invalid local TLS identity")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if cert.VerifyHostname(id+".localhost") != nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || cert.NotAfter.Sub(cert.NotBefore) > 8*24*time.Hour {
		return nil, errors.New("local TLS identity expired or mismatched")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), nil
}
