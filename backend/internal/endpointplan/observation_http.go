package endpointplan

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// RegisterObservation accepts an empty browser request for a bounded read-only TXT check.
// Production and CLI authority remain unavailable.
func (s *Store) RegisterObservation(sessions Sessions, dns TXTResolver) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/organizations/{id}/endpoint-choices/{choice}/dns-observation", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if _, present := r.Header["Authorization"]; present {
				reply(w, 403, map[string]string{"error": "Endpoint observations require a management browser session."})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			session, ok := sessions.Authorize(w, r, true)
			if !ok {
				return
			}
			if !s.Local {
				reply(w, 503, map[string]string{"error": "Custom endpoint qualification is not configured."})
				return
			}
			var body map[string]json.RawMessage
			if !choiceBody(w, r, &body) {
				return
			}
			if body == nil || len(body) != 0 {
				failure(w, ErrInvalid)
				return
			}
			result, err := s.Observe(ctx, session.OwnerID, r.PathValue("id"), r.PathValue("choice"), dns)
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, result)
		})
	}
}
