package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/fleet"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("node enrollment requires explicit local mode")
	}
	bootstrapPath := flag.String("bootstrap", "", "private one-time bootstrap file")
	out := flag.String("out", "", "private node state directory")
	flag.Parse()
	if *bootstrapPath == "" || *out == "" || flag.NArg() != 0 {
		return errors.New("usage: node-enroll --bootstrap FILE --out DIRECTORY")
	}
	info, err := os.Lstat(*bootstrapPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("bootstrap must be a private regular file")
	}
	data, err := os.ReadFile(*bootstrapPath)
	if err != nil {
		return err
	}
	var bootstrap fleet.Bootstrap
	if err = json.Unmarshal(data, &bootstrap); err != nil || !regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(bootstrap.NodeID) || len(bootstrap.Token) != 64 {
		return errors.New("invalid bootstrap file")
	}
	if err = os.MkdirAll(*out, 0700); err != nil {
		return err
	}
	info, err = os.Lstat(*out)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("node state must be a private directory")
	}
	keyPath := filepath.Join(*out, "node.key")
	var key *ecdsa.PrivateKey
	if info, statErr := os.Lstat(keyPath); statErr == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return errors.New("node key must be a private regular file")
	}
	keyPEM, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("node key already exists; retry enrollment")
		}
		if _, err = file.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
			file.Close()
			return err
		}
		if err = file.Sync(); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		info, err = os.Lstat(keyPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("node key must be a private regular file")
		}
		block, _ := pem.Decode(keyPEM)
		if block == nil {
			return errors.New("invalid saved node key")
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return errors.New("invalid saved node key")
		}
		var ok bool
		key, ok = parsed.(*ecdsa.PrivateKey)
		if !ok {
			return errors.New("saved node key must be ECDSA")
		}
	}
	caPEM, err := os.ReadFile(os.Getenv("PDS_ENROLLMENT_CA_FILE"))
	if err != nil {
		return errors.New("enrollment trust root required")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return errors.New("invalid enrollment trust root")
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("enrollment redirects rejected") }}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))})
	if err != nil {
		return err
	}
	directory, err := os.Open(*out)
	if err != nil {
		return err
	}
	err = directory.Sync()
	directory.Close()
	if err != nil {
		return err
	}
	base := "https://127.0.0.1:9092/v1/nodes/" + bootstrap.NodeID
	request, err := http.NewRequest("POST", base+"/enroll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	response, err := client.Do(request)
	var certificate []byte
	if err == nil {
		if response.StatusCode == 200 {
			certificate, err = io.ReadAll(io.LimitReader(response.Body, 8192))
		}
		response.Body.Close()
	}
	if len(certificate) == 0 {
		response, err = client.Get(base + "/certificate")
		if err != nil {
			return errors.New("enrollment could not be confirmed; retry with the same node key")
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return errors.New("enrollment rejected or unavailable; retain the node key and bootstrap file")
		}
		certificate, err = io.ReadAll(io.LimitReader(response.Body, 8192))
		response.Body.Close()
		if err != nil {
			return err
		}
	}
	block, _ := pem.Decode(certificate)
	if block == nil {
		return errors.New("invalid enrollment certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.Subject.CommonName != "pds-ass-node-"+bootstrap.NodeID {
		return errors.New("enrollment certificate identity mismatch")
	}
	public, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !public.Equal(&key.PublicKey) {
		return errors.New("enrollment certificate key mismatch")
	}
	if _, err = cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return errors.New("enrollment certificate trust validation failed")
	}
	path := filepath.Join(*out, "node.crt")
	if info, statErr := os.Lstat(path); statErr == nil && !info.Mode().IsRegular() {
		return errors.New("node certificate must be a regular file")
	}
	if existing, err := os.ReadFile(path); err == nil && !bytes.Equal(existing, certificate) {
		return errors.New("existing node certificate differs; preserve it for rotation")
	}
	if err = os.WriteFile(path, certificate, 0600); err != nil {
		return err
	}
	fmt.Printf("Node %s enrolled. Certificate and private key saved in %s\n", bootstrap.NodeID, *out)
	return nil
}
