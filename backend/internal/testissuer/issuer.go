// Package testissuer implements a loopback-only fixture, never a production identity provider.
package testissuer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type request struct {
	Redirect, State, Nonce, Challenge string
	Expires                           time.Time
}
type code struct {
	Request request
	Subject string
	Expires time.Time
}
type Issuer struct {
	BaseURL, ClientID, Callback, ClientSecret string
	key                                       *rsa.PrivateKey
	signer                                    jose.Signer
	mu                                        sync.Mutex
	requests                                  map[string]request
	codes                                     map[string]code
	fault                                     string
}

func New(baseURL, clientID, callback string) (*Issuer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "local-fixture", Algorithm: string(jose.RS256), Use: "sig"}}, nil)
	if err != nil {
		return nil, err
	}
	return &Issuer{BaseURL: baseURL, ClientID: clientID, Callback: callback, key: key, signer: signer, requests: map[string]request{}, codes: map[string]code{}}, nil
}

// SetFault exists for negative protocol tests. The command exposes no fault endpoint.
func (i *Issuer) SetFault(fault string) { i.mu.Lock(); defer i.mu.Unlock(); i.fault = fault }
func token() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func (i *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"issuer": i.BaseURL, "authorization_endpoint": i.BaseURL + "/authorize", "token_endpoint": i.BaseURL + "/token", "jwks_uri": i.BaseURL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic"}, "code_challenge_methods_supported": []string{"S256"}})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &i.key.PublicKey, KeyID: "local-fixture", Algorithm: string(jose.RS256), Use: "sig"}}})
	})
	mux.HandleFunc("GET /authorize", i.authorize)
	mux.HandleFunc("POST /approve", i.approve)
	mux.HandleFunc("POST /token", i.exchange)
	return mux
}

var page = template.Must(template.New("issuer").Parse(`<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Local sign-in fixture</title><style>body{font-family:system-ui;background:#f4f1e9;color:#242c28;max-width:480px;margin:10vh auto;padding:24px}h1{font-size:32px}button{padding:14px 20px;margin:10px 12px 10px 0;border:0;border-radius:6px;background:#244b3b;color:white;font-size:16px;cursor:pointer}p{line-height:1.6}</style></head><body><p>LOCAL TEST ISSUER</p><h1>Choose a test owner</h1><p>Alice and Bob are separate subjects with the same fixture email. This sign-in service is for local development only.</p><form method="post" action="/approve"><input type="hidden" name="request" value="{{.}}"><button name="owner" value="alice">Continue as Alice</button><button name="owner" value="bob">Continue as Bob</button></form></body></html>`))

func (i *Issuer) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != i.ClientID || q.Get("redirect_uri") != i.Callback || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || q.Get("state") == "" || q.Get("nonce") == "" || !strings.Contains(" "+q.Get("scope")+" ", " openid ") {
		http.Error(w, "Invalid fixture authorization request", 400)
		return
	}
	id := token()
	i.mu.Lock()
	for key, value := range i.requests {
		if time.Now().After(value.Expires) {
			delete(i.requests, key)
		}
	}
	i.requests[id] = request{Redirect: i.Callback, State: q.Get("state"), Nonce: q.Get("nonce"), Challenge: q.Get("code_challenge"), Expires: time.Now().Add(2 * time.Minute)}
	i.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, id)
}
func (i *Issuer) approve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != i.BaseURL {
		http.Error(w, "Fixture origin rejected", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if r.ParseForm() != nil {
		http.Error(w, "Invalid fixture request", 400)
		return
	}
	owner := r.PostForm.Get("owner")
	if owner != "alice" && owner != "bob" {
		http.Error(w, "Unknown test owner", 400)
		return
	}
	i.mu.Lock()
	request, ok := i.requests[r.PostForm.Get("request")]
	delete(i.requests, r.PostForm.Get("request"))
	if !ok || time.Now().After(request.Expires) {
		i.mu.Unlock()
		http.Error(w, "Fixture request expired", 400)
		return
	}
	value := token()
	i.codes[value] = code{Request: request, Subject: owner, Expires: time.Now().Add(time.Minute)}
	i.mu.Unlock()
	callback, _ := url.Parse(request.Redirect)
	q := callback.Query()
	q.Set("state", request.State)
	q.Set("code", value)
	callback.RawQuery = q.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, callback.String(), http.StatusSeeOther)
}
func (i *Issuer) exchange(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		reply(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	client := r.PostForm.Get("client_id")
	if i.ClientSecret == "" {
		if r.Header.Get("Authorization") != "" || r.PostForm.Get("client_secret") != "" {
			reply(w, 400, map[string]string{"error": "invalid_client"})
			return
		}
	} else {
		basic, secret, ok := r.BasicAuth()
		if !ok || basic != i.ClientID || subtle.ConstantTimeCompare([]byte(secret), []byte(i.ClientSecret)) != 1 {
			reply(w, 400, map[string]string{"error": "invalid_client"})
			return
		}
		client = basic
	}

	i.mu.Lock()
	entry, ok := i.codes[r.PostForm.Get("code")]
	delete(i.codes, r.PostForm.Get("code"))
	fault := i.fault
	i.mu.Unlock()
	digest := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if !ok || time.Now().After(entry.Expires) || client != i.ClientID || r.PostForm.Get("redirect_uri") != i.Callback || r.PostForm.Get("grant_type") != "authorization_code" || subtle.ConstantTimeCompare([]byte(challenge), []byte(entry.Request.Challenge)) != 1 {
		reply(w, 400, map[string]string{"error": "invalid_grant"})
		return
	}
	issuer, audience, nonce := i.BaseURL, i.ClientID, entry.Request.Nonce
	expiry := time.Now().Add(5 * time.Minute)
	switch fault {
	case "issuer":
		issuer = "https://wrong-issuer.invalid"
	case "audience":
		audience = "wrong-client"
	case "nonce":
		nonce = "wrong-nonce"
	case "expired":
		expiry = time.Now().Add(-time.Hour)
	}
	name := "Alice"
	if entry.Subject == "bob" {
		name = "Bob"
	}
	signer := i.signer
	if fault == "signature" {
		wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		signer, err = jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: wrongKey, KeyID: "local-fixture"}}, nil)
		if err != nil {
			panic(err)
		}
	}
	raw, err := jwt.Signed(signer).Claims(jwt.Claims{Issuer: issuer, Subject: entry.Subject, Audience: jwt.Audience{audience}, Expiry: jwt.NewNumericDate(expiry), IssuedAt: jwt.NewNumericDate(time.Now())}).Claims(map[string]any{"nonce": nonce, "name": name, "email": "same-email@fixture.test"}).Serialize()
	if err != nil {
		reply(w, 500, map[string]string{"error": "server_error"})
		return
	}
	reply(w, 200, map[string]any{"access_token": token(), "token_type": "Bearer", "expires_in": 300, "id_token": raw})
}
