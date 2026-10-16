// Package nodeagent exposes a fixed lifecycle contract; requests cannot supply host paths or images.
package nodeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Instance contains no runtime credentials or host administration details.
type Instance struct {
	Release           *RuntimeRelease `json:"release,omitempty"`
	ID                string          `json:"id"`
	State             string          `json:"state"`
	Port              string          `json:"port"`
	Endpoint          string          `json:"endpoint"`
	Healthy           bool            `json:"healthy"`
	ServiceGeneration int64           `json:"serviceGeneration,omitempty"`
	ServiceTicket     string          `json:"serviceTicket,omitempty"`
	ServicePhase      string          `json:"servicePhase,omitempty"`
}

// Runtime reconciles a stable, service-owned instance ID.
type Runtime interface {
	Ensure(context.Context, string) (Instance, error)
}

var instanceID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Handler accepts only an empty configuration object and a stable instance UUID.
func Handler(runtime Runtime) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/instances/{id}/enrollment/withdraw", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		provider, ok := runtime.(interface {
			WithdrawEnrollment(context.Context, string, string) (Enrollment, error)
		})
		if !ok {
			http.Error(w, "enrollment withdrawal unavailable", 503)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128)
		var body struct {
			Ticket string `json:"ticket"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !instanceID.MatchString(body.Ticket) {
			http.Error(w, "enrollment ticket required", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := provider.WithdrawEnrollment(ctx, r.PathValue("id"), body.Ticket)
		if err != nil {
			http.Error(w, "enrollment withdrawal could not be confirmed", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	member := func(withdraw bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			provider, ok := runtime.(interface {
				MemberInvitation(context.Context, string, string) (MemberInvitation, error)
				WithdrawMemberInvitation(context.Context, string, string) (MemberInvitation, error)
			})
			if !ok {
				http.Error(w, "member invitations unavailable", 503)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 128)
			var body struct {
				Ticket string `json:"ticket"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !instanceID.MatchString(body.Ticket) {
				http.Error(w, "member ticket required", 400)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
			defer cancel()
			call := provider.MemberInvitation
			if withdraw {
				call = provider.WithdrawMemberInvitation
			}
			result, err := call(ctx, r.PathValue("id"), body.Ticket)
			if err != nil {
				http.Error(w, "member invitation could not be confirmed", 503)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		}
	}
	mux.HandleFunc("POST /v1/instances/{id}/members/invitation", member(false))
	mux.HandleFunc("POST /v1/instances/{id}/members/invitation/withdraw", member(true))
	mux.HandleFunc("POST /v1/instances/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Export(context.Context, string, string) (PortableExport, error)
		})
		if !ok {
			http.Error(w, "portable export unavailable", 503)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		var body struct {
			DID string `json:"did"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !accountDID.MatchString(body.DID) || !instanceID.MatchString(r.PathValue("id")) {
			http.Error(w, "invalid portable export request", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := provider.Export(ctx, r.PathValue("id"), body.DID)
		if err != nil || !result.ValidFor(body.DID) {
			http.Error(w, "portable export could not be verified", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /v1/instances/{id}/account", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			AccountStatus(context.Context, string) (AccountStatus, error)
		})
		if !ok {
			http.Error(w, "account inspection unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := provider.AccountStatus(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "account inspection unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /v1/instances/{id}/account", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Account(context.Context, string, ServiceRequest) (AccountResult, error)
		})
		if !ok {
			http.Error(w, "account moderation unavailable", 503)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		var body ServiceRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !body.Valid() {
			http.Error(w, "invalid account request", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := provider.Account(ctx, r.PathValue("id"), body)
		if err != nil {
			http.Error(w, "account transition could not be confirmed", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /v1/instances/{id}/service", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Service(context.Context, string, ServiceRequest) (ServiceResult, error)
		})
		if !ok {
			http.Error(w, "service lifecycle unavailable", 503)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		var body ServiceRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !body.Valid() {
			http.Error(w, "invalid service request", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := provider.Service(ctx, r.PathValue("id"), body)
		if err != nil {
			http.Error(w, "service transition could not be confirmed", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("GET /v1/capacity", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Capacity(context.Context) (Capacity, error)
		})
		if !ok {
			http.Error(w, "capacity unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		capacity, err := provider.Capacity(ctx)
		if err != nil {
			http.Error(w, "capacity unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(capacity)
	})
	mux.HandleFunc("GET /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Status(context.Context, string) (Instance, error)
		})
		if !ok {
			http.Error(w, "instance inspection unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := provider.Status(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance inspection unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("GET /v1/instances/{id}/usage", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Usage(context.Context, string) (Usage, error)
		})
		if !ok {
			http.Error(w, "usage unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := provider.Usage(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "usage unavailable", 503)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /v1/instances/{id}/enrollment", func(w http.ResponseWriter, r *http.Request) {
		provider, ok := runtime.(interface {
			Enroll(context.Context, string, string) (Enrollment, error)
		})
		if !ok {
			http.Error(w, "enrollment unavailable", 503)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128)
		var body struct {
			Ticket string `json:"ticket"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || !instanceID.MatchString(body.Ticket) {
			http.Error(w, "enrollment ticket required", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := provider.Enroll(ctx, r.PathValue("id"), body.Ticket)
		if err != nil {
			http.Error(w, "enrollment could not be confirmed", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("PUT /v1/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if !instanceID.MatchString(r.PathValue("id")) {
			http.Error(w, "invalid instance ID", 400)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request struct{}
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "unsupported instance configuration", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, err := runtime.Ensure(ctx, r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance reconciliation unavailable", 503)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.RawPath == "" && r.URL.RawQuery == "" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/export") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/export")) {
			mux.ServeHTTP(w, r)
			return
		}
		if (r.Method == "POST" || r.Method == "GET") && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/account") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/account")) {
			mux.ServeHTTP(w, r)
			return
		}

		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/service") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/service")) {
			mux.ServeHTTP(w, r)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/v1/capacity" {
			mux.ServeHTTP(w, r)
			return
		}
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/usage") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/usage")) {
			mux.ServeHTTP(w, r)
			return
		}
		for _, suffix := range []string{"/members/invitation", "/members/invitation/withdraw"} {
			if r.Method == "POST" && r.URL.RawPath == "" && r.URL.RawQuery == "" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, suffix) && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), suffix)) {
				mux.ServeHTTP(w, r)
				return
			}
		}
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/enrollment/withdraw") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/enrollment/withdraw")) {
			mux.ServeHTTP(w, r)
			return
		}
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/instances/") && strings.HasSuffix(r.URL.Path, "/enrollment") && instanceID.MatchString(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/instances/"), "/enrollment")) {
			mux.ServeHTTP(w, r)
			return
		}
		if (r.Method != "PUT" && r.Method != "GET") || !strings.HasPrefix(r.URL.Path, "/v1/instances/") || !instanceID.MatchString(strings.TrimPrefix(r.URL.Path, "/v1/instances/")) {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Authenticate requires a TLS-verified client certificate issued by the dedicated agent CA.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].Subject.CommonName != "pds-ass-worker" {
			http.Error(w, "verified worker identity required", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
