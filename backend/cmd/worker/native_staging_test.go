package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/fleet"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nativeStagingScope struct {
	Node         string `json:"node"`
	Owner        string `json:"owner,omitempty"`
	Organization string `json:"organization,omitempty"`
	Deployment   string `json:"deployment,omitempty"`
	RequestKey   string `json:"requestKey,omitempty"`
}

func nativeStagingPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	if os.Getenv("PDS_QUALIFY_PRIVATE_STAGING") != "true" {
		t.Skip("requires designated private staging opt-in")
	}
	root := filepath.Join(os.Getenv("HOME"), ".local/state/prichal-staging-02-bootstrap")
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 35437 || cfg.ConnConfig.Database != "pds_local" || cfg.ConnConfig.User != "pds_staging" {
		t.Fatal("qualification requires the dedicated staging-02 registry")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Apply(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool, root
}
func nativeRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func nativeWrite(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestNativePrivateStagingBootstrap(t *testing.T) {
	pool, root := nativeStagingPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := os.Stat(filepath.Join(root, "scope.json")); err == nil {
		t.Fatal("retained scope exists; bootstrap cannot replace it")
	}
	block, _ := pem.Decode(nativeRead(t, filepath.Join(root, "pki/ca.crt")))
	if block == nil {
		t.Fatal("CA certificate unavailable")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	block, _ = pem.Decode(nativeRead(t, filepath.Join(root, "pki/ca.key")))
	if block == nil {
		t.Fatal("CA key unavailable")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		t.Fatal("CA signer unavailable")
	}
	store := fleet.Store{Pool: pool, CA: ca, Key: signer}
	bootstrap, err := store.Issue(ctx, "https://127.0.0.1:22271", 1)
	if err != nil {
		t.Fatal(err)
	}
	csr := nativeRead(t, filepath.Join(root, "node.csr"))
	cert, err := store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, csr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enroll(ctx, bootstrap.NodeID, bootstrap.Token, csr); err != fleet.ErrUnauthorized {
		t.Fatal("consumed enrollment accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "node.crt"), cert, 0600); err != nil {
		t.Fatal(err)
	}
	nativeWrite(t, filepath.Join(root, "scope.json"), nativeStagingScope{Node: bootstrap.NodeID})
	sum := sha256.Sum256(cert)
	t.Logf("enrolled fresh node %s, certificate SHA256 %s", bootstrap.NodeID, hex.EncodeToString(sum[:]))
}

type interruptedNativeAgent struct {
	*nodeagent.Client
	success chan nodeagent.Instance
}

func (a interruptedNativeAgent) Ensure(ctx context.Context, peer nodeagent.Peer, id string) (nodeagent.Instance, error) {
	instance, err := a.Client.Ensure(ctx, peer, id)
	if err != nil {
		return instance, err
	}
	select {
	case a.success <- instance:
	case <-ctx.Done():
		return nodeagent.Instance{}, ctx.Err()
	}
	<-ctx.Done()
	return nodeagent.Instance{}, ctx.Err()
}
func TestNativePrivateStagingWorkerRestart(t *testing.T) {
	pool, root := nativeStagingPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var scope nativeStagingScope
	if err := json.Unmarshal(nativeRead(t, filepath.Join(root, "scope.json")), &scope); err != nil {
		t.Fatal(err)
	}
	agent, err := nodeagent.NewClient(filepath.Join(root, "pki/ca.crt"), filepath.Join(root, "pki/worker.crt"), filepath.Join(root, "pki/worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	var endpoint string
	var pin []byte
	if err := pool.QueryRow(ctx, `SELECT agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1 AND state='active'`, scope.Node).Scan(&endpoint, &pin); err != nil {
		t.Fatal(err)
	}
	peer := nodeagent.Peer{NodeID: scope.Node, Endpoint: endpoint, PublicKeyHash: pin}
	before, err := agent.Capacity(ctx, peer)
	if err != nil {
		t.Fatal(err)
	}
	if before.MaximumSlots != 1 || before.CPUs != 3 {
		t.Fatalf("unexpected designated capacity: %+v", before)
	}
	if scope.Organization == "" {
		if len(before.InstanceIDs) != 0 {
			t.Fatal("fresh drill requires empty native node")
		}
		scope.RequestKey = fmt.Sprintf("native-staging-%d", time.Now().UnixNano())
		if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('https://git.subcult.tv',$1,'Synthetic staging owner') RETURNING id`, scope.RequestKey).Scan(&scope.Owner); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Private native staging qualification',$1) RETURNING id`, scope.Owner).Scan(&scope.Organization); err != nil {
			t.Fatal(err)
		}
		nativeWrite(t, filepath.Join(root, "scope.json"), scope)
	}
	if _, err := (entitlement.Store{Pool: pool}).Submit(ctx, "staging-qualification", scope.Organization, entitlement.Request{Key: scope.Organization, State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	owners, err := auth.New(ctx, pool, auth.Settings{Mode: "production", Issuer: "https://git.subcult.tv", ClientID: "private-staging-qualification", ClientSecret: "unused-synthetic-client-secret", Origin: "https://prichal-staging-02.subcult.tv"})
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	if _, err := pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'native-staging-csrf',now()+interval '1 hour')`, hash[:], scope.Owner); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM management_sessions WHERE token_hash=$1`, hash[:])
	store, err := deployment.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	store.Staging = true
	mux := http.NewServeMux()
	store.Register(owners, false)(mux)
	body := fmt.Sprintf(`{"kind":"agent","name":"native-staging-02","region":"local","idempotencyKey":%q}`, scope.RequestKey)
	submit := func() deployment.Operation {
		t.Helper()
		r := httptest.NewRequest("POST", "https://prichal-staging-02.subcult.tv/api/organizations/"+scope.Organization+"/deployments", strings.NewReader(body))
		r.Header.Set("Origin", "https://prichal-staging-02.subcult.tv")
		r.Header.Set("X-CSRF-Token", "native-staging-csrf")
		r.AddCookie(&http.Cookie{Name: "__Host-pds-session", Value: token})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("production-mode owner submission: %d %s", w.Code, w.Body.String())
		}
		var op deployment.Operation
		if err := json.Unmarshal(w.Body.Bytes(), &op); err != nil {
			t.Fatal(err)
		}
		return op
	}
	op := submit()
	if scope.Deployment != "" && scope.Deployment != op.ID {
		t.Fatal("deployment identity changed")
	}
	scope.Deployment = op.ID
	nativeWrite(t, filepath.Join(root, "scope.json"), scope)
	if op.State == "ready" {
		t.Fatal("retained ready drill cannot be represented as another fresh interruption")
	}
	interrupted := interruptedNativeAgent{Client: agent, success: make(chan nodeagent.Instance, 1)}
	firstCtx, stopFirst := context.WithCancel(ctx)
	firstDone := make(chan error, 1)
	go func() { firstDone <- runStagingWorker(firstCtx, pool, interrupted, false) }()
	defer stopFirst()
	var first nodeagent.Instance
	select {
	case first = <-interrupted.success:
	case err := <-firstDone:
		t.Fatalf("initial worker exited: %v", err)
	case <-ctx.Done():
		t.Fatal("native creation was not confirmed")
	}
	stopFirst()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("interrupted worker did not stop")
	}
	checkpoint, err := store.Get(ctx, scope.Owner, op.ID)
	if err != nil || checkpoint.State != "configuring" {
		t.Fatalf("native success checkpoint advanced before interruption: %+v %v", checkpoint, err)
	}
	workerCtx, stopSecond := context.WithCancel(ctx)
	secondDone := make(chan error, 1)
	go func() { secondDone <- runStagingWorker(workerCtx, pool, agent, false) }()
	defer func() {
		stopSecond()
		if err := <-secondDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		op, err = store.Get(ctx, scope.Owner, scope.Deployment)
		if err != nil {
			t.Fatal(err)
		}
		if op.State == "ready" {
			break
		}
		if op.State == "failed" {
			t.Fatalf("retained staging operation failed: %+v", op)
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("restarted native worker did not reach ready")
		}
	}
	if replay := submit(); replay.ID != op.ID {
		t.Fatal("request replay changed deployment")
	}
	after, err := agent.Capacity(ctx, peer)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.InstanceIDs) != 1 || after.InstanceIDs[0] != op.ID {
		t.Fatalf("native namespace changed: %+v", after)
	}
	final, err := agent.Status(ctx, peer, op.ID)
	if err != nil || !final.Healthy || final.ID != first.ID {
		t.Fatalf("retained native runtime: %+v %v", final, err)
	}
	var placements int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE deployment_id=$1 AND node_id=$2`, op.ID, scope.Node).Scan(&placements); err != nil || placements != 1 {
		t.Fatalf("placement changed: %d %v", placements, err)
	}
	nativeWrite(t, filepath.Join(root, "qualification.json"), map[string]any{"completedAt": time.Now().UTC(), "node": scope.Node, "deployment": op.ID, "endpoint": op.Endpoint, "state": op.State, "interruptedCheckpoint": checkpoint.State, "placements": placements, "nativeNamespaces": after.InstanceIDs, "productionModeOwnerHandler": true, "syntheticSession": true, "publicHostingQualified": false})
	t.Logf("native worker interruption/restart passed for %s on %s", op.ID, scope.Node)
}

func TestNativePrivateStagingBoundaries(t *testing.T) {
	pool, root := nativeStagingPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var scope nativeStagingScope
	if err := json.Unmarshal(nativeRead(t, filepath.Join(root, "scope.json")), &scope); err != nil {
		t.Fatal(err)
	}
	agent, err := nodeagent.NewClient(filepath.Join(root, "pki/ca.crt"), filepath.Join(root, "pki/worker.crt"), filepath.Join(root, "pki/worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	var endpoint string
	var pin []byte
	if err := pool.QueryRow(ctx, `SELECT agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1`, scope.Node).Scan(&endpoint, &pin); err != nil {
		t.Fatal(err)
	}
	peer := nodeagent.Peer{NodeID: scope.Node, Endpoint: endpoint, PublicKeyHash: pin}
	bad := peer
	bad.PublicKeyHash = make([]byte, 32)
	if _, err := agent.Capacity(ctx, bad); err == nil {
		t.Fatal("unregistered key accepted")
	}
	bad = peer
	bad.NodeID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	if _, err := agent.Capacity(ctx, bad); err == nil {
		t.Fatal("unregistered node identity accepted")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(nativeRead(t, filepath.Join(root, "pki/ca.crt"))) {
		t.Fatal("CA unavailable")
	}
	unauthTransport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
	defer unauthTransport.CloseIdleConnections()
	unauth := &http.Client{Timeout: 5 * time.Second, Transport: unauthTransport}
	if response, err := unauth.Get(endpoint + "/v1/capacity"); err == nil {
		response.Body.Close()
		t.Fatal("certificate-less agent access accepted")
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(root, "pki/worker.crt"), filepath.Join(root, "pki/worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	for _, probe := range []struct {
		method, path, body string
		status             int
	}{{"PUT", "/v1/instances/" + scope.Deployment, `{"image":"unapproved"}`, 400}, {"POST", "/v1/exec", `{}`, 404}, {"PUT", "/v1/instances/invalid", `{}`, 404}} {
		request, err := http.NewRequestWithContext(ctx, probe.method, endpoint+probe.path, strings.NewReader(probe.body))
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != probe.status {
			t.Fatalf("allowlist path %s: %d, want %d", probe.path, response.StatusCode, probe.status)
		}
	}
	other := "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"
	if _, err := agent.Ensure(ctx, peer, other); err == nil {
		t.Fatal("occupied native ceiling exceeded")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := agent.Ensure(canceled, peer, scope.Deployment); err == nil {
		t.Fatal("canceled request succeeded")
	}
	observed, err := agent.Capacity(ctx, peer)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.InstanceIDs) != 1 || observed.InstanceIDs[0] != scope.Deployment {
		t.Fatalf("rejection changed namespace inventory: %+v", observed)
	}
	nativeWrite(t, filepath.Join(root, "boundaries.json"), map[string]any{"completedAt": time.Now().UTC(), "wrongKeyRejected": true, "wrongNodeRejected": true, "certificateRequired": true, "arbitraryImageAndCommandRejected": true, "capacityExhaustionPreservesNamespace": true, "canceledRequestPreservesNamespace": true, "nativeNamespaces": observed.InstanceIDs})
}
