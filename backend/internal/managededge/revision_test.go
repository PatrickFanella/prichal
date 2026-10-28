package managededge

import (
	"strings"
	"testing"
	"time"
)

func TestPublicationPinsImmutableRevisionAndRequiresAuthority(t *testing.T) {
	i, d, now := fixture(t)
	expires := now.Add(InventoryLifetime)
	i.Generation, i.ExpiresAt = 4, &expires
	got, err := RenderRevision(i, d, now, invitation)
	if err != nil || !strings.Contains(got, certificateMount+"/"+invitation+"/"+instance+".crt") {
		t.Fatal("revision did not pin immutable trust", err)
	}
	for _, revision := range []string{"", "../foreign", strings.ToUpper(invitation)} {
		if _, err := RenderRevision(i, d, now, revision); err == nil {
			t.Fatal("unsafe publication revision accepted")
		}
	}
	for _, mutate := range []func(){func() { i.Generation = 0 }, func() { i.Generation = 4; expires = now.Add(-time.Second) }} {
		mutate()
		if _, err := RenderRevision(i, d, now, invitation); err == nil {
			t.Fatal("unqualified publication authority accepted")
		}
	}
}
