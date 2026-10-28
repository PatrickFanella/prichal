//go:build linux

package nodeagent

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// LockStateDirectory fences the server against local operator conversion/recovery.
// The caller retains the returned descriptor until all native execution has stopped.
func LockStateDirectory(directory string) (*os.File, error) {
	info, err := os.Lstat(directory)
	if err != nil || !filepath.IsAbs(directory) || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private native state directory required")
	}
	path := filepath.Join(directory, ".native-owner.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	info, err = os.Lstat(path)
	opened, statErr := file.Stat()
	if err != nil || statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("private regular native ownership lock required")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("native state already has an execution owner")
	}
	return file, nil
}
