package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

// RegisterMembers exposes owner-only member invitation routes. Member passwords
// and email go directly to the native PDS.
func (s *Store) RegisterMembers(owners *auth.Service, local bool) func(*http.ServeMux) {
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
					respond(w, 503, map[string]string{"error": "Member invitations are not configured."})
					return
				}
				if !uuid.MatchString(r.PathValue("id")) {
					memberReply(w, nil, ErrNotFound)
					return
				}
				handler(w, r, session)
			})
		}
		empty := func(w http.ResponseWriter, r *http.Request) bool {
			var request struct{}
			r.Body = http.MaxBytesReader(w, r.Body, 128)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				memberReply(w, nil, ErrInvalid)
				return false
			}
			return true
		}
		bind("POST /api/deployments/{id}/members/invitations", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			var request MemberRequest
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
				memberReply(w, nil, ErrInvalid)
				return
			}
			invitation, err := s.SubmitMember(r.Context(), session.OwnerID, r.PathValue("id"), request)
			memberReply(w, invitation, err)
		})
		bind("GET /api/deployments/{id}/members/invitations", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			summary, err := s.ListMembers(r.Context(), session.OwnerID, r.PathValue("id"))
			memberReply(w, summary, err)
		})
		bind("GET /api/member-invitations/{id}", false, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			invitation, err := s.GetMember(r.Context(), session.OwnerID, r.PathValue("id"))
			memberReply(w, invitation, err)
		})
		bind("POST /api/member-invitations/{id}/refresh", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if empty(w, r) {
				invitation, err := s.RefreshMember(r.Context(), session.OwnerID, r.PathValue("id"))
				memberReply(w, invitation, err)
			}
		})
		bind("POST /api/member-invitations/{id}/withdraw", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if empty(w, r) {
				invitation, err := s.WithdrawMember(r.Context(), session.OwnerID, r.PathValue("id"))
				memberReply(w, invitation, err)
			}
		})
	}
}

func memberReply(w http.ResponseWriter, value any, err error) {
	if err == nil {
		respond(w, 200, value)
		return
	}
	status, message := 503, "The member invitation could not be confirmed. Retry with the same request key."
	switch {
	case errors.Is(err, ErrNotFound):
		status, message = 404, "Member invitation or deployment not found."
	case errors.Is(err, ErrInvalid):
		status, message = 400, "Use a valid member request key. Members enter passwords directly on the PDS."
	case errors.Is(err, recovery.ErrHeld):
		status, message = 503, "Management recovery is in progress. Member invitations are paused."
	case errors.Is(err, ErrAdmissionClosed):
		status, message = 503, "New invitation requests are paused. Existing invitations remain valid until expiry."
	case errors.Is(err, ErrOwnerAccountRequired):
		status, message = 409, "Create the owner's first account before inviting members."
	case errors.Is(err, ErrHostingExit):
		status, message = 409, "This server has a hosting exit in progress. New members cannot join."
	case errors.Is(err, ErrMemberCeiling):
		status, message = 409, "This server has reached its member limit."
	case errors.Is(err, ErrConflict):
		status, message = 409, "The member invitation is not open, was refreshed in the last five seconds, or the hourly limit was reached."
	}
	respond(w, status, map[string]string{"error": message})
}
