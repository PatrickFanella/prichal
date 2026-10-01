package fleet

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var nodeIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Handler belongs on the private HTTPS enrollment listener, independently of owner routes.
func (s Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/nodes/{id}/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !nodeIDPattern.MatchString(r.PathValue("id")) {
			http.NotFound(w, r)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "node enrollment credential rejected", 401)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var body struct {
			CSR string `json:"csr"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid enrollment request", 400)
			return
		}
		certificate, err := s.Enroll(r.Context(), r.PathValue("id"), token, []byte(body.CSR))
		if errors.Is(err, ErrUnauthorized) {
			http.Error(w, "node enrollment credential rejected", 401)
			return
		}
		if err != nil {
			http.Error(w, "node enrollment unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(certificate)
	})
	// Certificates are public. A lost enrollment response can be recovered without reusing the token.
	mux.HandleFunc("GET /v1/nodes/{id}/certificate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !nodeIDPattern.MatchString(r.PathValue("id")) {
			http.NotFound(w, r)
			return
		}
		var certificate []byte
		if err := s.Pool.QueryRow(r.Context(), `SELECT certificate_pem FROM hosting_nodes WHERE id=$1 AND certificate_pem IS NOT NULL`, r.PathValue("id")).Scan(&certificate); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(certificate)
	})
	return mux
}
