package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

func (s *Store) Register(owners *auth.Service, local bool) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation bool, handler func(http.ResponseWriter, *http.Request, auth.Session)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				session, ok := owners.Authorize(w, r, mutation)
				if !ok {
					return
				}
				if !local || s.Cipher == nil {
					respond(w, 503, map[string]string{"error": "First-account enrollment is not configured."})
					return
				}
				if !uuid.MatchString(r.PathValue("id")) {
					operationReply(w, Operation{}, ErrNotFound)
					return
				}
				handler(w, r, session)
			})
		}
		bind("POST /api/deployments/{id}/enrollment", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			var request Request
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				operationReply(w, Operation{}, ErrInvalid)
				return
			}
			operation, err := s.Submit(r.Context(), session.OwnerID, r.PathValue("id"), request)
			operationReply(w, operation, err)
		})
		bind("GET /api/deployments/{id}/enrollment", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			operation, err := s.Latest(r.Context(), session.OwnerID, r.PathValue("id"))
			operationReply(w, operation, err)
		})
		bind("GET /api/enrollments/{id}", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			operation, err := s.Get(r.Context(), session.OwnerID, r.PathValue("id"))
			operationReply(w, operation, err)
		})
		bind("POST /api/enrollments/{id}/refresh", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			var request struct{}
			r.Body = http.MaxBytesReader(w, r.Body, 128)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				operationReply(w, Operation{}, ErrInvalid)
				return
			}
			operation, err := s.Refresh(r.Context(), session.OwnerID, r.PathValue("id"))
			operationReply(w, operation, err)
		})
	}
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func operationReply(w http.ResponseWriter, operation Operation, err error) {
	if err == nil {
		respond(w, 200, operation)
		return
	}
	status, message := 503, "Enrollment could not be confirmed. Retry with the same request key."
	switch {
	case errors.Is(err, ErrNotFound):
		status, message = 404, "Enrollment or deployment not found."
	case errors.Is(err, ErrInvalid):
		status, message = 400, "Use a valid enrollment request key. Passwords are entered directly on your PDS."
	case errors.Is(err, ErrConflict):
		status, message = 409, "Enrollment is active or its recovery limit was reached. Refresh its status or contact support."
	}
	respond(w, status, map[string]string{"error": message})
}
