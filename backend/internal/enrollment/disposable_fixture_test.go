package enrollment

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// keepQualificationFixtures reports whether a disposable local qualification
// should keep its stopped containers, networks and volumes for debugging.
func keepQualificationFixtures() bool {
	value := os.Getenv("PRICHAL_KEEP_FIXTURES")
	return value == "1" || value == "true"
}

var qualificationDocker = func(args ...string) ([]byte, error) {
	return exec.Command("docker", args...).Output()
}

var errForeignFixture = errors.New("foreign fixture ownership refused")

// removeOwnedDocker removes one container, network or volume only when its
// label key holds exactly owner. An absent object is not an error.
func removeOwnedDocker(kind, name, key, owner string) (bool, error) {
	var inspect, remove []string
	switch kind {
	case "container":
		inspect = []string{"inspect", "--type", "container", "--format", `{{index .Config.Labels "` + key + `"}}`, name}
		remove = []string{"rm", "--force", "--volumes", name}
	case "network", "volume":
		inspect = []string{kind, "inspect", "--format", `{{index .Labels "` + key + `"}}`, name}
		remove = []string{kind, "rm", name}
	default:
		return false, errors.New("unsupported fixture kind")
	}
	if key == "" || owner == "" {
		return false, errors.New("fixture ownership label required")
	}
	out, err := qualificationDocker(inspect...)
	if err != nil {
		return false, nil
	}
	if strings.TrimSpace(string(out)) != owner {
		return false, errForeignFixture
	}
	if _, err = qualificationDocker(remove...); err != nil {
		return false, errors.New("disposable fixture removal failed")
	}
	return true, nil
}

func TestRemoveOwnedDockerRequiresExactOwner(t *testing.T) {
	previous := qualificationDocker
	t.Cleanup(func() { qualificationDocker = previous })
	labels := map[string]string{"own": "fixture-1", "other": "fixture-2"}
	var removed []string
	qualificationDocker = func(args ...string) ([]byte, error) {
		name := args[len(args)-1]
		if args[0] == "rm" || (len(args) > 1 && args[1] == "rm") {
			removed = append(removed, strings.Join(args, " "))
			return nil, nil
		}
		label, ok := labels[name]
		if !ok {
			return nil, errors.New("no such object")
		}
		return []byte(label + "\n"), nil
	}
	if ok, err := removeOwnedDocker("container", "own", "tv.subcult.pds-ass.instance", "fixture-1"); !ok || err != nil {
		t.Fatal("owned container not removed", ok, err)
	}
	if ok, err := removeOwnedDocker("volume", "other", "tv.subcult.pds-ass.instance", "fixture-1"); ok || !errors.Is(err, errForeignFixture) {
		t.Fatal("foreign volume not refused", ok, err)
	}
	if ok, err := removeOwnedDocker("network", "missing", "tv.subcult.pds-ass.instance", "fixture-1"); ok || err != nil {
		t.Fatal("absent network should be skipped", ok, err)
	}
	if _, err := removeOwnedDocker("volume", "own", "tv.subcult.pds-ass.instance", ""); err == nil {
		t.Fatal("empty owner accepted")
	}
	if len(removed) != 1 || removed[0] != "rm --force --volumes own" {
		t.Fatal("unexpected removals", removed)
	}
}

func TestKeepQualificationFixturesIsExplicit(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "yes": false, "1": true, "true": true} {
		t.Setenv("PRICHAL_KEEP_FIXTURES", value)
		if keepQualificationFixtures() != want {
			t.Fatalf("PRICHAL_KEEP_FIXTURES=%q", value)
		}
	}
}
