package nodeagent

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
)

// This optional check reads only a newly allocated, network-isolated qualification
// container. The smoke suite has no Docker socket requirement.
func TestNativeDockerReleaseObservation(t *testing.T) {
	name := os.Getenv("PDS_TEST_RELEASE_CONTAINER")
	if name == "" {
		t.Skip("fresh disposable image qualification container not selected")
	}
	if !regexp.MustCompile(`^prichal-release-[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`).MatchString(name) {
		t.Fatal("disposable release qualification namespace required")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocker("/var/run/docker.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var current inspected
	if _, err = d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
		t.Fatal("qualification container unavailable")
	}
	if current.Config.Labels["tv.subcult.prichal.release-qualification"] != name || current.HostConfig.NetworkMode != "none" || len(current.Mounts) != 0 || len(current.HostConfig.PortBindings) != 0 || current.Config.Image != PDSImage {
		t.Fatal("qualification container policy mismatch")
	}
	value := d.runtimeRelease(ctx, current)
	if value == nil || !value.Valid() || value.ImageRef != current.Config.Image || value.ImageID != current.Image || value.Version == "" {
		t.Fatal("native image evidence unavailable or inconsistent")
	}
	t.Logf("observed image version %s, manifest %s, config %s", value.Version, value.Digest, value.ImageID)
}
