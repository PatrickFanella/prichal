package hostingexit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateExplicitLocalExitPolicy(t *testing.T) {
	t.Setenv("PDS_LOCAL_EXIT_POLICY_FILE", "")
	policy, err := LoadPolicy(true)
	if err != nil || policy != nil {
		t.Fatal("implicit exit policy")
	}
	directory := t.TempDir()
	file := filepath.Join(directory, "policy.json")
	if err = os.WriteFile(file, []byte(`{"version":1,"delaySeconds":30,"retentionDays":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PDS_LOCAL_EXIT_POLICY_FILE", file)
	policy, err = LoadPolicy(true)
	if err != nil || policy == nil || policy.RetentionDays != 7 {
		t.Fatal("valid local policy rejected")
	}
	if _, err = LoadPolicy(false); err == nil {
		t.Fatal("production accepted fixture policy")
	}
	if err = os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadPolicy(true); err == nil {
		t.Fatal("public policy accepted")
	}
	if err = os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"version":1,"delaySeconds":0,"retentionDays":7}`, `{"version":1,"delaySeconds":30,"retentionDays":0}`, `{"version":1,"delaySeconds":30,"retentionDays":7,"delete":true}`, `{"version":1,"delaySeconds":30,"retentionDays":7} {}`} {
		if err = os.WriteFile(file, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadPolicy(true); err == nil {
			t.Fatal("unsafe policy accepted")
		}
	}
	alias := filepath.Join(directory, "alias")
	if err = os.Symlink(file, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PDS_LOCAL_EXIT_POLICY_FILE", alias)
	if _, err = LoadPolicy(true); err == nil {
		t.Fatal("symlink policy accepted")
	}
}
