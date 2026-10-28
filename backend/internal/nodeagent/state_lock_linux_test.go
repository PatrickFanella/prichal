//go:build linux

package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeOwnershipLockRejectsConcurrentAndSymlinkOwners(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := LockStateDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockStateDirectory(dir); err == nil {
		second.Close()
		t.Fatal("second native owner accepted")
	}
	first.Close()
	second, err := LockStateDirectory(dir)
	if err != nil {
		t.Fatal("stopped owner did not release native state", err)
	}
	second.Close()
	if err := os.Rename(filepath.Join(dir, ".native-owner.lock"), filepath.Join(dir, "retained.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "retained.lock"), filepath.Join(dir, ".native-owner.lock")); err != nil {
		t.Fatal(err)
	}
	if owner, err := LockStateDirectory(dir); err == nil {
		owner.Close()
		t.Fatal("symlink native lock accepted")
	}
}
