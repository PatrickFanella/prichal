package support

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
)

// Sessions validates existing management cookie sessions and CSRF, separately from operator rights.
type Sessions interface {
	Authorize(http.ResponseWriter, *http.Request, bool) (auth.Session, bool)
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func unavailable(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		reply(w, 404, map[string]string{"error": "Support reference not found."})
	} else {
		reply(w, 503, map[string]string{"error": "Support evidence is temporarily unavailable."})
	}
}

// Register exposes fixed support actions without Docker, native credentials or impersonation.
func (s *Store) Register(sessions Sessions, actions *lifecycle.Store) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation, operator bool, handler func(http.ResponseWriter, *http.Request, auth.Session, string)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if operator && len(s.Settings.Identities) == 0 {
					reply(w, 404, map[string]string{"error": "Operator access is not configured."})
					return
				}
				if operator {
					if _, present := r.Header["Authorization"]; present {
						reply(w, 403, map[string]string{"error": "Operator access requires a management browser session."})
						return
					}
				}
				session, ok := sessions.Authorize(w, r, mutation)
				if !ok {
					return
				}
				role := ""
				if operator {
					var err error
					role, err = s.role(ctx, session.OwnerID)
					if err != nil {
						unavailable(w, err)
						return
					}
					if role == "" {
						reply(w, 404, map[string]string{"error": "Operator access unavailable."})
						return
					}
					if mutation && role != "operate" {
						reply(w, 403, map[string]string{"error": "This operator role permits inspection only."})
						return
					}
				}
				handler(w, r, session, role)
			})
		}
		bind("GET /api/operator/session", false, true, func(w http.ResponseWriter, r *http.Request, session auth.Session, role string) {
			reply(w, 200, map[string]string{"role": role})
		})
		bind("GET /api/operator/organizations/{id}", false, true, func(w http.ResponseWriter, r *http.Request, session auth.Session, role string) {
			view, err := s.Organization(r.Context(), session.OwnerID, role, r.PathValue("id"))
			if err != nil {
				unavailable(w, err)
				return
			}
			reply(w, 200, view)
		})
		bind("GET /api/operator/deployments/{id}", false, true, func(w http.ResponseWriter, r *http.Request, session auth.Session, role string) {
			view, err := s.Inspect(r.Context(), session.OwnerID, role, r.PathValue("id"))
			if err != nil {
				unavailable(w, err)
				return
			}
			reply(w, 200, view)
		})
		activity := func(operator bool) func(http.ResponseWriter, *http.Request, auth.Session, string) {
			return func(w http.ResponseWriter, r *http.Request, session auth.Session, role string) {
				before := int64(0)
				if raw := r.URL.Query().Get("before"); raw != "" {
					var err error
					before, err = strconv.ParseInt(raw, 10, 64)
					if err != nil || before <= 0 || before > 1<<53 {
						reply(w, 400, map[string]string{"error": "Use a valid history cursor."})
						return
					}
				}
				if operator {
					if _, err := s.Inspect(r.Context(), session.OwnerID, role, r.PathValue("id")); err != nil {
						unavailable(w, err)
						return
					}
				}
				view, err := s.Activity(r.Context(), session.OwnerID, r.PathValue("id"), operator, before)
				if err != nil {
					unavailable(w, err)
					return
				}
				reply(w, 200, view)
			}
		}
		bind("GET /api/operator/deployments/{id}/activity", false, true, activity(true))
		bind("GET /api/deployments/{id}/activity", false, false, activity(false))
		bind("POST /api/operator/deployments/{id}/lifecycle", true, true, func(w http.ResponseWriter, r *http.Request, session auth.Session, role string) {
			if !uuid.MatchString(r.PathValue("id")) {
				unavailable(w, ErrNotFound)
				return
			}
			var request struct {
				Key    string `json:"idempotencyKey"`
				Kind   string `json:"kind"`
				State  string `json:"state"`
				Reason string `json:"reason"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 2048)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !uuid.MatchString(request.Key) || (request.Kind != "service" && request.Kind != "account") || (request.State != "running" && request.State != "suspended") || (request.Reason != "maintenance" && request.Reason != "safety_review" && request.Reason != "resource_pressure") {
				reply(w, 400, map[string]string{"error": "Choose a fixed lifecycle action, reason and request key."})
				return
			}
			if actions == nil {
				unavailable(w, errors.New("actions unavailable"))
				return
			}
			operation, err := actions.Submit(r.Context(), "oidc-owner:"+session.OwnerID, r.PathValue("id"), lifecycle.Request{Key: request.Key, Kind: request.Kind, State: request.State, Reason: request.Reason})
			if err != nil {
				reply(w, 409, map[string]string{"error": "Lifecycle intent conflicts or cannot be confirmed. Inspect current history and retry the same key."})
				return
			}
			reply(w, 200, operation)
		})
	}
}
