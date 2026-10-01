package handleproof

import (
	"context"
	"encoding/json"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"io"
	"net/http"
	"regexp"
	"time"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (s *Store) Register(owners *auth.Service, local bool) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		bind := func(pattern string, mutation bool, handler func(context.Context, string, string, *http.Request) (Configuration, error)) {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "application/json")
				session, ok := owners.Authorize(w, r, mutation)
				if !ok {
					return
				}
				if !local {
					reply(w, Configuration{}, errors.New("live handle qualification is not configured"))
					return
				}
				id := r.PathValue("id")
				if !uuid.MatchString(id) {
					reply(w, Configuration{}, ErrNotFound)
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
				defer cancel()
				r.Body = http.MaxBytesReader(w, r.Body, 1024)
				value, err := handler(ctx, session.OwnerID, id, r)
				reply(w, value, err)
			})
		}
		bind("GET /api/deployments/{id}/handle", false, func(ctx context.Context, owner, id string, _ *http.Request) (Configuration, error) {
			return s.Get(ctx, owner, id)
		})
		bind("POST /api/deployments/{id}/handle", true, func(ctx context.Context, owner, id string, r *http.Request) (Configuration, error) {
			var body struct {
				Handle string `json:"handle"`
			}
			if !decode(r.Body, &body) {
				return Configuration{}, ErrInvalid
			}
			return s.Set(ctx, owner, id, body.Handle)
		})
		bind("POST /api/deployments/{id}/handle/check", true, func(ctx context.Context, owner, id string, r *http.Request) (Configuration, error) {
			var body struct{}
			if !decode(r.Body, &body) {
				return Configuration{}, ErrInvalid
			}
			return s.Check(ctx, owner, id)
		})
	}
}
func decode(body io.Reader, value any) bool {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value) == nil && decoder.Decode(new(any)) == io.EOF
}
func reply(w http.ResponseWriter, value Configuration, err error) {
	if err == nil {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(value)
		return
	}
	status, message := 503, "Domain verification is temporarily unavailable."
	switch {
	case errors.Is(err, ErrNotFound):
		status, message = 404, "Confirmed account or handle configuration not found."
	case errors.Is(err, ErrInvalid):
		status, message = 400, "Use a valid domain hostname without a URL, port or path."
	case errors.Is(err, ErrConflict):
		status, message = 409, "Wait at least 30 seconds before checking again."
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
