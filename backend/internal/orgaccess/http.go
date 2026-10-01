package orgaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

type Sessions interface {
	Authorize(http.ResponseWriter, *http.Request, bool) (auth.Session, bool)
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, err error) {
	status, message := 503, "Organization collaboration is temporarily unavailable."
	if errors.Is(err, ErrNotFound) {
		status, message = 404, "Organization collaboration reference not found."
	}
	if errors.Is(err, ErrConflict) {
		status, message = 409, "Review current membership, invitation expiry and handover eligibility before retrying."
	}
	if errors.Is(err, ErrCursor) {
		status, message = 400, "Refresh current organization access before loading another page."
	}
	reply(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		reply(w, 400, map[string]string{"error": "Use the supported collaboration request fields."})
		return false
	}
	return true
}
func (s *Store) Register(sessions Sessions) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation bool, handler func(http.ResponseWriter, *http.Request, auth.Session)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if !s.Enabled {
					failure(w, ErrNotFound)
					return
				}
				if _, present := r.Header["Authorization"]; present {
					reply(w, 403, map[string]string{"error": "Collaboration requires a management browser session."})
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				session, ok := sessions.Authorize(w, r, mutation)
				if ok {
					handler(w, r, session)
				}
			})
		}
		bind("GET /api/team/organizations", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			v, err := s.Mine(r.Context(), a.OwnerID)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/organizations/page", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			after, err := organizationQuery(r.URL.RawQuery)
			if err != nil {
				failure(w, err)
				return
			}
			v, err := s.MinePage(r.Context(), a.OwnerID, after)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/incoming", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			v, err := s.Incoming(r.Context(), a.OwnerID)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/organizations/{id}", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			v, err := s.Shared(r.Context(), a.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/organizations/{id}/instances/page", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			after, err := listQuery(r.URL.RawQuery, "after", 384)
			if err != nil {
				failure(w, err)
				return
			}
			v, err := s.InstancesPage(r.Context(), a.OwnerID, r.PathValue("id"), after)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/organizations/{id}/members", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			v, err := s.Manage(r.Context(), a.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("GET /api/team/organizations/{id}/audit", false, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			before, err := auditQuery(r.URL.RawQuery)
			if err != nil {
				failure(w, err)
				return
			}
			v, err := s.Audit(r.Context(), a.OwnerID, r.PathValue("id"), before)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("POST /api/team/organizations/{id}/invitations", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct {
				Target string `json:"identityId"`
				Role   string `json:"role"`
				Key    string `json:"idempotencyKey"`
			}
			if !decode(w, r, &body) {
				return
			}
			v, err := s.Invite(r.Context(), a.OwnerID, r.PathValue("id"), body.Target, body.Role, body.Key)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("POST /api/team/invitations/{id}/accept", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct{}
			if !decode(w, r, &body) {
				return
			}
			v, err := s.Accept(r.Context(), a.OwnerID, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, v)
		})
		bind("PATCH /api/team/organizations/{id}/members/{target}", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct {
				Role string `json:"role"`
			}
			if !decode(w, r, &body) {
				return
			}
			if err := s.ChangeMember(r.Context(), a.OwnerID, r.PathValue("id"), r.PathValue("target"), body.Role); err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, map[string]bool{"recorded": true})
		})
		bind("PATCH /api/team/organizations/{id}", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct {
				Name string `json:"name"`
			}
			if !decode(w, r, &body) {
				return
			}
			if err := s.Rename(r.Context(), a.OwnerID, r.PathValue("id"), body.Name); err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, map[string]bool{"recorded": true})
		})
		bind("POST /api/team/organizations/{id}/transfers", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct {
				Target  string `json:"identityId"`
				Key     string `json:"idempotencyKey"`
				Confirm bool   `json:"confirmEmptyOrganizationHandover"`
			}
			if !decode(w, r, &body) {
				return
			}
			id, err := s.Transfer(r.Context(), a.OwnerID, r.PathValue("id"), body.Target, body.Key, body.Confirm)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, map[string]string{"id": id})
		})
		bind("POST /api/team/transfers/{id}/accept", true, func(w http.ResponseWriter, r *http.Request, a auth.Session) {
			var body struct {
				Confirm bool `json:"confirmEmptyOrganizationHandover"`
			}
			if !decode(w, r, &body) {
				return
			}
			id, err := s.AcceptTransfer(r.Context(), a.OwnerID, r.PathValue("id"), body.Confirm)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, map[string]string{"id": id})
		})
	}
}
