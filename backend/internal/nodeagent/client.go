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
	roots                     *x509.CertPool
	certificate               tls.Certificate
	caPath, certPath, keyPath string
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
	return &Client{roots: roots, certificate: certificate, caPath: caPath, certPath: certPath, keyPath: keyPath}, nil
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
	// Every operation sees atomically renewed files rather than a startup snapshot.
	// A malformed replacement fails closed; old credentials are never a fallback.
	current, err := NewClient(c.caPath, c.certPath, c.keyPath)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: current.roots, Certificates: []tls.Certificate{current.certificate}, VerifyConnection: func(state tls.ConnectionState) error {
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
	return c.instance(ctx, peer, id, "PUT")
}

// Status authenticates a registered node before inspecting an existing runtime.
func (c *Client) Status(ctx context.Context, peer Peer, id string) (Instance, error) {
	return c.instance(ctx, peer, id, "GET")
}

func (c *Client) instance(ctx context.Context, peer Peer, id, method string) (Instance, error) {
	if !instanceID.MatchString(id) {
		return Instance{}, errors.New("invalid instance ID")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return Instance{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, method, peer.Endpoint+"/v1/instances/"+id, bytes.NewBufferString("{}"))
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
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.ID != id || result.Endpoint != LocalEndpoint(id) || (result.State != "running" && result.State != "starting" && result.State != "suspended") || !validServiceMetadata(result) || (result.Release != nil && !result.Release.Valid()) {
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

// Enroll requests a fixed tenant-scoped invitation from the authenticated node.
func (c *Client) Enroll(ctx context.Context, peer Peer, id, ticket string) (Enrollment, error) {
	return c.enrollment(ctx, peer, id, ticket, false)
}

// WithdrawEnrollment authenticates the registered node before closing a retained ticket.
func (c *Client) WithdrawEnrollment(ctx context.Context, peer Peer, id, ticket string) (Enrollment, error) {
	return c.enrollment(ctx, peer, id, ticket, true)
}

func (c *Client) enrollment(ctx context.Context, peer Peer, id, ticket string, withdraw bool) (Enrollment, error) {
	if !instanceID.MatchString(id) || !instanceID.MatchString(ticket) {
		return Enrollment{}, errors.New("invalid enrollment request")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return Enrollment{}, err
	}
	defer client.CloseIdleConnections()
	body, _ := json.Marshal(struct {
		Ticket string `json:"ticket"`
	}{ticket})
	path := "/v1/instances/" + id + "/enrollment"
	if withdraw {
		path += "/withdraw"
	}
	request, err := http.NewRequestWithContext(ctx, "POST", peer.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return Enrollment{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Enrollment{}, errors.New("registered enrollment node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Enrollment{}, errors.New("registered node could not confirm enrollment")
	}
	var result Enrollment
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.InstanceID != id || result.Ticket != ticket {
		return Enrollment{}, errors.New("invalid node enrollment response")
	}
	switch result.State {
	case "invitation_ready":
		if withdraw || !validInvitationCode(result.Code) || result.ExpiresAt <= time.Now().UnixMilli() || result.ExpiresAt > time.Now().Add(15*time.Minute+5*time.Second).UnixMilli() || result.DID != "" {
			return Enrollment{}, errors.New("invalid expiring invitation")
		}
	case "account_created", "verified":
		if !accountDID.MatchString(result.DID) || result.Code != "" || result.Handle == "" || len(result.Handle) > 253 || result.ExpiresAt != 0 || (result.State == "verified" && !result.EmailConfirmed) {
			return Enrollment{}, errors.New("invalid account status")
		}
	case "expired":
		if withdraw || result.Code != "" || result.DID != "" {
			return Enrollment{}, errors.New("invalid expired enrollment")
		}
	case "withdrawn":
		if !withdraw || result.Code != "" || result.DID != "" || result.Handle != "" || result.EmailConfirmed || result.ExpiresAt != 0 {
			return Enrollment{}, errors.New("invalid withdrawn enrollment")
		}
	default:
		return Enrollment{}, errors.New("unsupported enrollment state")
	}
	return result, nil
}

// Export authenticates the registered node before requesting the expected sole native actor.
// MemberInvitation authenticates the registered node before issuing or reporting a member code.
func (c *Client) MemberInvitation(ctx context.Context, peer Peer, id, ticket string) (MemberInvitation, error) {
	return c.member(ctx, peer, id, ticket, false)
}

// WithdrawMemberInvitation authenticates the registered node before disabling an unused member code.
func (c *Client) WithdrawMemberInvitation(ctx context.Context, peer Peer, id, ticket string) (MemberInvitation, error) {
	return c.member(ctx, peer, id, ticket, true)
}

func (c *Client) member(ctx context.Context, peer Peer, id, ticket string, withdraw bool) (MemberInvitation, error) {
	if !instanceID.MatchString(id) || !instanceID.MatchString(ticket) {
		return MemberInvitation{}, errors.New("invalid member invitation request")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return MemberInvitation{}, err
	}
	defer client.CloseIdleConnections()
	body, _ := json.Marshal(struct {
		Ticket string `json:"ticket"`
	}{ticket})
	path := "/v1/instances/" + id + "/members/invitation"
	if withdraw {
		path += "/withdraw"
	}
	request, err := http.NewRequestWithContext(ctx, "POST", peer.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return MemberInvitation{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return MemberInvitation{}, errors.New("registered member node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return MemberInvitation{}, errors.New("registered node could not confirm member invitation")
	}
	var result MemberInvitation
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || !result.Valid(id, ticket, withdraw) {
		return MemberInvitation{}, errors.New("invalid node member response")
	}
	return result, nil
}

func (c *Client) Export(ctx context.Context, peer Peer, id, did string) (PortableExport, error) {
	if !instanceID.MatchString(id) || !accountDID.MatchString(did) {
		return PortableExport{}, errors.New("invalid portable export reference")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return PortableExport{}, err
	}
	defer client.CloseIdleConnections()
	body, marshalErr := json.Marshal(struct {
		DID string `json:"did"`
	}{did})
	if marshalErr != nil {
		return PortableExport{}, marshalErr
	}
	request, err := http.NewRequestWithContext(ctx, "POST", peer.Endpoint+"/v1/instances/"+id+"/export", bytes.NewReader(body))
	if err != nil {
		return PortableExport{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return PortableExport{}, errors.New("registered node export unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return PortableExport{}, errors.New("registered node export could not be confirmed")
	}
	var result PortableExport
	decoder := json.NewDecoder(io.LimitReader(response.Body, (24<<20)+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || !result.ValidFor(did) {
		return PortableExport{}, errors.New("registered node portable export invalid")
	}
	return result, nil
}
