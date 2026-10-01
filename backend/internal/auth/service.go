// Package auth owns management identity and sessions, independently of PDS accounts.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const sessionLifetime = 24 * time.Hour

type Service struct {
	settings Settings
	pool     *pgxpool.Pool
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, pool *pgxpool.Pool, settings Settings) (*Service, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !settings.endpointAllowed(req.URL.String()) {
			return errors.New("OIDC redirect destination rejected")
		}
		return nil
	}}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, settings.Issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed; check the configured issuer")
	}
	var discovery struct {
		JWKS    string   `json:"jwks_uri"`
		Methods []string `json:"token_endpoint_auth_methods_supported"`
	}
	if err = provider.Claims(&discovery); err != nil {
		return nil, errors.New("invalid OIDC discovery")
	}
	endpoint := provider.Endpoint()
	method := "client_secret_basic"
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	if settings.ClientSecret == "" {
		method = "none"
		endpoint.AuthStyle = oauth2.AuthStyleInParams
	}
	supported := len(discovery.Methods) == 0 && method == "client_secret_basic"
	for _, value := range discovery.Methods {
		if value == method {
			supported = true
		}
	}
	if !supported {
		return nil, errors.New("OIDC provider does not support the configured client authentication method")
	}

	if !settings.endpointAllowed(endpoint.AuthURL) || !settings.endpointAllowed(endpoint.TokenURL) || !settings.endpointAllowed(discovery.JWKS) {
		return nil, errors.New("OIDC discovery contains an unsafe endpoint")
	}
	return &Service{settings: settings, pool: pool, oauth: oauth2.Config{ClientID: settings.ClientID, ClientSecret: settings.ClientSecret, Endpoint: endpoint, RedirectURL: settings.callback(), Scopes: []string{oidc.ScopeOpenID, "profile"}}, verifier: provider.Verifier(&oidc.Config{ClientID: settings.ClientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}})}, nil
}
func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("system random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func hash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func (s *Service) cookie(w http.ResponseWriter, suffix, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: s.settings.cookie(suffix), Value: value, Path: "/", HttpOnly: true, Secure: s.settings.Mode == "production", SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func jsonReply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func problem(w http.ResponseWriter, status int, message string) {
	jsonReply(w, status, map[string]string{"error": message})
}
func (s *Service) Register(mux *http.ServeMux) {
	bind := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			w.Header().Set("X-Content-Type-Options", "nosniff")
			handler(w, r.WithContext(ctx))
		})
	}
	bind("GET /api/auth/login", s.login)
	bind("GET /api/auth/callback", s.callback)
	bind("GET /api/session", s.session)
	bind("POST /api/auth/logout", s.logout)
	bind("POST /api/auth/revoke-all", s.revokeAll)
	bind("GET /api/organizations", s.organizations)
	bind("POST /api/organizations", s.createOrganization)
	bind("GET /api/organizations/{id}", s.organization)
	bind("PATCH /api/organizations/{id}", s.renameOrganization)
}
func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	state, binding, nonce, pkce := randomToken(), randomToken(), randomToken(), oauth2.GenerateVerifier()
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		problem(w, 503, "Sign-in is temporarily unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `DELETE FROM login_attempts WHERE expires_at <= now()`)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO login_attempts(state_hash,binding_hash,nonce,pkce_verifier,expires_at) VALUES($1,$2,$3,$4,now()+interval '10 minutes')`, hash(state), hash(binding), nonce, pkce)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		problem(w, 503, "Sign-in is temporarily unavailable.")
		return
	}
	s.cookie(w, "login", binding, 600)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// There is deliberately no caller-supplied return URL.
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(pkce)), http.StatusSeeOther)
}
func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	binding, err := r.Cookie(s.settings.cookie("login"))
	state := r.URL.Query().Get("state")
	if err != nil || len(binding.Value) != 43 || len(state) != 43 {
		callbackProblem(w, 400, "Sign-in expired or was already used. Start again.")
		return
	}
	var nonce, pkce string
	// Atomic consumption happens before token exchange, including failed callbacks.
	err = s.pool.QueryRow(r.Context(), `DELETE FROM login_attempts WHERE state_hash=$1 AND binding_hash=$2 AND expires_at>now() RETURNING nonce,pkce_verifier`, hash(state), hash(binding.Value)).Scan(&nonce, &pkce)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			callbackProblem(w, 503, "Sign-in is temporarily unavailable.")
		} else {
			callbackProblem(w, 400, "Sign-in expired or was already used. Start again.")
		}
		return
	}
	s.cookie(w, "login", "", -1)
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 2048 {
		callbackProblem(w, 400, "Sign-in was not completed. Start again.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !s.settings.endpointAllowed(req.URL.String()) {
			return errors.New("OIDC redirect rejected")
		}
		return nil
	}})
	token, err := s.oauth.Exchange(ctx, code, oauth2.VerifierOption(pkce))
	if err != nil {
		callbackProblem(w, 400, "The sign-in response was rejected. Start again.")
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		callbackProblem(w, 400, "The sign-in response was rejected. Start again.")
		return
	}
	identity, err := s.verifier.Verify(ctx, raw)
	if err != nil || identity.Subject == "" || len(identity.Subject) > 255 || subtle.ConstantTimeCompare([]byte(identity.Nonce), []byte(nonce)) != 1 {
		callbackProblem(w, 400, "The sign-in response was rejected. Start again.")
		return
	}
	var claims struct {
		Name string `json:"name"`
	}
	if err = identity.Claims(&claims); err != nil {
		callbackProblem(w, 400, "The sign-in response was rejected. Start again.")
		return
	}
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = "Creator"
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	session, csrf := randomToken(), randomToken()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		callbackProblem(w, 503, "Sign-in is temporarily unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var ownerID string
	err = tx.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES($1,$2,$3) ON CONFLICT(issuer,subject) DO UPDATE SET display_name=EXCLUDED.display_name RETURNING id`, s.settings.Issuer, identity.Subject, name).Scan(&ownerID)
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM management_sessions WHERE expires_at <= now()`)
	}
	// Replace the current browser's session without changing other devices' sessions.
	if previous, e := r.Cookie(s.settings.cookie("session")); e == nil && len(previous.Value) == 43 && err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM management_sessions WHERE token_hash=$1`, hash(previous.Value))
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,$3,now()+interval '24 hours')`, hash(session), ownerID, csrf)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		callbackProblem(w, 503, "Sign-in is temporarily unavailable.")
		return
	}
	s.cookie(w, "session", session, int(sessionLifetime.Seconds()))
	http.Redirect(w, r, s.settings.Origin+"/", http.StatusSeeOther)
}

