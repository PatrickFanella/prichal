package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type nativeRecoveryLogin struct {
	Handle   string `json:"handle"`
	Password string `json:"password"`
	DID      string `json:"did"`
}

func qualifyMemberNativeRecovery(t *testing.T, ctx context.Context, root, id string, logins []nativeRecoveryLogin) map[string]any {
	t.Helper()
	// The Node driver records each owned container before allocation. This cleanup
	// remains available even if its process is killed by the qualification deadline.
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(root, "cold-recovery", "resources.json"))
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Error("cold recovery resource receipt unavailable")
			return
		}
		var resources struct {
			ID         string   `json:"id"`
			Containers []string `json:"containers"`
		}
		if json.Unmarshal(data, &resources) != nil || !uuid.MatchString(resources.ID) {
			t.Error("cold recovery scope invalid")
			return
		}
		for _, name := range resources.Containers {
			if !strings.HasPrefix(name, "pds-ass-member-recovery-"+resources.ID+"-") {
				t.Error("foreign cold recovery cleanup refused")
				continue
			}
			out, e := exec.Command("docker", "inspect", "--format", `{{index .Config.Labels "pds-ass.member-recovery"}}`, name).Output()
			if e != nil {
				continue
			}
			if strings.TrimSpace(string(out)) != resources.ID {
				t.Error("foreign cold recovery label refused")
				continue
			}
			if exec.Command("docker", "stop", "--time", "10", name).Run() != nil {
				t.Error("cold recovery container stop failed")
			}
		}
	})
	script, err := filepath.Abs("../../../scripts/qualify-member-native-recovery.mjs")
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(logins)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "node", script, root, id)
	cmd.Dir = filepath.Join("..", "..", "..")
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("native cold recovery failed; inspect private cold-recovery receipt")
	}
	var result map[string]any
	if json.Unmarshal(output, &result) != nil || result["result"] != "passed" {
		t.Fatal("native cold recovery receipt invalid")
	}
	return result
}
