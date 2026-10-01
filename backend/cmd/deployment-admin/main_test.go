package main

import "testing"

func TestAdministrationRejectsUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name, mode, url string
		args            []string
	}{
		{"production", "production", "postgres://localhost/pds_local", []string{"status"}},
		{"remote", "local", "postgres://192.0.2.1/pds_local", []string{"status"}},
		{"wrong-database", "local", "postgres://127.0.0.1/customer", []string{"status"}},
		{"missing-review", "local", "postgres://127.0.0.1/pds_local", []string{"set", "--state", "closed"}},
		{"unknown-state", "local", "postgres://127.0.0.1/pds_local", []string{"set", "--state", "maybe", "--reason", "maintenance"}},
		{"extra-argument", "local", "postgres://127.0.0.1/pds_local", []string{"status", "unexpected"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PDS_MODE", tc.mode)
			t.Setenv("PDS_DATABASE_URL", tc.url)
			if err := run(tc.args); err == nil {
				t.Fatal("unsafe or unreviewed command accepted")
			}
		})
	}
}
