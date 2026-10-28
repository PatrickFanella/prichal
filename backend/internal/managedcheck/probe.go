// Package managedcheck performs bounded public observations for generated managed hosts only.
package managedcheck

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

const Timeout = 30 * time.Second

type Result struct {
	Hostname             string     `json:"hostname"`
	Code                 string     `json:"code"`
	ObservedAt           time.Time  `json:"observedAt"`
	Addresses            []string   `json:"addresses"`
	CertificateSHA256    string     `json:"certificateSha256,omitempty"`
	CertificateExpiresAt *time.Time `json:"certificateExpiresAt,omitempty"`
	Version              string     `json:"version,omitempty"`
	UnknownHost          string     `json:"unknownHost,omitempty"`
}

type Checker interface {
	Check(context.Context, string) Result
}

type Probe struct {
	lookup func(context.Context, string, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
	roots  *x509.CertPool // Only package tests replace system trust/transport; no environment URL override exists.
}

func New() *Probe {
	return &Probe{lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}
}

var denied = func() []netip.Prefix {
	var result []netip.Prefix
	for _, value := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}()

func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range denied {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func (r Result) Validate(id string, now time.Time) error {
	host, err := managednamespace.Endpoint(id)
	if err != nil || r.Hostname != host || r.ObservedAt.IsZero() || r.ObservedAt.Before(now.Add(-Timeout-time.Second)) || r.ObservedAt.After(now.Add(time.Second)) || len(r.Addresses) > 16 || len(r.Version) > 64 {
		return errors.New("invalid public check result")
	}
	switch r.Code {
	case "ready", "dns", "tls", "health", "identity", "websocket", "unknown_host", "registration", "native":
	default:
		return errors.New("invalid public check outcome")
	}
	for _, value := range r.Addresses {
		ip, err := netip.ParseAddr(value)
		if err != nil || !PublicAddress(ip) {
			return errors.New("non-public check address")
		}
	}
	if r.CertificateSHA256 != "" {
		value, err := hex.DecodeString(r.CertificateSHA256)
		if err != nil || len(value) != 32 || strings.ToLower(r.CertificateSHA256) != r.CertificateSHA256 {
			return errors.New("invalid public certificate digest")
		}
	}
	if r.Code == "ready" {
		if len(r.Addresses) == 0 || r.CertificateSHA256 == "" || r.CertificateExpiresAt == nil || !now.Before(*r.CertificateExpiresAt) || strings.TrimSpace(r.Version) == "" {
			return errors.New("incomplete public readiness")
		}
		if !strings.HasPrefix(r.UnknownHost, "pds-") || !strings.HasSuffix(r.UnknownHost, ".subcult.tv") || r.UnknownHost == host {
			return errors.New("missing unknown-host observation")
		}
		compact := strings.TrimSuffix(strings.TrimPrefix(r.UnknownHost, "pds-"), ".subcult.tv")
		if len(compact) != 32 {
			return errors.New("invalid unknown-host observation")
		}
		unknownID := compact[:8] + "-" + compact[8:12] + "-" + compact[12:16] + "-" + compact[16:20] + "-" + compact[20:]
		if expected, err := managednamespace.Endpoint(unknownID); err != nil || expected != r.UnknownHost {
			return errors.New("invalid unknown-host observation")
		}
	}
	return nil
}

func (p *Probe) transport(ctx context.Context, host string) (*http.Transport, []netip.Addr, error) {
	addresses, err := p.lookup(ctx, "ip", host)
	if err != nil || len(addresses) == 0 || len(addresses) > 16 {
		return nil, nil, errors.New("public DNS unavailable")
	}
	for i, ip := range addresses {
		if !PublicAddress(ip) {
			return nil, nil, errors.New("non-public DNS answer")
		}
		addresses[i] = ip.Unmap()
	}
	sort.Slice(addresses, func(i, j int) bool {
		if addresses[i].Is4() != addresses[j].Is4() {
			return addresses[i].Is4()
		}
		return addresses[i].Less(addresses[j])
	})
	transport := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384, ResponseHeaderTimeout: 5 * time.Second, TLSHandshakeTimeout: 5 * time.Second, TLSClientConfig: &tls.Config{RootCAs: p.roots, ServerName: host, MinVersion: tls.VersionTLS12}, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		h, port, err := net.SplitHostPort(address)
		if err != nil || h != host || port != "443" {
			return nil, errors.New("unexpected public destination")
		}
		return p.connect(ctx, addresses)
	}
	return transport, addresses, nil
}

func (p *Probe) connect(ctx context.Context, addresses []netip.Addr) (net.Conn, error) {
	for _, ip := range addresses {
		connection, err := p.dial(ctx, "tcp", net.JoinHostPort(ip.String(), "443"))
		if err == nil {
			return connection, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("public transport unavailable")
}

func tlsFailure(err error) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	var record tls.RecordHeaderError
	return errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &hostname) || errors.As(err, &record)
}

func request(ctx context.Context, client *http.Client, host, path, method string) (*http.Response, []byte, error) {
	var input io.Reader
	if method == http.MethodPost {
		input = strings.NewReader("{}")
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+host+path, input)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "prichal-managed-readiness/1")
	req.Header.Set("Cache-Control", "no-cache")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32769))
	if err != nil || len(body) > 32768 {
		return response, nil, errors.New("public response exceeded its bound")
	}
	return response, body, nil
}

