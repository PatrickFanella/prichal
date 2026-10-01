// Package nodeagent exposes a fixed lifecycle contract; requests cannot supply host paths or images.
package nodeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Instance contains no runtime credentials or host administration details.
type Instance struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Port     string `json:"port"`
	Endpoint string `json:"endpoint"`
	Healthy  bool   `json:"healthy"`
}

// Runtime reconciles a stable, service-owned instance ID.
type Runtime interface {
	Ensure(context.Context, string) (Instance, error)
}

var instanceID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Handler accepts only an empty configuration object and a stable instance UUID.
func Handler(runtime Runtime) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capacity", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Capacity(context.Context) (Capacity, error)
		})
		if !ok {
			http.Error(w, "capacity unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		capacity, err := provider.Capacity(ctx)
		if err != nil {
			http.Error(w, "capacity unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(capacity)
	})
	mux.HandleFunc("GET /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Status(context.Context, string) (Instance, error)
		})
		if !ok {
			http.Error(w, "instance inspection unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := provider.Status(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance inspection unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("PUT /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if !instanceID.MatchString(r.PathValue("id")) {
			http.Error(w, "invalid instance ID", 400)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request struct{}
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "unsupported instance configuration", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := runtime.Ensure(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance reconciliation unavailable", 503)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/v1/capacity" {
			mux.ServeHTTP(w, r)
			return
		}
		if (r.Method != "PUT" && r.Method != "GET") || !strings.HasPrefix(r.URL.Path, "/v1/instances/") || !instanceID.MatchString(strings.TrimPrefix(r.URL.Path, "/v1/instances/")) {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Authenticate requires a TLS-verified client certificate issued by the dedicated agent CA.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].Subject.CommonName != "pds-ass-worker" {
			http.Error(w, "verified worker identity required", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
