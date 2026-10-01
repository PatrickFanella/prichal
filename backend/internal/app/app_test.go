package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type probeStub struct{ err error }

func (p probeStub) Check(context.Context) error { return p.err }
func TestReadiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"ready", nil, 200, `"ready":true`},
		{"unavailable", errors.New("password=private-value"), 503, `"ready":false`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			Handler(probeStub{tc.err}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/readiness", nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "private-value") {
				t.Fatal("readiness leaked configuration")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("readiness must not be cached")
			}
		})
	}
}
func TestReadinessRejectsMutation(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(probeStub{}).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/readiness", nil))
	if w.Code != 405 {
		t.Fatalf("got %d", w.Code)
	}
}
