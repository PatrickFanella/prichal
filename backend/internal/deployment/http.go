package deployment

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
				if ok {
					handler(w, r, session)
				}
			})
		}
		bind("POST /api/organizations/{id}/deployments", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if !local {
				reply(w, 503, map[string]string{"error": "Live provisioning is not configured."})
				return
			}
			var request Request
			r.Body = http.MaxBytesReader(w, r.Body, 2048)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				reply(w, 400, map[string]string{"error": "Enter a server name, region and request key."})
				return
			}
			if !uuidPattern.MatchString(r.PathValue("id")) {
				operationReply(w, Operation{}, ErrNotFound)
				return
			}
			op, err := s.Submit(r.Context(), session.OwnerID, r.PathValue("id"), request)
			operationReply(w, op, err)
		})
		bind("GET /api/deployments/{id}", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if !uuidPattern.MatchString(r.PathValue("id")) {
				operationReply(w, Operation{}, ErrNotFound)
				return
			}
			op, err := s.Get(r.Context(), session.OwnerID, r.PathValue("id"))
			operationReply(w, op, err)
		})
		bind("GET /api/organizations/{id}/deployments", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			id := r.PathValue("id")
			if !uuidPattern.MatchString(id) {
				operationReply(w, Operation{}, ErrNotFound)
				return
			}
			var found string
			if err := s.Pool.QueryRow(r.Context(), `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2`, id, session.OwnerID).Scan(&found); err != nil {
				operationReply(w, Operation{}, ErrNotFound)
				return
			}
			rows, err := s.Pool.Query(r.Context(), `SELECT id FROM deployments WHERE organization_id=$1 ORDER BY created_at,id`, id)
			if err != nil {
				operationReply(w, Operation{}, err)
				return
			}
			ids := []string{}
			for rows.Next() {
				var opID string
				if err = rows.Scan(&opID); err != nil {
					break
				}
				ids = append(ids, opID)
			}
			rows.Close()
			if err != nil || rows.Err() != nil {
				operationReply(w, Operation{}, errors.New("list unavailable"))
				return
			}
			ops := []Operation{}
			for _, opID := range ids {
				op, err := s.Get(r.Context(), session.OwnerID, opID)
				if err != nil {
					operationReply(w, Operation{}, err)
					return
				}
				ops = append(ops, op)
			}
			reply(w, 200, ops)
		})
	}
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func operationReply(w http.ResponseWriter, op Operation, err error) {
	if err == nil {
		reply(w, 200, op)
		return
	}
	status, message := 503, "Deployment status is temporarily unavailable. Retry with the same request key."
	switch {
	case errors.Is(err, ErrInvalid):
		status, message = 400, "Use a lowercase server name, the local region and a valid request key."
	case errors.Is(err, ErrNotFound):
		status, message = 404, "Deployment or organization not found."
	case errors.Is(err, ErrConflict):
		status, message = 409, "That request key or server name is already used. Refresh your deployments."
	}
	reply(w, status, map[string]string{"error": message})
}
