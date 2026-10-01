// Package app exposes the public management HTTP surface.
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Probe interface {
	Check(context.Context) error
	Version() int
}
type Readiness struct {
	Ready         bool   `json:"ready"`
	Database      string `json:"database"`
	SchemaVersion int    `json:"schemaVersion"`
}

func Handler(probe Probe, routes ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/readiness", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		result := Readiness{Database: "unavailable", SchemaVersion: probe.Version()}
		if probe.Check(ctx) == nil {
			result.Ready = true
			result.Database = "ready"
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	for _, register := range routes {
		register(mux)
	}
	return mux
}
