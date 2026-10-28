package main

import (
	"os"
	"strings"
	"testing"
)

func TestPublicRetryScopeBeforeDatabase(t *testing.T) {
	for _, bad := range []string{"mode", "key", "confirmation", "registry"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("PDS_MODE", "production")
			t.Setenv("PDS_STAGING_PROVISIONING", "true")
			t.Setenv("PDS_DATABASE_URL", "postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local")
			before := os.Args
			defer func() { os.Args = before }()
			os.Args = []string{"managed-public-retry", "--key", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "--confirm-staging-02"}
			switch bad {
			case "mode":
				t.Setenv("PDS_MODE", "local")
			case "key":
				os.Args[2] = "invalid"
			case "confirmation":
				os.Args = os.Args[:3]
			}
			err := run()
			if err == nil || strings.Contains(err.Error(), "connect") {
				t.Fatal("invalid scope reached database", err)
			}
		})
	}
}
