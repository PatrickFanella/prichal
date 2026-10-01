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
