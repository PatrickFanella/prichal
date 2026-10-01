package exports

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	status, message := 503, "Portable export is temporarily unavailable. Existing data is retained."
	if errors.Is(err, ErrNotFound) {
		status, message = 404, "Export or deployment not found."
	}
	if errors.Is(err, ErrLimit) {
		status, message = 409, "The local retained-export allowance is full. Retry an existing request or contact support."
	}
	reply(w, status, map[string]string{"error": message})
}

// Register preserves owner session/CSRF authorization on all export access.
func (s *Store) Register(owners *auth.Service) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation bool, handler func(http.ResponseWriter, *http.Request, auth.Session)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				session, ok := owners.Authorize(w, r, mutation)
				if ok {
					handler(w, r, session)
				}
			})
		}
		bind("POST /api/deployments/{id}/exports", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			var body struct {
				Key string `json:"idempotencyKey"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 256)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !uuid.MatchString(body.Key) {
				reply(w, 400, map[string]string{"error": "Use a stable UUID export request key."})
				return
			}
			operation, err := s.Submit(r.Context(), session.OwnerID, r.PathValue("id"), body.Key)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, operation)
		})
		bind("GET /api/deployments/{id}/exports", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			operations, err := s.List(r.Context(), session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, operations)
		})
		bind("GET /api/exports/{id}", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			operation, err := s.Get(r.Context(), session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, operation)
		})
		bind("GET /api/exports/{id}/download", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			data, err := s.Download(r.Context(), session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/x-tar")
			w.Header().Set("Content-Disposition", `attachment; filename="pds-export-`+r.PathValue("id")+`.tar"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data)
		})
	}
}
