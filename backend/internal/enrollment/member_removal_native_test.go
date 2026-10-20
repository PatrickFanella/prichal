package enrollment

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
)

// This opt-in qualification uses real Docker/native accounts. Normal smoke/CI skips it.
// The fixed canonical PLC/mail fixture is retained; only fresh tenant resources are allocated.
func TestMemberRemovalNativeMigrationWindow(t *testing.T) {
	if os.Getenv("PDS_QUALIFY_NATIVE_MEMBER_WINDOW") != "true" {
		t.Skip("explicit disposable native qualification required")
	}
	root := os.Getenv("PDS_NATIVE_MEMBER_WINDOW_DIRECTORY")
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(repository, ".local", "member-removal-native") + string(filepath.Separator)
	if !filepath.IsAbs(root) || !strings.HasPrefix(filepath.Clean(root), allowed) {
		t.Fatal("private ignored qualification directory required")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal("fresh qualification directory required")
	}
	budget := 3 * time.Minute
	if os.Getenv("PDS_QUALIFY_NATIVE_MEMBER_RECOVERY") == "true" {
		budget = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	docker := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", args...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("disposable Docker %s failed", args[0])
		}
		return out
	}
	inspect := func(name string) map[string]any {
		t.Helper()
		var rows []map[string]any
		if json.Unmarshal(docker("inspect", name), &rows) != nil || len(rows) != 1 {
			t.Fatal("Docker inspection unavailable")
		}
		return rows[0]
	}
	protocol := inspect("pds-ass-runtime-protocol-fixtures-1")
	config := protocol["Config"].(map[string]any)
	labels := config["Labels"].(map[string]any)
	if config["Image"] != nodeagent.PDSImage || labels["com.docker.compose.project"] != "pds-ass-runtime" || labels["com.docker.compose.service"] != "protocol-fixtures" {
		t.Fatal("canonical disposable protocol fixture required")
	}
	command, _ := json.Marshal(config["Cmd"])
	if string(command) != `["node","/fixtures/protocol-services.mjs"]` {
		t.Fatal("fixed protocol command required")
	}
	protocolID := protocol["Id"].(string)
	wasRunning := protocol["State"].(map[string]any)["Running"].(bool)
	if os.Getenv("PDS_QUALIFY_NATIVE_MEMBER_RECOVERY") == "true" && wasRunning {
		t.Fatal("cold recovery requires the shared protocol fixture to be initially stopped")
	}
	socket := strings.TrimSpace(string(docker("context", "inspect", "--format", "{{.Endpoints.docker.Host}}")))
	if !strings.HasPrefix(socket, "unix:///") {
		t.Fatal("local Unix Docker socket required")
	}
	state := filepath.Join(root, "agent-state")
	runtime, err := nodeagent.NewDocker(strings.TrimPrefix(socket, "unix://"), state)
	if err != nil {
		t.Fatal(err)
	}
	runtime.EnableLocalIdentityFixtures()
	capacity, err := runtime.Capacity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Retained stopped namespaces still reserve slots. Admit exactly one new test
	// namespace without pruning or starting them; measured host limits still apply.
	requestedCeiling := len(capacity.InstanceIDs) + 1
	blocked := func(reason string) {
		t.Helper()
		receipt := map[string]any{"result": "blocked", "stage": "capacity_preflight", "reason": reason, "retainedNamespaces": len(capacity.InstanceIDs), "requestedCeiling": requestedCeiling, "protocolOriginallyRunning": wasRunning, "protocolFixtureStarted": false, "tenantAllocated": false}
		data, writeErr := json.MarshalIndent(receipt, "", "  ")
		if writeErr != nil || os.WriteFile(filepath.Join(root, "preflight.json"), data, 0600) != nil {
			t.Fatal("capacity preflight receipt unavailable")
		}
		t.Fatalf("native qualification capacity blocked (%s): %d retained namespaces; no fixture started or tenant allocated; see private preflight.json", reason, len(capacity.InstanceIDs))
	}
	if err = runtime.SetCapacityCeiling(requestedCeiling); err != nil {
		blocked("bounded_namespace_ceiling")
	}
	capacity, err = runtime.Capacity(ctx)
	if err != nil {
		t.Fatal("capacity preflight read failed before fixture startup")
	}
	if capacity.MaximumSlots <= len(capacity.InstanceIDs) {
		blocked("measured_host_capacity")
	}
	if !wasRunning {
		t.Cleanup(func() {
			cmd := exec.Command("docker", "stop", "--time", "10", protocolID)
			if cmd.Run() != nil {
				t.Error("protocol fixture stop failed")
			}
		})
		docker("start", protocolID)
	}
	f := newStoreFixture(t)
	id := f.deployment
	names := []string{"pds-ass-tenant-" + id, "pds-ass-edge-" + id, "pds-ass-enrollment-" + id}
	report := map[string]any{"deployment": id, "image": nodeagent.PDSImage, "protocolFixtureReused": true, "protocolOriginallyRunning": wasRunning, "physicalThirtyDayRun": false, "nativeDeactivationAttempted": false, "result": "failed"}
	t.Cleanup(func() {
		stopped := []string{}
		for _, name := range names {
			cmd := exec.Command("docker", "inspect", "--format", `{{index .Config.Labels "tv.subcult.pds-ass.instance"}}`, name)
			out, e := cmd.Output()
			if e != nil {
				continue
			}
			if strings.TrimSpace(string(out)) != id {
				t.Error("foreign cleanup scope refused")
				continue
			}
			if exec.Command("docker", "stop", "--time", "10", name).Run() != nil {
				t.Error("tenant stop failed")
			} else {
				stopped = append(stopped, name)
			}
		}
		// Remove only this fresh tenant's attachment from the shared protocol fixture.
		network := "pds-ass-tenant-" + id + "-network"
		labelOut, e := exec.Command("docker", "network", "inspect", "--format", `{{index .Labels "tv.subcult.pds-ass.instance"}}`, network).Output()
		if e == nil && strings.TrimSpace(string(labelOut)) == id {
			if exec.Command("docker", "network", "disconnect", network, protocolID).Run() != nil {
				t.Error("fresh protocol network disconnect failed")
			} else {
				report["protocolTenantAttachmentRemoved"] = true
			}
		}
		report["stoppedContainers"] = stopped
		report["volumesRetained"] = true
		raw, _ := json.MarshalIndent(report, "", "  ")
		if os.WriteFile(filepath.Join(root, "report.json"), raw, 0600) != nil {
			t.Error("qualification receipt unavailable")
		}
	})
	readyUntil := time.Now().Add(90 * time.Second)
	for {
		current, e := runtime.Ensure(ctx, id)
		if e != nil {
			t.Fatal("fresh guarded native runtime unavailable", e)
		}
		if current.Healthy {
			break
		}
		if time.Now().After(readyUntil) {
			t.Fatal("fresh guarded native runtime did not become healthy")
		}
		time.Sleep(500 * time.Millisecond)
	}
	ownerTicket := memberKey(1000).Key
	enrollment, err := runtime.Enroll(ctx, id, ownerTicket)
	if err != nil || enrollment.Code == "" {
		t.Fatal("native owner invitation unavailable", err, enrollment.State)
	}
	endpoint, _ := url.Parse(nodeagent.LocalEndpoint(id))
	cert, err := os.ReadFile(filepath.Join(state, id+"-tls.pem"))
	if err != nil {
		t.Fatal(err)
	}
	nativeRoots := x509.NewCertPool()
	if !nativeRoots.AppendCertsFromPEM(cert) {
		t.Fatal("native fixture certificate missing")
	}
	nativeClient := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: nativeRoots, ServerName: endpoint.Hostname()}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", endpoint.Port()))
	}}}
	t.Cleanup(nativeClient.CloseIdleConnections)
	native := func(method string, body any, token string) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", endpoint.String()+"/xrpc/"+method, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := nativeClient.Do(req)
		if e != nil {
			t.Fatal("native fixture transport failed")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("native %s returned %d", method, response.StatusCode)
		}
		var result map[string]any
		if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil {
			t.Fatal("native fixture response invalid")
		}
		return result
	}
	randomToken := func() string {
		var b [24]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(b[:])
	}
	ownerHandle := "ow-" + id[:6] + ".test"
	ownerPassword := randomToken()
	owner := native("com.atproto.server.createAccount", map[string]any{"handle": ownerHandle, "email": "owner-" + id[:6] + "@fixture.invalid", "password": ownerPassword, "inviteCode": enrollment.Code}, "")
	ownerDID := owner["did"].(string)
	observedOwner, e := runtime.Enroll(ctx, id, ownerTicket)
	if e != nil || observedOwner.DID != ownerDID || observedOwner.State != "account_created" {
		t.Fatal("native owner completion not reconciled")
	}

	// Seed only the already-existing owner precondition; member/removal requests use HTTP below.
	if _, err = f.pool.Exec(ctx, `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,message) VALUES($1,$2,gen_random_uuid(),'account_created',$3,$4,'Native owner created in qualification')`, id, f.owner, ownerDID, ownerHandle); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT organization_id,$1,'administrator','active' FROM deployments WHERE id=$2`, f.bob, id); err != nil {
		t.Fatal(err)
	}
	f.store.MemberAdministrators = true
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "test", "http://localhost/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	t.Cleanup(issuerServer.Close)
	owners, err := auth.New(ctx, f.pool, auth.Settings{Issuer: issuerURL, ClientID: "test", Origin: "http://localhost", Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	token := randomToken()[:43]
	hash := sha256.Sum256([]byte(token))
	if _, err = f.pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'native-window-csrf',now()+interval '1 hour')`, hash[:], f.bob); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	f.store.RegisterMembers(owners, true)(mux)
	api := func(path string, body any) map[string]any {
		t.Helper()
		method := "GET"
		var raw []byte
		if body != nil {
			method = "POST"
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: "pds-local-session", Value: token})
		req.Header.Set("Origin", "http://localhost")
		req.Header.Set("X-CSRF-Token", "native-window-csrf")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("management %s returned %d", path, w.Code)
		}
		var value map[string]any
		if json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatal("management response invalid")
		}
		return value
	}
	var nodeID string
	if err = f.pool.QueryRow(ctx, `SELECT node_id::text FROM instance_placements WHERE deployment_id=$1`, id).Scan(&nodeID); err != nil {
		t.Fatal(err)
	}
	agentTLS, client, pin := nativeWindowTLS(t, root, nodeID)
	startAgent := func(runtime *nodeagent.Docker) *httptest.Server {
		server := httptest.NewUnstartedServer(nodeagent.Authenticate(nodeagent.Handler(runtime)))
		server.TLS = agentTLS
		server.StartTLS()
		if _, err = f.pool.Exec(ctx, `UPDATE hosting_nodes SET agent_endpoint=$2,public_key_hash=$3 WHERE id=$1`, nodeID, server.URL, pin); err != nil {
			server.Close()
			t.Fatal(err)
		}
		return server
	}
	agentServer := startAgent(runtime)
	t.Cleanup(func() { agentServer.Close() })
	invitation := api("/api/deployments/"+id+"/members/invitations", map[string]string{"idempotencyKey": memberKey(1001).Key})
	invitationID := invitation["id"].(string)
	memberWorker := &MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Queue: f.store.Queue, Agent: client}
	if err = memberWorker.Work(ctx, memberJob(invitationID, 1)); err != nil {
		t.Fatal(err)
	}
	invitation = api("/api/member-invitations/"+invitationID, nil)
	memberHandle := "mb-" + id[:6] + ".test"
	memberPassword := randomToken()
	member := native("com.atproto.server.createAccount", map[string]any{"handle": memberHandle, "email": "member-" + id[:6] + "@fixture.invalid", "password": memberPassword, "inviteCode": invitation["code"]}, "")
	memberDID := member["did"].(string)
	memberToken := member["accessJwt"].(string)
	api("/api/member-invitations/"+invitationID+"/refresh", map[string]any{})
	if err = memberWorker.Work(ctx, memberJob(invitationID, 1)); err != nil {
		t.Fatal(err)
	}
	if got := api("/api/member-invitations/"+invitationID, nil); got["state"] != "accepted" || got["did"] != memberDID {
		t.Fatal("native member acceptance not retained")
	}
	post := func(text string) {
		native("com.atproto.repo.createRecord", map[string]any{"repo": memberDID, "collection": "app.bsky.feed.post", "record": map[string]any{"$type": "app.bsky.feed.post", "text": text, "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}}, memberToken)
	}
	post("Before qualification migration window")
	removalPath := "/api/member-invitations/" + invitationID + "/removal"
	if api(removalPath, nil)["state"] != "not_requested" {
		t.Fatal("unexpected existing removal")
	}
	request := map[string]string{"idempotencyKey": memberKey(1002).Key}
	requested := api(removalPath, request)
	removalID := requested["id"].(string)
	worker := &MemberRemovalWorker{Pool: f.pool, Agent: client}
	requireRemovalSnooze(t, worker.Work(ctx, removalJob(removalID, 1)))
	window := api(removalPath, nil)
	if window["state"] != "migration_window" || window["did"] != memberDID {
		t.Fatal("native window missing")
	}
	start, e1 := time.Parse(time.RFC3339Nano, window["hostRequestedAt"].(string))
	end, e2 := time.Parse(time.RFC3339Nano, window["deactivateAfter"].(string))
	if e1 != nil || e2 != nil || end.Sub(start) != nodeagent.MemberMigrationWindow {
		t.Fatal("native window duration differs")
	}
	if replay := api(removalPath, request); replay["id"] != removalID || replay["deactivateAfter"] != window["deactivateAfter"] {
		t.Fatal("management replay changed native window")
	}
	post("Inside qualification migration window")
	// Recreate the authenticated server and Docker broker from retained disk state.
	agentServer.Close()
	runtime, err = nodeagent.NewDocker(strings.TrimPrefix(socket, "unix://"), state)
	if err != nil {
		t.Fatal(err)
	}
	runtime.EnableLocalIdentityFixtures()
	agentServer = startAgent(runtime)
	var peer nodeagent.Peer
	if err = f.pool.QueryRow(ctx, `SELECT id::text,agent_endpoint,public_key_hash FROM hosting_nodes WHERE id=$1`, nodeID).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		t.Fatal(err)
	}
	replay, err := client.MemberExit(ctx, peer, id, invitationID)
	if err != nil || replay.State != "migration_window" || replay.DeactivateAfter != end.UnixMilli() {
		t.Fatal("reconstructed broker changed retained deadline", err)
	}
	worker = &MemberRemovalWorker{Pool: f.pool, Agent: client}
	requireRemovalSnooze(t, worker.Work(ctx, removalJob(removalID, 2)))
	post("After qualification broker reconstruction")
	receipt, err := f.store.GetMemberRemoval(ctx, f.owner, invitationID)
	if err != nil || len(receipt.Events) != 2 || receipt.RequestedBy != f.bob || receipt.Events[0].ActorID == nil || *receipt.Events[0].ActorID != f.bob || receipt.Events[1].ActorID != nil {
		t.Fatal("native removal audit mismatch")
	}
	summary, err := f.store.ListMembers(ctx, f.owner, id)
	if err != nil || summary.Members != 1 || len(summary.Accounts) != 1 {
		t.Fatal("native window changed accepted quota")
	}
	if os.Getenv("PDS_QUALIFY_NATIVE_MEMBER_RECOVERY") == "true" {
		report["coldRecovery"] = qualifyMemberNativeRecovery(t, ctx, root, id, []nativeRecoveryLogin{{ownerHandle, ownerPassword, ownerDID}, {memberHandle, memberPassword, memberDID}})
	}
	report["result"] = "passed"
	report["ownerDID"] = ownerDID
	report["memberDID"] = memberDID
	report["invitationID"] = invitationID
	report["removalID"] = removalID
	report["deactivateAfter"] = end
	report["checks"] = []string{"administrator HTTP invitation and native signup", "accepted native identity", "HTTP removal through registered mTLS node", "exact retained 30-day window", "same-key replay", "native writes before and during window", "broker/server reconstruction preserves deadline and writes", "real manager and worker audit separation", "accepted quota retained"}
	t.Log("Native initial-window qualification receipt:", filepath.Join(root, "report.json"))
}

func nativeWindowTLS(t *testing.T, root, nodeID string) (*tls.Config, *nodeagent.Client, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "native-window-ca"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	issue := func(name string, usage x509.ExtKeyUsage) (tls.Certificate, []byte, []byte) {
		leaf, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &leaf.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		private, err := x509.MarshalPKCS8PrivateKey(leaf)
		if err != nil {
			t.Fatal(err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			t.Fatal(err)
		}
		return pair, certPEM, keyPEM
	}
	server, _, _ := issue("pds-ass-node-"+nodeID, x509.ExtKeyUsageServerAuth)
	_, worker, workerKey := issue("pds-ass-worker", x509.ExtKeyUsageClientAuth)
	for name, data := range map[string][]byte{"ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), "worker.pem": worker, "worker.key": workerKey} {
		if os.WriteFile(filepath.Join(root, name), data, 0600) != nil {
			t.Fatal("private transport fixture unavailable")
		}
	}
	client, err := nodeagent.NewClient(filepath.Join(root, "ca.pem"), filepath.Join(root, "worker.pem"), filepath.Join(root, "worker.key"))
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(public)
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{server}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}, client, pin[:]
}
