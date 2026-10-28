package config

import "testing"

func TestLocalListenerCannotBePublic(t *testing.T) {
	t.Setenv("PDS_DATABASE_URL", "postgres://local.invalid/pds_local")
	t.Setenv("PDS_MODE", "local")
	t.Setenv("PDS_OIDC_ISSUER", "http://localhost:5556")
	t.Setenv("PDS_OIDC_CLIENT_ID", "pds-local")
	t.Setenv("PDS_PUBLIC_ORIGIN", "http://localhost:5173")
	t.Setenv("PDS_LISTEN_ADDRESS", "127.0.0.1:8080")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PDS_LISTEN_ADDRESS", "0.0.0.0:8080")
	if _, err := Load(); err == nil {
		t.Fatal("local mode accepted public listener")
	}
	t.Setenv("PDS_LISTEN_ADDRESS", "127.0.0.1:8080")
	t.Setenv("PDS_MODE", "")
	if _, err := Load(); err == nil {
		t.Fatal("default production mode accepted HTTP fixture configuration")
	}
}

func TestStagingProvisioningOptIn(t *testing.T) {
	for _, mode := range []string{"", "local", "production", "pilot", "staging"} {
		for _, value := range []string{"", "false", "true", "1"} {
			t.Run(mode+"/"+value, func(t *testing.T) {
				t.Setenv("PDS_STAGING_PROVISIONING", value)
				enabled, err := LoadStagingProvisioning(mode)
				valid := value == "" || value == "true" && mode == "production"
				if (err == nil) != valid || enabled != (value == "true" && mode == "production") {
					t.Fatalf("enabled=%v error=%v", enabled, err)
				}
			})
		}
	}
}

func TestStagingAPIKeepsProductionAuthenticationRequirements(t *testing.T) {
	t.Setenv("PDS_DATABASE_URL", "postgres://staging.invalid/management")
	t.Setenv("PDS_MODE", "production")
	t.Setenv("PDS_STAGING_PROVISIONING", "true")
	t.Setenv("PDS_OIDC_ISSUER", "https://issuer.example.test")
	t.Setenv("PDS_OIDC_CLIENT_ID", "private-staging")
	t.Setenv("PDS_PUBLIC_ORIGIN", "https://management.example.test")
	t.Setenv("PDS_LISTEN_ADDRESS", "127.0.0.1:8080")
	cfg, err := Load()
	if err != nil || !cfg.StagingProvisioning {
		t.Fatalf("private staging configuration: %+v %v", cfg, err)
	}
	t.Setenv("PDS_OIDC_ISSUER", "http://localhost:5556")
	if _, err := Load(); err == nil {
		t.Fatal("staging opt-in accepted local issuer with production authentication")
	}
}

func TestManagedIntentsRequireExplicitProductionStaging(t *testing.T) {
	for _, mode := range []string{"", "local", "production"} {
		for _, staging := range []bool{false, true} {
			for _, value := range []string{"", "false", "true", "1"} {
				t.Run(mode+"/"+value+map[bool]string{false: "/off", true: "/staging"}[staging], func(t *testing.T) {
					t.Setenv("PDS_STAGING_MANAGED_INTENTS", value)
					enabled, err := LoadStagingManagedIntents(mode, staging)
					want := value == "true" && mode == "production" && staging
					if enabled != want || (err == nil) != (value == "" || want) {
						t.Fatalf("enabled=%v error=%v", enabled, err)
					}
				})
			}
		}
	}
}

func TestManagedRuntimeRequiresExplicitProductionStaging(t *testing.T) {
	for _, mode := range []string{"", "local", "pilot", "production"} {
		for _, staging := range []bool{false, true} {
			for _, value := range []string{"", "true", "false", "1"} {
				t.Setenv("PDS_STAGING_MANAGED_RUNTIME", value)
				enabled, err := LoadStagingManagedRuntime(mode, staging)
				want := value == "true" && mode == "production" && staging
				if enabled != want || (err == nil) != (value == "" || want) {
					t.Fatalf("mode=%s staging=%v value=%s enabled=%v error=%v", mode, staging, value, enabled, err)
				}
			}
		}
	}
}

func TestPublicChecksRequireAllWorkerOptIns(t *testing.T) {
	for _, mode := range []string{"", "local", "production"} {
		for _, staging := range []bool{false, true} {
			for _, managed := range []bool{false, true} {
				for _, value := range []string{"", "true", "false", "1"} {
					t.Setenv("PDS_STAGING_MANAGED_PUBLIC_CHECKS", value)
					got, err := LoadStagingManagedPublicChecks(mode, staging, managed)
					want := value == "true" && mode == "production" && staging && managed
					if got != want || (err == nil) != (value == "" || want) {
						t.Fatalf("mode=%s staging=%v managed=%v value=%s got=%v error=%v", mode, staging, managed, value, got, err)
					}
				}
			}
		}
	}
}
