package nodeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Peer is read from the node registry, never a customer-provided URL.
type Peer struct {
	NodeID        string
	Endpoint      string
	PublicKeyHash []byte
}

// Client carries worker credentials, independently of public API configuration.
type Client struct {
	roots       *x509.CertPool
	certificate tls.Certificate
}

// NewClient loads a dedicated agent CA and worker client certificate.
func NewClient(caPath, certPath, keyPath string) (*Client, error) {
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, errors.New("worker agent CA required")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid worker agent CA")
	}
	info, err := os.Lstat(keyPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("worker agent key must be a private regular file")
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, errors.New("worker agent certificate unavailable")
	}
	return &Client{roots, certificate}, nil
}
func (c *Client) httpClient(peer Peer) (*http.Client, error) {
	endpoint, err := url.Parse(peer.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || !instanceID.MatchString(peer.NodeID) || len(peer.PublicKeyHash) != 32 {
		return nil, errors.New("invalid registered node")
	}
	// The current executable only supports local qualification nodes.
	host := endpoint.Hostname()
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return nil, errors.New("nonlocal agent endpoint is not yet supported")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: c.roots, Certificates: []tls.Certificate{c.certificate}, VerifyConnection: func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 || state.PeerCertificates[0].Subject.CommonName != "pds-ass-node-"+peer.NodeID {
			return errors.New("registered node identity mismatch")
		}
		public, err := x509.MarshalPKIXPublicKey(state.PeerCertificates[0].PublicKey)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(public)
		if subtle.ConstantTimeCompare(hash[:], peer.PublicKeyHash) != 1 {
			return errors.New("registered node key mismatch")
		}
		return nil
	}}}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("agent redirects rejected") }}, nil
}

// Ensure verifies both the certificate chain and registry key before requesting a fixed runtime.
func (c *Client) Ensure(ctx context.Context, peer Peer, id string) (Instance, error) {
	if !instanceID.MatchString(id) {
		return Instance{}, errors.New("invalid instance ID")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return Instance{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "PUT", peer.Endpoint+"/v1/instances/"+id, bytes.NewBufferString("{}"))
	if err != nil {
		return Instance{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Instance{}, errors.New("registered node is unavailable or its identity could not be verified")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Instance{}, errors.New("registered node could not confirm reconciliation")
	}
	var result Instance
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || result.ID != id || result.Endpoint != LocalEndpoint(id) || (result.State != "running" && result.State != "starting") {
		return Instance{}, errors.New("invalid node reconciliation response")
	}
	return result, nil
}

// Capacity reads measurements only after authenticating the registered node.
func (c *Client) Capacity(ctx context.Context, peer Peer) (Capacity, error) {
	client, err := c.httpClient(peer)
	if err != nil {
		return Capacity{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "GET", peer.Endpoint+"/v1/capacity", nil)
	if err != nil {
		return Capacity{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return Capacity{}, errors.New("registered node measurements unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Capacity{}, errors.New("registered node measurements unavailable")
	}
	var result Capacity
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<16))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || result.MaximumSlots < 0 || result.MaximumSlots > 1000 || result.CPUs < 1 || result.MemoryBytes < 1 {
		return Capacity{}, errors.New("invalid node measurements")
	}
	seen := map[string]bool{}
	for _, id := range result.InstanceIDs {
		if !instanceID.MatchString(id) || seen[id] {
			return Capacity{}, errors.New("invalid node namespace inventory")
		}
		seen[id] = true
	}
	return result, nil
}
