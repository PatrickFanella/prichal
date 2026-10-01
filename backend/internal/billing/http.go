package billing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Register commits authenticated fixture notifications before acknowledging them.
func (s *Store) Register(settings Settings) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/billing/webhook", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			reply := func(code int, body any) { w.WriteHeader(code); _ = json.NewEncoder(w).Encode(body) }
			if !settings.Enabled {
				reply(503, map[string]string{"error": "Billing delivery is not configured."})
				return
			}
			if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || len(r.Header.Values("Stripe-Signature")) != 1 {
				reply(400, map[string]string{"error": "Billing notification rejected."})
				return
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 262144))
			if err != nil {
				reply(400, map[string]string{"error": "Billing notification rejected."})
				return
			}
			event, supported, err := Verify(raw, r.Header.Get("Stripe-Signature"), settings.WebhookSecret)
			if err != nil {
				reply(400, map[string]string{"error": "Billing notification rejected."})
				return
			}
			if !supported {
				reply(200, map[string]bool{"ignored": true})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			duplicate, err := s.Receive(ctx, event)
			if err != nil {
				code := 503
				if err == ErrConflict {
					code = 400
				}
				reply(code, map[string]string{"error": "Billing notification could not be retained."})
				return
			}
			reply(200, map[string]bool{"accepted": true, "duplicate": duplicate})
		})
	}
}
