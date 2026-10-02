package billing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

// CheckoutSessions authorizes management browser sessions and mutation CSRF.
type CheckoutSessions interface {
	Authorize(http.ResponseWriter, *http.Request, bool) (auth.Session, bool)
}

func checkoutReply(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func checkoutFailure(w http.ResponseWriter, err error) {
	status, message := 503, "Checkout is temporarily unavailable. Keep the original request and retry its receipt."
	switch {
	case errors.Is(err, ErrCheckoutNotFound):
		status, message = 404, "Checkout reference not found."
	case errors.Is(err, ErrInvalidEvent):
		status, message = 400, "Review the configured fixture plan and use its original request."
	case errors.Is(err, ErrConflict):
		status, message = 409, "Checkout requires review of the retained request and current configuration."
	}
	checkoutReply(w, status, map[string]string{"error": message})
}

// checkoutBody rejects duplicate, unknown, null and trailing fields before mutation.
func checkoutBody(w http.ResponseWriter, r *http.Request, prepare bool) (string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return "", false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return "", false
	}
	seen := map[string]bool{}
	key, confirmed := "", false
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return "", false
		}
		field, ok := token.(string)
		if !ok || seen[field] {
			return "", false
		}
		seen[field] = true
		var value any
		if err = d.Decode(&value); err != nil {
			return "", false
		}
		switch field {
		case "idempotencyKey":
			if !prepare {
				return "", false
			}
			key, ok = value.(string)
			if !ok || !uuid.MatchString(key) {
				return "", false
			}
		case "confirmed":
			confirmed, ok = value.(bool)
			if !ok || !confirmed {
				return "", false
			}
		default:
			return "", false
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF || !confirmed || prepare && key == "" {
		return "", false
	}
	return key, true
}

// RegisterCheckout exposes explicitly enabled local owner preparation/dispatch and
// exact receipt/session recovery. No bearer, host, entitlement or Portal authority is added.
func (s *CheckoutIntents) RegisterCheckout(sessions CheckoutSessions, adapter *FixtureSessions) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		register := func(pattern, action string, mutation bool) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Referrer-Policy", "no-referrer")
				if _, present := r.Header["Authorization"]; present {
					checkoutReply(w, 403, map[string]string{"error": "Checkout requires a management browser session."})
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				if sessions == nil {
					checkoutFailure(w, ErrUnavailable)
					return
				}
				owner, ok := sessions.Authorize(w, r, mutation)
				if !ok {
					return
				}
				if s == nil || adapter == nil {
					checkoutFailure(w, ErrUnavailable)
					return
				}
				var key string
				if action == "byKey" {
					values, err := url.ParseQuery(r.URL.RawQuery)
					if err != nil || len(values) != 1 || len(values["key"]) != 1 || !uuid.MatchString(values.Get("key")) {
						checkoutFailure(w, ErrInvalidEvent)
						return
					}
					key = values.Get("key")
				} else if r.URL.RawQuery != "" || r.URL.ForceQuery {
					checkoutFailure(w, ErrInvalidEvent)
					return
				}
				if !mutation && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
					checkoutFailure(w, ErrInvalidEvent)
					return
				}
				if mutation {
					var valid bool
					key, valid = checkoutBody(w, r, action == "prepare")
					if !valid {
						checkoutFailure(w, ErrInvalidEvent)
						return
					}
				}
				org, id := r.PathValue("id"), r.PathValue("checkout")
				var result any
				var err error
				switch action {
				case "plan":
					result, err = s.Plan(ctx, owner.OwnerID, org)
				case "prepare":
					result, err = s.Prepare(ctx, owner.OwnerID, org, key, true)
				case "start":
					result, err = s.Start(ctx, owner.OwnerID, org, id, true)
				case "byKey":
					result, err = s.GetByKey(ctx, owner.OwnerID, org, key)
				case "get":
					result, err = s.Get(ctx, owner.OwnerID, org, id)
				case "session":
					result, err = s.RecoverSession(ctx, owner.OwnerID, org, id, adapter)
				}
				if err != nil {
					checkoutFailure(w, err)
					return
				}
				checkoutReply(w, 200, result)
			})
		}
		register("GET /api/organizations/{id}/billing/checkout-plan", "plan", false)
		register("POST /api/organizations/{id}/billing/checkout", "prepare", true)
		register("GET /api/organizations/{id}/billing/checkout", "byKey", false)
		register("GET /api/organizations/{id}/billing/checkout/{checkout}", "get", false)
		register("POST /api/organizations/{id}/billing/checkout/{checkout}/start", "start", true)
		register("GET /api/organizations/{id}/billing/checkout/{checkout}/session", "session", false)
	}
}
