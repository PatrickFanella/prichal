package managednamespace

import (
	"strings"
	"testing"
)

func TestGeneratedNamesKeepEndpointHandleAndExistingServicesSeparate(t *testing.T) {
	id := "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
	endpoint, err := Endpoint(id)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := Handle(id)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv" || handle != "at-2395105c995847b.subcult.tv" || endpoint == handle {
		t.Fatal(endpoint, handle)
	}
	if label := strings.Split(handle, ".")[0]; len(label) != HandleLabelLength {
		t.Fatalf("handle label %q exceeds the PDS limit", label)
	}
	// The first staging-02 account (ticket 49aac16a-…) holds exactly this generated handle.
	if live, _ := Handle("49aac16a-2663-4eb4-a95d-4c5c3029506e"); live != "at-49aac16a26634eb.subcult.tv" {
		t.Fatal(live)
	}
	for _, host := range []string{endpoint, handle} {
		if len(strings.Split(host, ".")) != 3 || len(strings.Split(host, ".")[0]) > 63 {
			t.Fatal("not first-level", host)
		}
	}
	for _, bad := range []string{"", strings.ToUpper(id), "../../etc/passwd", "alice", "prichal.subcult.tv", id + "\n", "00000000-0000-0000-0000-000000000000", "2395105c-9958-47b5-7e8b-5b0424cbfc4f"} {
		if _, err := Handle(bad); err == nil {
			t.Fatalf("invalid handle identity accepted: %q", bad)
		}
		if _, err := Endpoint(bad); err == nil {
			t.Fatalf("invalid identity accepted: %q", bad)
		}
	}
}
