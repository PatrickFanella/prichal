package hostingexit

import (
	"context"
	"encoding/json"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"io"
	"net/http"
	"time"
)

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	status, message := 503, "Hosting exit is temporarily unavailable. Your data is retained."
	if errors.Is(err, ErrNotFound) {
		status, message = 404, "Hosting exit or deployment not found."
	}
	if errors.Is(err, ErrConflict) {
		status, message = 409, "This exit requires review. Refresh its status or contact support. Your data is retained."
	}
	reply(w, status, map[string]string{"error": message})
}
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
		bind("GET /api/deployments/{id}/exit-policy", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if err := s.ownedDeployment(r.Context(), session.OwnerID, r.PathValue("id")); err != nil {
				failure(w, err)
				return
			}
			policy := map[string]any{"enabled": false, "automaticDeletion": false, "paymentCancellation": false}
			if s.Policy != nil {
				policy["enabled"] = true
				policy["delaySeconds"] = s.Policy.DelaySeconds
				policy["retentionDays"] = s.Policy.RetentionDays
			}
			reply(w, 200, policy)
		})
		bind("GET /api/deployments/{id}/exits", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			ops, err := s.List(r.Context(), session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, ops)
		})
		bind("POST /api/deployments/{id}/exits", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			r.Body = http.MaxBytesReader(w, r.Body, 512)
			var request Request
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !uuid.MatchString(request.Key) || !uuid.MatchString(request.ExportID) || !request.ConfirmTimeline || !request.ConfirmExportSaved {
				reply(w, 400, map[string]string{"error": "Select a verified export and confirm the exit timeline with a stable UUID request."})
				return
			}
			op, err := s.Schedule(r.Context(), session.OwnerID, r.PathValue("id"), request)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, op)
		})
		bind("GET /api/exits/{id}", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			op, err := s.Get(r.Context(), session.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, op)
		})
		bind("POST /api/exits/{id}/reverse", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			r.Body = http.MaxBytesReader(w, r.Body, 256)
			var request struct {
				Key string `json:"idempotencyKey"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !uuid.MatchString(request.Key) {
				reply(w, 400, map[string]string{"error": "Use a stable UUID reversal request."})
				return
			}
			op, err := s.Reverse(r.Context(), session.OwnerID, r.PathValue("id"), request.Key)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, op)
		})
	}
}
