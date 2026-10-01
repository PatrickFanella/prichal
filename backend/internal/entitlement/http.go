package entitlement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

// Register exposes owner read-only status. There is no customer grant mutation endpoint.
func (s Store) Register(owners *auth.Service) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/organizations/{id}/entitlement", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			owner, ok := owners.Authorize(w, r, false)
			if !ok {
				return
			}
			result, err := s.Get(ctx, owner.OwnerID, r.PathValue("id"))
			if err != nil {
				status := 503
				message := "Hosting entitlement is temporarily unavailable."
				if errors.Is(err, ErrNotFound) {
					status = 404
					message = "Organization not found."
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
				return
			}
			_ = json.NewEncoder(w).Encode(result)
		})
	}
}