func (p *Probe) Check(ctx context.Context, id string) (result Result) {
	host, err := managednamespace.Endpoint(id)
	result = Result{Hostname: host, Code: "identity", Addresses: []string{}}
	defer func() { result.ObservedAt = time.Now().UTC() }()
	if err != nil {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	transport, addresses, err := p.transport(ctx, host)
	if err != nil {
		result.Code = "dns"
		return result
	}
	defer transport.CloseIdleConnections()
	for _, ip := range addresses {
		result.Addresses = append(result.Addresses, ip.String())
	}
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, body, err := request(ctx, client, host, "/xrpc/_health", http.MethodGet)
	if err != nil {
		result.Code = "health"
		if tlsFailure(err) {
			result.Code = "tls"
		}
		return result
	}
	var health struct {
		Version string `json:"version"`
	}
	if response.StatusCode != 200 || json.Unmarshal(body, &health) != nil || len(health.Version) == 0 || len(health.Version) > 64 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		result.Code = "health"
		return result
	}
	leaf := response.TLS.PeerCertificates[0]
	hash := sha256.Sum256(leaf.Raw)
	result.CertificateSHA256 = hex.EncodeToString(hash[:])
	expires := leaf.NotAfter
	result.CertificateExpiresAt = &expires
	result.Version = health.Version
	for _, path := range []string{"/xrpc/com.atproto.server.describeServer", "/.well-known/oauth-authorization-server"} {
		response, body, err = request(ctx, client, host, path, http.MethodGet)
		if err != nil || response.StatusCode != 200 {
			result.Code = "identity"
			return result
		}
		var value struct {
			DID           string `json:"did"`
			Issuer        string `json:"issuer"`
			Authorization string `json:"authorization_endpoint"`
			Token         string `json:"token_endpoint"`
		}
		if json.Unmarshal(body, &value) != nil || path == "/xrpc/com.atproto.server.describeServer" && value.DID != "did:web:"+host || path == "/.well-known/oauth-authorization-server" && (value.Issuer != "https://"+host || value.Authorization != "https://"+host+"/oauth/authorize" || value.Token != "https://"+host+"/oauth/token") {
			result.Code = "identity"
			return result
		}
	}
	for _, path := range []string{"/xrpc/com.atproto.server.createAccount", "/@atproto/oauth-provider/~api/sign-up"} {
		response, _, err = request(ctx, client, host, path, http.MethodPost)
		if err != nil || response.StatusCode != 403 {
			result.Code = "registration"
			return result
		}
	}
	if !p.websocket(ctx, host, addresses) {
		result.Code = "websocket"
		return result
	}
	var identity [16]byte
	if _, err = rand.Read(identity[:]); err != nil {
		return result
	}
	identity[6] = (identity[6] & 15) | 64
	identity[8] = (identity[8] & 63) | 128
	unknownID := fmt.Sprintf("%x-%x-%x-%x-%x", identity[:4], identity[4:6], identity[6:8], identity[8:10], identity[10:])
	unknown, _ := managednamespace.Endpoint(unknownID)
	unknownTransport, _, err := p.transport(ctx, unknown)
	if err != nil {
		result.Code = "unknown_host"
		return result
	}
	defer unknownTransport.CloseIdleConnections()
	unknownClient := &http.Client{Transport: unknownTransport, Timeout: 6 * time.Second, CheckRedirect: client.CheckRedirect}
	response, _, err = request(ctx, unknownClient, unknown, "/xrpc/_health", http.MethodGet)
	if err != nil || (response.StatusCode != 404 && response.StatusCode != 421) {
		result.Code = "unknown_host"
		return result
	}
	result.UnknownHost = unknown
	result.Code = "ready"
	return result
}

func (p *Probe) websocket(ctx context.Context, host string, addresses []netip.Addr) bool {
	connection, err := p.connect(ctx, addresses)
	if err != nil {
		return false
	}
	defer connection.Close()
	deadline := time.Now().Add(6 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	_ = connection.SetDeadline(deadline)
	secure := tls.Client(connection, &tls.Config{ServerName: host, RootCAs: p.roots, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
	if secure.HandshakeContext(ctx) != nil {
		return false
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return false
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/xrpc/com.atproto.sync.subscribeRepos", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", key)
	if req.Write(secure) != nil {
		return false
	}
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(secure, 16384)), req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	accept := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	upgrade := false
	for _, token := range strings.Split(response.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			upgrade = true
		}
	}
	return response.StatusCode == 101 && strings.EqualFold(response.Header.Get("Upgrade"), "websocket") && upgrade && response.Header.Get("Sec-WebSocket-Accept") == base64.StdEncoding.EncodeToString(accept[:])
}
