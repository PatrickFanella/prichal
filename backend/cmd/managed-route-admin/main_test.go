package main

import (
	"os"
	"strings"
	"testing"
)

func TestOperatorCommandsRejectAmbiguousScopeBeforeDatabase(t *testing.T) {
	for _, entry := range []struct {
		args     []string
		expected string
	}{
		{[]string{"reserve", "--deployment", "2395105c-9958-47b5-ae8b-5b0424cbfc4f", "--node", "ignored"}, "reserve requires only"},
		{[]string{"reserve-empty", "--deployment", "2395105c-9958-47b5-ae8b-5b0424cbfc4f", "--node", "known"}, "confirm-empty-fixture"},
		{[]string{"observe", "--deployment", "2395105c-9958-47b5-ae8b-5b0424cbfc4f", "--node", "known"}, "--key"},
		{[]string{"export", "--out", "unused", "--deployment", "2395105c-9958-47b5-ae8b-5b0424cbfc4f"}, "export requires only"},
		{[]string{"reserve-empty", "--deployment", "2395105c-9958-47b5-ae8b-5b0424cbfc4f", "--node", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "--confirm-empty-fixture"}, "explicit staging-02 scope"},
	} {
		t.Run(strings.Join(entry.args, " "), func(t *testing.T) {
			t.Setenv("PDS_MODE", "local")
			t.Setenv("PDS_STAGING_PROVISIONING", "")
			t.Setenv("PDS_DATABASE_URL", "postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local?sslmode=disable")
			previous := os.Args
			os.Args = append([]string{"managed-route-admin"}, entry.args...)
			defer func() { os.Args = previous }()
			if err := run(); err == nil || !strings.Contains(err.Error(), entry.expected) {
				t.Fatalf("ambiguous scope accepted: %v", err)
			}
		})
	}
}
