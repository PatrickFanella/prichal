package managedcheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

const instance = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"

func fixture(t *testing.T, fault string) (*Probe, *atomic.Int64) {
	t.Helper()
	host, _ := managednamespace.Endpoint(instance)
	calls := new(atomic.Int64)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"*.subcult.tv"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	parsed, _ := x509.ParseCertificate(der)
	roots.AddCert(parsed)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Host != host {
			if fault == "unknown_host" {
				w.WriteHeader(200)
			} else {
				w.WriteHeader(404)
			}
			return
		}
		if fault == "redirect" {
			http.Redirect(w, r, "https://127.0.0.1/secret", 302)
			return
		}
		if fault == "oversize" {
			_, _ = w.Write([]byte(strings.Repeat("x", 32769)))
			return
		}
		switch r.URL.Path {
		case "/xrpc/_health":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.4.5036"})
		case "/xrpc/com.atproto.server.describeServer":
			did := "did:web:" + host
			if fault == "identity" {
				did = "did:web:foreign.subcult.tv"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"did": did})
		case "/.well-known/oauth-authorization-server":
			issuer := "https://" + host
			if fault == "oauth" {
				issuer += "/"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": "https://" + host + "/oauth/authorize", "token_endpoint": "https://" + host + "/oauth/token"})
		case "/xrpc/com.atproto.server.createAccount", "/@atproto/oauth-provider/~api/sign-up":
			if fault == "registration" {
				w.WriteHeader(200)
			} else {
				w.WriteHeader(403)
			}
		case "/xrpc/com.atproto.sync.subscribeRepos":
			if fault == "websocket" {
				w.WriteHeader(200)
				return
			}
			connection, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer connection.Close()
			sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
			accept, connectionHeader := base64.StdEncoding.EncodeToString(sum[:]), "Upgrade"
			if fault == "websocket_accept" {
				accept = "foreign"
			}
			if fault == "websocket_connection" {
				connectionHeader = "notupgrade"
			}
			_, _ = fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nConnection: %s\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\n\r\n", connectionHeader, accept)
			_ = buffer.Flush()
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	probe := &Probe{roots: roots, lookup: func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}, dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "1.1.1.1:443" {
			t.Errorf("unvalidated dial %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	return probe, calls
}

func TestPinnedPublicProtocolAndFailures(t *testing.T) {
	for fault, want := range map[string]string{"": "ready", "identity": "identity", "oauth": "identity", "redirect": "health", "oversize": "health", "registration": "registration", "websocket": "websocket", "websocket_accept": "websocket", "websocket_connection": "websocket", "unknown_host": "unknown_host"} {
		t.Run(fault, func(t *testing.T) {
			p, calls := fixture(t, fault)
			result := p.Check(context.Background(), instance)
			if result.Code != want {
				t.Fatalf("got %s want %s", result.Code, want)
			}
			if err := result.Validate(instance, time.Now()); err != nil {
				t.Fatal(err)
			}
			if fault == "redirect" && calls.Load() != 1 {
				t.Fatal("redirect was followed")
			}
		})
	}
}

func TestDNSAndSystemTrustFailClosed(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.50", "100.64.0.1", "169.254.169.254", "192.0.2.1", "198.18.0.1", "::1", "::ffff:10.0.0.1", "64:ff9b::a00:1", "2001:db8::1", "2002:a00:1::", "fc00::1", "fe80::1", "3fff::1"} {
		if PublicAddress(netip.MustParseAddr(ip)) {
			t.Fatalf("special address accepted: %s", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "104.21.50.228", "2606:4700:3032::6815:32e4"} {
		if !PublicAddress(netip.MustParseAddr(ip)) {
			t.Fatal("public address refused", ip)
		}
	}
	p, calls := fixture(t, "")
	p.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("10.0.0.50")}, nil
	}
	if result := p.Check(context.Background(), instance); result.Code != "dns" || calls.Load() != 0 {
		t.Fatal("mixed private answer reached transport")
	}
	p, _ = fixture(t, "")
	p.roots = x509.NewCertPool()
	if result := p.Check(context.Background(), instance); result.Code != "tls" {
		t.Fatal("untrusted public TLS accepted", result.Code)
	}
}

func TestCanceledProbeAndInvalidIdentityDoNotActivate(t *testing.T) {
	p, calls := fixture(t, "")
	if result := p.Check(context.Background(), "../host"); result.Code == "ready" || calls.Load() != 0 {
		t.Fatal("invalid identity made a request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := p.Check(ctx, instance); result.Code == "ready" {
		t.Fatal("canceled probe ready")
	}
	result := Result{Hostname: "other.subcult.tv", Code: "ready", ObservedAt: time.Now()}
	if result.Validate(instance, time.Now()) == nil {
		t.Fatal("incomplete result accepted")
	}
}
