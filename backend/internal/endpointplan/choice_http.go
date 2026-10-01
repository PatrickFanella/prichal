package endpointplan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

// RegisterChoices exposes local reviewed selection without DNS or host authority.
func (s *Store) RegisterChoices(sessions Sessions) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation bool, handle func(http.ResponseWriter, *http.Request, auth.Session)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if _, present := r.Header["Authorization"]; present {
					reply(w, 403, map[string]string{"error": "Endpoint choices require a management browser session."})
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				session, ok := sessions.Authorize(w, r, mutation)
				if !ok {
					return
				}
				if !s.Local {
					reply(w, 503, map[string]string{"error": "Custom endpoint qualification is not configured."})
					return
				}
				handle(w, r, session)
			})
		}
		bind("POST /api/organizations/{id}/endpoint-choices", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var request ChoiceRequest
			if !choiceBody(w, r, &request) {
				return
			}
			c, err := s.SaveChoice(r.Context(), a.OwnerID, r.PathValue("id"), request)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, c)
		})
		bind("GET /api/organizations/{id}/endpoint-choice", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			c, err := s.CurrentChoice(r.Context(), a.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, map[string]any{"choice": c})
		})
		bind("GET /api/organizations/{id}/endpoint-choices/{choice}", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			c, err := s.GetChoice(r.Context(), a.OwnerID, r.PathValue("id"), r.PathValue("choice"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, c)
		})
		bind("POST /api/organizations/{id}/endpoint-choices/{choice}/cancel", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var request struct {
				Key       string `json:"idempotencyKey"`
				Confirmed bool   `json:"confirmed"`
			}
			if !choiceBody(w, r, &request) {
				return
			}
			c, err := s.CancelChoice(r.Context(), a.OwnerID, r.PathValue("id"), r.PathValue("choice"), request.Key, request.Confirmed)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, c)
		})
	}
}
func choiceBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		failure(w, ErrInvalid)
		return false
	}
	return true
}
