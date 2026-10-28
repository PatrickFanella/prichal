package nodeagent

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

// This opt-in validates the exact generated PDS environment in one fresh,
// network-disabled container. It does not reconcile a node or publish ingress.
func TestNativeManagedProtocolEnvironment(t *testing.T) {
	name := os.Getenv("PDS_TEST_MANAGED_NATIVE_CONTAINER")
	if name == "" {
		t.Skip("fresh disposable managed environment container not selected")
	}
	if !regexp.MustCompile(`^prichal-managed-env-[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`).MatchString(name) {
		t.Fatal("allocator-owned managed qualification name required")
	}
	id := strings.TrimPrefix(name, "prichal-managed-env-")
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocker("/var/run/docker.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnableManagedEndpoints(); err != nil {
		t.Fatal(err)
	}
	retainManagedReceipt(t, d, id)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if status, _ := d.call(ctx, "GET", "/containers/"+name+"/json", nil, nil); status != 404 {
		t.Fatal("qualification name is not fresh; refusing adoption")
	}
	var created struct{ ID string }
	secret := secrets{JWT: strings.Repeat("a", 64), Admin: strings.Repeat("b", 64), Rotation: strings.Repeat("0", 63) + "1"}
	payload := map[string]any{"Image": PDSImage, "Env": d.runtimeEnvironment(id, secret), "Labels": map[string]string{"tv.subcult.prichal.managed-env-qualification": name}, "HostConfig": map[string]any{"NetworkMode": "none", "Memory": int64(768 << 20), "NanoCpus": int64(1e9), "PidsLimit": 128, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Tmpfs": map[string]string{"/pds": "rw,mode=0700,size=67108864", "/blobs": "rw,mode=0700,size=8388608"}, "RestartPolicy": map[string]string{"Name": "no"}}}
	status, err := d.call(ctx, "POST", "/containers/create?name="+name, payload, &created)
	if err != nil || status != 201 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(created.ID) {
		t.Fatal("could not allocate fresh qualification container", err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 8*time.Second)
		defer done()
		var current inspected
		if _, err := d.call(cleanup, "GET", "/containers/"+created.ID+"/json", nil, &current); err != nil || current.ID != created.ID || current.Config.Labels["tv.subcult.prichal.managed-env-qualification"] != name {
			t.Error("qualification cleanup ownership unavailable")
			return
		}
		if _, err := d.call(cleanup, "DELETE", "/containers/"+created.ID+"?force=true&v=true", nil, nil); err != nil {
			t.Error("qualification cleanup failed", err)
		}
	}()
	if _, err := d.call(ctx, "POST", "/containers/"+created.ID+"/start", nil, nil); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Version       string `json:"version"`
		DID           string `json:"did"`
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization"`
		Token         string `json:"token"`
		Repositories  int    `json:"repositories"`
	}
	const probe = `(async()=>{const base='http://127.0.0.1:2583';const get=async(path)=>{const r=await fetch(base+path,{signal:AbortSignal.timeout(800)});if(!r.ok)throw Error('native read unavailable');return r.json()};let health;for(let i=0;i<20;i++){try{health=await get('/xrpc/_health');break}catch{await new Promise(r=>setTimeout(r,250))}}if(!health)throw Error('native startup unavailable');const server=await get('/xrpc/com.atproto.server.describeServer');const oauth=await get('/.well-known/oauth-authorization-server');const repos=await get('/xrpc/com.atproto.sync.listRepos?limit=1');console.log(JSON.stringify({version:health.version,did:server.did,issuer:oauth.issuer,authorization:oauth.authorization_endpoint,token:oauth.token_endpoint,repositories:repos.repos.length}))})().catch(()=>process.exit(1))`
	if err := d.executeFixed(ctx, created.ID, probe, nil, &result); err != nil {
		t.Fatal("pinned native environment did not start or advertise identity", err)
	}
	host, _ := managednamespace.Endpoint(id)
	origin := "https://" + host
	if result.Version == "" || result.DID != "did:web:"+host || result.Issuer != origin || result.Authorization != origin+"/oauth/authorize" || result.Token != origin+"/oauth/token" || result.Repositories != 0 {
		t.Fatal("native descriptor differs from managed identity", result)
	}
	t.Logf("network-disabled pinned PDS %s advertised exact managed DID/OAuth identity with zero repositories", result.Version)
}
