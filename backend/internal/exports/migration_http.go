package exports

import (
	"context"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

// MigrationSessions accepts only the management browser authorization boundary.
type MigrationSessions interface {
	Authorize(http.ResponseWriter, *http.Request, bool) (auth.Session, bool)
}

// RegisterMigration exposes bounded local inventory without migration execution.
func (s *Store) RegisterMigration(sessions MigrationSessions, local bool) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/exports/{id}/migration-preflight", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if _, present := r.Header["Authorization"]; present {
				reply(w, 403, map[string]string{"error": "Migration inventory requires a management browser session."})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			session, ok := sessions.Authorize(w, r, false)
			if !ok {
				return
			}
			if !local {
				failure(w, ErrUnavailable)
				return
			}
			result, err := s.PreflightMigration(ctx, session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, result)
		})
	}
}