type Session struct {
	OwnerID     string    `json:"ownerId"`
	DisplayName string    `json:"displayName"`
	CSRFToken   string    `json:"csrfToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func (s *Service) Authorize(w http.ResponseWriter, r *http.Request, mutation bool) (Session, bool) {
	var session Session
	cookie, err := r.Cookie(s.settings.cookie("session"))
	if err != nil || len(cookie.Value) != 43 {
		problem(w, 401, "Sign in to continue.")
		return session, false
	}
	err = s.pool.QueryRow(r.Context(), `SELECT o.id,o.display_name,m.csrf_token,m.expires_at FROM management_sessions m JOIN owners o ON o.id=m.owner_id WHERE m.token_hash=$1 AND m.expires_at>now()`, hash(cookie.Value)).Scan(&session.OwnerID, &session.DisplayName, &session.CSRFToken, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 401, "Sign in to continue.")
		} else {
			problem(w, 503, "Your workspace is temporarily unavailable.")
		}
		return session, false
	}
	if mutation && (r.Header.Get("Origin") != s.settings.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) != 1) {
		problem(w, 403, "Refresh this page and try again.")
		return session, false
	}
	return session, true
}
func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	if session, ok := s.Authorize(w, r, false); ok {
		jsonReply(w, 200, session)
	}
}
func (s *Service) logout(w http.ResponseWriter, r *http.Request)    { s.endSessions(w, r, false) }
func (s *Service) revokeAll(w http.ResponseWriter, r *http.Request) { s.endSessions(w, r, true) }
func (s *Service) endSessions(w http.ResponseWriter, r *http.Request, all bool) {
	session, ok := s.Authorize(w, r, true)
	if !ok {
		return
	}
	var err error
	if all {
		_, err = s.pool.Exec(r.Context(), `DELETE FROM management_sessions WHERE owner_id=$1`, session.OwnerID)
	} else {
		cookie, _ := r.Cookie(s.settings.cookie("session"))
		_, err = s.pool.Exec(r.Context(), `DELETE FROM management_sessions WHERE token_hash=$1`, hash(cookie.Value))
	}
	if err != nil {
		problem(w, 503, "Sign-out is temporarily unavailable.")
		return
	}
	s.cookie(w, "session", "", -1)
	jsonReply(w, 200, map[string]bool{"signedOut": true})
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) organizations(w http.ResponseWriter, r *http.Request) {
	session, ok := s.Authorize(w, r, false)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id,name FROM organizations WHERE owner_id=$1 ORDER BY created_at,id`, session.OwnerID)
	if err != nil {
		problem(w, 503, "Your organizations are temporarily unavailable.")
		return
	}
	defer rows.Close()
	organizations := []Organization{}
	for rows.Next() {
		var org Organization
		if err = rows.Scan(&org.ID, &org.Name); err != nil {
			break
		}
		organizations = append(organizations, org)
	}
	if err != nil || rows.Err() != nil {
		problem(w, 503, "Your organizations are temporarily unavailable.")
		return
	}
	jsonReply(w, 200, organizations)
}
func organizationName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, 400, "Enter an organization name.")
		return "", false
	}
	name := strings.TrimSpace(body.Name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		problem(w, 400, "Use an organization name between 1 and 100 characters.")
		return "", false
	}
	return name, true
}
func (s *Service) createOrganization(w http.ResponseWriter, r *http.Request) {
	session, ok := s.Authorize(w, r, true)
	if !ok {
		return
	}
	name, ok := organizationName(w, r)
	if !ok {
		return
	}
	var org Organization
	err := s.pool.QueryRow(r.Context(), `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),$1,$2) RETURNING id,name`, name, session.OwnerID).Scan(&org.ID, &org.Name)
	if err != nil {
		problem(w, 503, "Your organization could not be created. Try again.")
		return
	}
	jsonReply(w, 201, org)
}
func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func (s *Service) organization(w http.ResponseWriter, r *http.Request) {
	s.singleOrganization(w, r, false)
}
func (s *Service) renameOrganization(w http.ResponseWriter, r *http.Request) {
	s.singleOrganization(w, r, true)
}
func (s *Service) singleOrganization(w http.ResponseWriter, r *http.Request, rename bool) {
	session, ok := s.Authorize(w, r, rename)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "Organization not found.")
		return
	}
	var org Organization
	var err error
	if rename {
		name, ok := organizationName(w, r)
		if !ok {
			return
		}
		err = s.pool.QueryRow(r.Context(), `UPDATE organizations SET name=$1 WHERE id=$2 AND owner_id=$3 RETURNING id,name`, name, id, session.OwnerID).Scan(&org.ID, &org.Name)
	} else {
		err = s.pool.QueryRow(r.Context(), `SELECT id,name FROM organizations WHERE id=$1 AND owner_id=$2`, id, session.OwnerID).Scan(&org.ID, &org.Name)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Organization not found.")
		return
	}
	if err != nil {
		problem(w, 503, "Your organization is temporarily unavailable.")
		return
	}
	jsonReply(w, 200, org)
}

func callbackProblem(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign-in could not be completed</title></head><body><h1>Sign-in could not be completed</h1><p>`+html.EscapeString(message)+`</p><a href="/api/auth/login">Sign in again</a></body></html>`)
}
