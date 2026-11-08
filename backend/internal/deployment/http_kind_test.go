package deployment

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnavailableKindExplainsTheDeploymentMode(t *testing.T) {
	for err, want := range map[error]string{ErrKindUnavailable: "Create a local PDS", ErrInvalid: "lowercase server name"} {
		w := httptest.NewRecorder()
		operationReply(w, Operation{}, err)
		var body map[string]string
		if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &body) != nil || !strings.Contains(body["error"], want) {
			t.Fatalf("%v: %d %s", err, w.Code, w.Body.String())
		}
	}
}
