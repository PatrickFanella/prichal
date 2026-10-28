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
	if endpoint != "pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv" || handle != "at-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv" || endpoint == handle {
		t.Fatal(endpoint, handle)
	}
	for _, host := range []string{endpoint, handle} {
		if len(strings.Split(host, ".")) != 3 || len(strings.Split(host, ".")[0]) > 63 {
			t.Fatal("not first-level", host)
		}
	}
	for _, bad := range []string{"", strings.ToUpper(id), "../../etc/passwd", "alice", "prichal.subcult.tv", id + "\n", "00000000-0000-0000-0000-000000000000", "2395105c-9958-47b5-7e8b-5b0424cbfc4f"} {
		if _, err := Endpoint(bad); err == nil {
			t.Fatalf("invalid identity accepted: %q", bad)
		}
	}
}
