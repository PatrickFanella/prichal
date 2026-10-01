package nodeagent

import (
	"context"
	"testing"
	"time"
)

func TestUsageRejectsUntrustedBoundsIdentityAndFreshness(t *testing.T) {
	const id = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	base := Usage{InstanceID: id, DataBytes: 1000, BlobBytes: 68, DataFiles: 20, BlobFiles: 1, Accounts: 1, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}
	for _, entry := range []struct {
		name   string
		change func(*Usage)
	}{{"different tenant", func(u *Usage) { u.InstanceID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" }}, {"negative bytes", func(u *Usage) { u.DataBytes = -1 }}, {"overflow bytes", func(u *Usage) { u.BlobBytes = 1 << 62 }}, {"file overflow", func(u *Usage) { u.DataFiles = 1 << 62; u.BlobFiles = 1 << 62 }}, {"account bound", func(u *Usage) { u.Accounts = 1001 }}, {"invalid headroom", func(u *Usage) { u.FilesystemAvailableBytes = u.FilesystemBytes + 1 }}, {"stale", func(u *Usage) { u.ObservedAtMillis = time.Now().Add(-2 * time.Minute).UnixMilli() }}, {"future", func(u *Usage) { u.ObservedAtMillis = time.Now().Add(time.Minute).UnixMilli() }}} {
		t.Run(entry.name, func(t *testing.T) {
			value := base
			entry.change(&value)
			if value.Validate(id) == nil {
				t.Fatal("untrusted usage accepted")
			}
		})
	}
	if err := base.Validate(id); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Docker{}).Usage(context.Background(), "../../host"); err == nil {
		t.Fatal("invalid usage path accepted")
	}
}
