package endpointplan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

// Sessions is the management browser authorization boundary.
type Sessions interface {
	Authorize(http.ResponseWriter, *http.Request, bool) (auth.Session, bool)
}

// Register exposes local owner-only preflight. It persists no endpoint choice.
func (s *Store) Register(sessions Sessions) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/organizations/{id}/endpoint-preflight", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if _, present := r.Header["Authorization"]; present {
				reply(w, 403, map[string]string{"error": "Endpoint planning requires a management browser session."})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			session, ok := sessions.Authorize(w, r, true)
			if !ok {
				return
			}
			if !s.Local {
				reply(w, 503, map[string]string{"error": "Custom endpoint qualification is not configured."})
				return
			}
			var request Request
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				failure(w, ErrInvalid)
				return
			}
			result, err := s.Preflight(ctx, session.OwnerID, r.PathValue("id"), request)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, result)
		})
	}
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	status, message := 503, "Endpoint planning is temporarily unavailable."
	switch {
	case errors.Is(err, ErrNotFound):
		status, message = 404, "Organization reference not found."
	case errors.Is(err, ErrInvalid):
		status, message = 400, "Use a DNS hostname without a URL, port or IP address, and list up to eight application/OAuth hostnames."
	case errors.Is(err, ErrDomainSeparation):
		status, message = 400, "Use a separate registrable domain for PDS hosting and application/OAuth pages. Sibling subdomains are not sufficient."
	}
	reply(w, status, map[string]string{"error": message})
}
