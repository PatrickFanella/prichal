package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateLocalOperatorPolicy(t *testing.T) {
	t.Setenv("PDS_OPERATOR_FILE", "")
	t.Setenv("PDS_LOCAL_OPERATOR_FILE", "")
	if cfg, err := LoadSettings(false, "http://localhost:5556"); err != nil || len(cfg.Identities) != 0 {
		t.Fatal("default policy open")
	}
	path := filepath.Join(t.TempDir(), "operators.json")
	t.Setenv("PDS_LOCAL_OPERATOR_FILE", path)
	for _, tc := range []struct {
		name, body string
		mode       os.FileMode
		local      bool
		valid      bool
	}{
		{"valid", `[{"issuer":"http://localhost:5556","subject":"alice","role":"inspect"}]`, 0600, true, true},
		{"public", `[]`, 0644, true, false}, {"live", `[]`, 0600, false, false}, {"email authority", `[{"email":"fixture@example.test","role":"operate"}]`, 0600, true, false},
		{"other issuer", `[{"issuer":"http://localhost:5557","subject":"alice","role":"operate"}]`, 0600, true, false},
		{"duplicate", `[{"issuer":"http://localhost:5556","subject":"alice","role":"inspect"},{"issuer":"http://localhost:5556","subject":"alice","role":"operate"}]`, 0600, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSettings(tc.local, "http://localhost:5556")
			if (err == nil) != tc.valid {
				t.Fatal("policy validation mismatch")
			}
		})
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PDS_LOCAL_OPERATOR_FILE", link)
	if _, err := LoadSettings(true, "http://localhost:5556"); err == nil {
		t.Fatal("symlink policy accepted")
	}
}

func TestProductionInspectionPolicy(t *testing.T) {
	t.Setenv("PDS_LOCAL_OPERATOR_FILE", "")
	path := filepath.Join(t.TempDir(), "operators.json")
	t.Setenv("PDS_OPERATOR_FILE", path)
	for _, tc := range []struct {
		name, issuer, subject, role string
		local, valid                bool
	}{
		{"verified subject", "https://git.subcult.tv", "1", "inspect", false, true},
		{"mutation policy", "https://git.subcult.tv", "1", "operate", false, false},
		{"local mode", "https://git.subcult.tv", "1", "inspect", true, false},
		{"HTTP issuer", "http://localhost:5556", "alice", "inspect", false, false},
		{"missing subject", "https://git.subcult.tv", "", "inspect", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `[{"issuer":"` + tc.issuer + `","subject":"` + tc.subject + `","role":"` + tc.role + `"}]`
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSettings(tc.local, tc.issuer)
			if (err == nil) != tc.valid {
				t.Fatal("production inspection validation mismatch")
			}
		})
	}
	t.Setenv("PDS_LOCAL_OPERATOR_FILE", path)
	if _, err := LoadSettings(false, "https://git.subcult.tv"); err == nil {
		t.Fatal("ambiguous policies accepted")
	}
}
