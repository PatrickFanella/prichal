package main

import (
	"os"
	"strings"
	"testing"
)

func TestExplicitAndLegacyRetryTargets(t *testing.T) {
	key := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	deployment := "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"
	placement := "cccccccc-cccc-4ccc-accc-cccccccccccc"
	got, err := parseTarget([]string{"--deployment", deployment, "--node", placement, "--key", key, "--confirm-managed-endpoint"})
	if err != nil || got != (retryTarget{deployment, placement, key}) {
		t.Fatal("explicit selection differs", got, err)
	}
	got, err = parseTarget([]string{"--key", key, "--confirm-staging-02"})
	if err != nil || got != (retryTarget{instance, node, key}) {
		t.Fatal("legacy target changed", got, err)
	}
	for _, args := range [][]string{
		{"--key", key},
		{"--key", key, "--confirm-managed-endpoint"},
		{"--key", key, "--deployment", deployment, "--confirm-managed-endpoint"},
		{"--key", key, "--node", placement, "--confirm-managed-endpoint"},
		{"--key", key, "--deployment", deployment, "--node", placement, "--confirm-staging-02"},
		{"--key", key, "--confirm-staging-02", "--confirm-managed-endpoint"},
		{"--key", key, "--confirm-staging-02", "extra"},
		{"--key", "invalid", "--confirm-staging-02"},
		{"--key", key, "--deployment", "foreign.subcult.tv", "--node", placement, "--confirm-managed-endpoint"},
		{"--key", key, "--deployment", deployment, "--node", "../node", "--confirm-managed-endpoint"},
	} {
		if _, err := parseTarget(args); err == nil {
			t.Fatalf("unsafe target accepted: %q", args)
		}
	}
}

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

func TestExplicitTargetStillRequiresDedicatedRegistry(t *testing.T) {
	t.Setenv("PDS_MODE", "production")
	t.Setenv("PDS_STAGING_PROVISIONING", "true")
	t.Setenv("PDS_DATABASE_URL", "postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local")
	before := os.Args
	defer func() { os.Args = before }()
	os.Args = []string{"managed-public-retry", "--deployment", "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb", "--node", node, "--key", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "--confirm-managed-endpoint"}
	if err := run(); err == nil || err.Error() != "dedicated staging-02 management registry required" {
		t.Fatal("explicit target bypassed registry scope", err)
	}
}
