package main

import (
	"strings"
	"testing"
)

func TestRetirementScopeBeforeDatabase(t *testing.T) {
	key := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	sha := strings.Repeat("a", 64)
	good := []string{"prepare", "--key", key, "--native-backup-sha256", sha, "--management-backup-sha256", sha, "--confirm-empty-staging-02"}
	if _, e := parse(good); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{good[:len(good)-1], append(append([]string{}, good...), "--deployment", instance), append(append([]string{}, good...), "--withdrawal-sha256", sha), {"release", "--key", key, "--confirm-empty-staging-02"}} {
		if _, e := parse(args); e == nil {
			t.Fatal("ambiguous retirement scope accepted")
		}
	}
	t.Setenv("PDS_MODE", "production")
	t.Setenv("PDS_STAGING_PROVISIONING", "true")
	t.Setenv("PDS_DATABASE_URL", "postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local?sslmode=disable")
	if e := run(good); e == nil || !strings.Contains(e.Error(), "dedicated staging-02") {
		t.Fatal("foreign registry accepted", e)
	}
}
