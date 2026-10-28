package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateOutputCannotReplaceFilesOrSymlinks(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "live.Caddyfile")
	if err := os.WriteFile(live, []byte("retained configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.Caddyfile")
	if err := os.Symlink(live, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{live, link} {
		if err := writeCandidate(path, "replacement"); err == nil {
			t.Fatalf("existing output replaced: %s", path)
		}
	}
	retained, err := os.ReadFile(live)
	if err != nil || string(retained) != "retained configuration" {
		t.Fatal("live configuration changed")
	}
	candidate := filepath.Join(root, "candidate.Caddyfile")
	if err := writeCandidate(candidate, "reviewable configuration"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(candidate)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("candidate privacy not retained")
	}
}
