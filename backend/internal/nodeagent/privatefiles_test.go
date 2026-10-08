package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateGuardRejectsReaderDrift(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	directory := t.TempDir()
	runtime := &Docker{directory: directory}
	network := "pds-ass-tenant-" + id + "-network"
	policy := filepath.Join(directory, id+"-enrollment")
	if err := os.Mkdir(policy, 0700); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"Config":          map[string]any{"User": privateFileUser(), "Image": PDSImage, "Labels": map[string]string{label: id, roleLabel: "enrollment-guard"}, "Cmd": []string{"node", "--input-type=module", "-e", enrollmentGuardScript}, "Env": []string{"PDS_GUARD_INSTANCE=" + id}},
		"HostConfig":      map[string]any{"ReadonlyRootfs": true, "Memory": 64 << 20, "NanoCpus": 125000000, "PidsLimit": 32, "NetworkMode": network, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Binds": []string{policy + ":/enrollment:ro"}},
		"Mounts":          []map[string]any{{"Type": "bind", "Source": policy, "Destination": "/enrollment", "RW": false}},
		"NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var current inspected
	if err = json.Unmarshal(data, &current); err != nil {
		t.Fatal(err)
	}
	if err = runtime.validateEnrollmentGuard(id, current); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"", "0", "12345:12345"} {
		current.Config.User = user
		if err = runtime.validateEnrollmentGuard(id, current); err == nil {
			t.Fatal("accepted a changed private-file reader")
		}
	}
}

func TestIngressWritablePathsPreserveImageDefaultsAndRejectDrift(t *testing.T) {
	valid := append(ingressEnvironment(), "PATH=/usr/bin", "CADDY_VERSION=v2.10.2")
	if !validIngressEnvironment(valid) {
		t.Fatal("image defaults rejected")
	}
	for _, environment := range [][]string{
		nil,
		{"XDG_CONFIG_HOME=/config", "XDG_DATA_HOME=/tmp/caddy-data"},
		append(valid, "XDG_CONFIG_HOME=/tmp/caddy-config"),
	} {
		if validIngressEnvironment(environment) {
			t.Fatal("missing, changed or duplicate writable path accepted")
		}
	}
}
