package main

import (
	"strconv"
	"testing"
)

func TestPartialAgentCredentialsFailBeforeDatabaseStartup(t *testing.T) {
	names := []string{"PDS_WORKER_AGENT_CA_FILE", "PDS_WORKER_AGENT_CERT_FILE", "PDS_WORKER_AGENT_KEY_FILE"}
	for mask := 1; mask < 7; mask++ {
		t.Run("mask-"+strconv.Itoa(mask), func(t *testing.T) {
			t.Setenv("PDS_MODE", "local")
			// Invalid database configuration must not obscure an incomplete transport.
			t.Setenv("PDS_DATABASE_URL", "invalid-database-url")
			for i, name := range names {
				value := ""
				if mask&(1<<i) != 0 {
					value = "/private/credential-path"
				}
				t.Setenv(name, value)
			}
			err := run()
			want := "PDS_WORKER_AGENT_CA_FILE, PDS_WORKER_AGENT_CERT_FILE and PDS_WORKER_AGENT_KEY_FILE must be configured together"
			if err == nil || err.Error() != want {
				t.Fatalf("expected credential configuration rejection, got %v", err)
			}
		})
	}
}

func TestAgentClientOptionalForLocalSimulation(t *testing.T) {
	for _, name := range []string{"PDS_WORKER_AGENT_CA_FILE", "PDS_WORKER_AGENT_CERT_FILE", "PDS_WORKER_AGENT_KEY_FILE"} {
		t.Setenv(name, "")
	}
	client, err := loadAgentClient()
	if err != nil || client != nil {
		t.Fatalf("unconfigured simulation transport: %v, %v", client, err)
	}
}

func TestInvalidAgentCredentialsFailBeforeDatabaseStartup(t *testing.T) {
	t.Setenv("PDS_MODE", "local")
	t.Setenv("PDS_DATABASE_URL", "invalid-database-url")
	missing := t.TempDir() + "/absent"
	for _, name := range []string{"PDS_WORKER_AGENT_CA_FILE", "PDS_WORKER_AGENT_CERT_FILE", "PDS_WORKER_AGENT_KEY_FILE"} {
		t.Setenv(name, missing)
	}
	if err := run(); err == nil || err.Error() != "worker agent CA required" {
		t.Fatalf("expected credential load rejection, got %v", err)
	}
}

func TestNonLocalWorkerRemainsGated(t *testing.T) {
	for _, mode := range []string{"", "production", "pilot", "staging"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PDS_MODE", mode)
			if err := run(); err == nil || err.Error() != "worker requires local mode or production mode with PDS_STAGING_PROVISIONING=true" {
				t.Fatalf("unsupported worker mode: %v", err)
			}
		})
	}
}

func TestStagingWorkerRequiresTransportBeforeDatabase(t *testing.T) {
	t.Setenv("PDS_MODE", "production")
	t.Setenv("PDS_STAGING_PROVISIONING", "true")
	t.Setenv("PDS_DATABASE_URL", "invalid-database-url")
	for _, name := range []string{"PDS_WORKER_AGENT_CA_FILE", "PDS_WORKER_AGENT_CERT_FILE", "PDS_WORKER_AGENT_KEY_FILE"} {
		t.Setenv(name, "")
	}
	if err := run(); err == nil || err.Error() != "staging worker requires authenticated agent credentials" {
		t.Fatalf("unconfigured staging transport accepted: %v", err)
	}
}
