package main

import (
	"encoding/json"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfirmationScopeRefusedBeforeDatabase(t *testing.T) {
	for _, change := range []string{"node", "accounts", "backup", "key", "registry"} {
		t.Run(change, func(t *testing.T) {
			t.Setenv("PDS_MODE", "production")
			t.Setenv("PDS_STAGING_PROVISIONING", "true")
			t.Setenv("PDS_DATABASE_URL", "postgres://pds_staging:synthetic@127.0.0.1:35435/pds_local?sslmode=disable")
			p := conversionProof{Version: 1, DeploymentID: stagingDeployment, NodeID: stagingNode, CreatedAt: time.Now().UTC(), BackupVerified: true, Request: deployment.EmptyConversionRequest{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", NativeBackupSHA256: strings.Repeat("a", 64), ManagementBackupSHA256: strings.Repeat("b", 64), CredentialsSHA256: strings.Repeat("c", 64), CertificateSHA256: strings.Repeat("d", 64), NativeReceiptSHA256: strings.Repeat("e", 64)}}
			switch change {
			case "node":
				p.NodeID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
			case "accounts":
				p.SourceAccounts = 1
			case "backup":
				p.BackupVerified = false
			case "key":
				p.Request.Key = "not-a-uuid"
			}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "proof.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			previous := os.Args
			os.Args = []string{"managed-conversion-confirm", "--proof", path, "--confirm-staging-02"}
			defer func() { os.Args = previous }()
			err = run()
			if err == nil || strings.Contains(err.Error(), "connect") {
				t.Fatal("invalid conversion reached database", err)
			}
			if change == "registry" && !strings.Contains(err.Error(), "dedicated staging-02") {
				t.Fatal("registry scope was not checked before transport", err)
			}
		})
	}
}
