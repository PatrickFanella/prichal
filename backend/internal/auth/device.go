package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const cliClient = "pds-ass-cli"

var humanCode = regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}$`)
var uuidPath = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`
var cliOrganization = regexp.MustCompile(`^/api/organizations/` + uuidPath + `$`)
var cliDeployments = regexp.MustCompile(`^/api/organizations/` + uuidPath + `/deployments$`)
var cliDeployment = regexp.MustCompile(`^/api/deployments/` + uuidPath + `(/status)?$`)

func cliRoute(method, path string) bool {
	if method == "GET" {
		return path == "/api/organizations" || cliOrganization.MatchString(path) || cliDeployments.MatchString(path) || cliDeployment.MatchString(path)
	}
	return method == "POST" && (cliDeployments.MatchString(path) || path == "/api/cli/logout")
}
func (s *Service) authorizeCLI(w http.ResponseWriter, r *http.Request) (Session, bool) {
	var session Session
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || len(values[0]) != 50 || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.URL.RawPath != "" {
		problem(w, 401, "Use a CLI credential without browser cookies or Origin.")
		return session, false
	}
	if !cliRoute(r.Method, r.URL.Path) {
		problem(w, 403, "This CLI credential does not allow that operation.")
		return session, false
	}
	err := s.pool.QueryRow(r.Context(), `SELECT o.id,o.display_name,c.expires_at FROM cli_credentials c JOIN owners o ON o.id=c.owner_id WHERE c.token_hash=$1 AND c.expires_at>now() AND c.scope='deployments'`, hash(values[0][7:])).Scan(&session.OwnerID, &session.DisplayName, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 401, "CLI sign-in expired or was revoked. Sign in again.")
		} else {
			problem(w, 503, "CLI sign-in is temporarily unavailable.")
		}
		return session, false
	}
	return session, true
}
func deviceBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		problem(w, 400, "Send a JSON request.")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		problem(w, 400, "Invalid device request.")
		return false
	}
	return true
}
func anonymousDevice(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || len(r.Header.Values("Authorization")) > 0 {
		problem(w, 400, "Start or poll this request from the CLI.")
		return false
	}
	return true
}
func (s *Service) registerDevices(bind func(string, http.HandlerFunc)) {
	bind("POST /api/cli/device", s.startDevice)
	bind("POST /api/cli/token", s.pollDevice)
	bind("POST /api/cli/review", s.reviewDevice)
	bind("POST /api/cli/approve", s.approveDevice)
	bind("GET /api/cli/credentials", s.cliCredentials)
	bind("POST /api/cli/credentials/{id}/revoke", s.revokeCredential)
	bind("POST /api/cli/logout", s.cliLogout)
}
func (s *Service) startDevice(w http.ResponseWriter, r *http.Request) {
	if !anonymousDevice(w, r) {
		return
	}
	var request struct {
		ClientID string `json:"clientId"`
	}
	if !deviceBody(w, r, &request) {
		return
	}
	if request.ClientID != cliClient {
		problem(w, 400, "Unknown CLI client.")
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		problem(w, 400, "Invalid client address.")
		return
	}
	// Do not trust caller-supplied forwarding headers. Live proxy policy remains a qualification gate.
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		problem(w, 503, "CLI sign-in is unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(73647303); DELETE FROM cli_device_grants WHERE created_at<now()-interval '1 hour'`)
	var own, total int
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE requester_hash=$1),count(*) FROM cli_device_grants`, hash(ip)).Scan(&own, &total)
	}
	if err != nil {
		problem(w, 503, "CLI sign-in is unavailable.")
		return
	}
	if own >= 12 || total >= 1000 {
		w.Header().Set("Retry-After", "3600")
		problem(w, 429, "Too many sign-in requests. Try again later.")
		return
	}
	var entropy [5]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		problem(w, 503, "CLI sign-in is unavailable.")
		return
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(entropy[:])
	code = code[:4] + "-" + code[4:]
	secret := randomToken()
	_, err = tx.Exec(r.Context(), `INSERT INTO cli_device_grants(device_hash,user_hash,requester_hash) VALUES($1,$2,$3)`, hash(secret), hash(code), hash(ip))
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		problem(w, 503, "CLI sign-in is unavailable.")
		return
	}
	jsonReply(w, 200, map[string]any{"deviceCode": secret, "userCode": code, "verificationUri": s.settings.Origin + "/device", "expiresIn": 600, "interval": 5})
}
func (s *Service) pollDevice(w http.ResponseWriter, r *http.Request) {
	if !anonymousDevice(w, r) {
		return
	}
	var request struct {
		ClientID   string `json:"clientId"`
		DeviceCode string `json:"deviceCode"`
	}
	if !deviceBody(w, r, &request) {
		return
	}
	if request.ClientID != cliClient || len(request.DeviceCode) != 43 {
		problem(w, 400, "invalid_grant")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		problem(w, 503, "temporarily_unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var state string
	var owner *string
	var interval int
	var last *time.Time
	var now, expires time.Time
	err = tx.QueryRow(r.Context(), `SELECT state,owner_id::text,interval_seconds,last_poll_at,now(),expires_at FROM cli_device_grants WHERE device_hash=$1 FOR UPDATE`, hash(request.DeviceCode)).Scan(&state, &owner, &interval, &last, &now, &expires)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 400, "invalid_grant")
		} else {
			problem(w, 503, "temporarily_unavailable")
		}
		return
	}
	if !now.Before(expires) {
		problem(w, 400, "expired_token")
		return
	}
	if state == "denied" {
		problem(w, 400, "access_denied")
		return
	}
	if state == "consumed" {
		problem(w, 400, "invalid_grant")
		return
	}
	result := "authorization_pending"
	if last != nil && now.Sub(*last) < time.Duration(interval)*time.Second {
		interval += 5
		if interval > 600 {
			interval = 600
		}
		result = "slow_down"
	}
	_, err = tx.Exec(r.Context(), `UPDATE cli_device_grants SET interval_seconds=$2,last_poll_at=now() WHERE device_hash=$1`, hash(request.DeviceCode), interval)
	if result == "slow_down" || state == "pending" {
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			problem(w, 503, "temporarily_unavailable")
			return
		}
		jsonReply(w, 400, map[string]any{"error": result, "interval": interval})
		return
	}
	token := randomToken()
	var id string
	var credentialExpires time.Time
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO cli_credentials(token_hash,owner_id) VALUES($1,$2) RETURNING id,expires_at`, hash(token), owner).Scan(&id, &credentialExpires)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE cli_device_grants SET state='consumed' WHERE device_hash=$1`, hash(request.DeviceCode))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		problem(w, 503, "temporarily_unavailable")
		return
	}
	jsonReply(w, 200, map[string]any{"accessToken": token, "credentialId": id, "expiresAt": credentialExpires, "scope": "deployments", "tokenType": "Bearer"})
}
func (s *Service) deviceDecision(w http.ResponseWriter, r *http.Request, decision bool) {
	session, ok := s.Authorize(w, r, true)
	if !ok {
		return
	}
	var request struct {
		UserCode string `json:"userCode"`
		Approved *bool  `json:"approved,omitempty"`
	}
	if !deviceBody(w, r, &request) {
		return
	}
	request.UserCode = strings.ToUpper(strings.TrimSpace(request.UserCode))
	if !humanCode.MatchString(request.UserCode) || (decision && request.Approved == nil) || (!decision && request.Approved != nil) {
		problem(w, 400, "Enter the code shown by your CLI.")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		problem(w, 503, "CLI approval is unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT id FROM owners WHERE id=$1 FOR UPDATE`, session.OwnerID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM cli_approval_attempts WHERE attempted_at<now()-interval '1 minute'`)
	}
	var attempts int
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM cli_approval_attempts WHERE owner_id=$1`, session.OwnerID).Scan(&attempts)
	}
	if err != nil {
		problem(w, 503, "CLI approval is unavailable.")
		return
	}
	if attempts >= 10 {
		problem(w, 429, "Too many code attempts. Wait a minute.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO cli_approval_attempts(owner_id) VALUES($1)`, session.OwnerID)
	var count int
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM cli_device_grants WHERE user_hash=$1 AND state='pending' AND expires_at>now()`, hash(request.UserCode)).Scan(&count)
	}
	if err == nil && decision && count == 1 {
		state := "denied"
		if *request.Approved {
			state = "approved"
		}
		var tagRows int64
		tag, updateErr := tx.Exec(r.Context(), `UPDATE cli_device_grants SET state=$2,owner_id=$3 WHERE user_hash=$1 AND state='pending' AND expires_at>now()`, hash(request.UserCode), state, session.OwnerID)
		err = updateErr
		tagRows = tag.RowsAffected()
		if tagRows != 1 {
			count = 0
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		problem(w, 503, "CLI approval is unavailable.")
		return
	}
	if count != 1 {
		problem(w, 400, "Code expired or already used. Start again in your CLI.")
		return
	}
	jsonReply(w, 200, map[string]any{"userCode": request.UserCode, "permissions": "Read your organizations and servers; create deployments.", "expiresIn": 600, "recorded": decision})
}
func (s *Service) reviewDevice(w http.ResponseWriter, r *http.Request)  { s.deviceDecision(w, r, false) }
func (s *Service) approveDevice(w http.ResponseWriter, r *http.Request) { s.deviceDecision(w, r, true) }
func (s *Service) cliCredentials(w http.ResponseWriter, r *http.Request) {
	session, ok := s.Authorize(w, r, false)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id,created_at,expires_at FROM cli_credentials WHERE owner_id=$1 AND expires_at>now() ORDER BY created_at,id`, session.OwnerID)
	if err != nil {
		problem(w, 503, "CLI credentials are unavailable.")
		return
	}
	defer rows.Close()
	type credential struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	result := []credential{}
	for rows.Next() {
		var c credential
		if rows.Scan(&c.ID, &c.CreatedAt, &c.ExpiresAt) != nil {
			problem(w, 503, "CLI credentials are unavailable.")
			return
		}
		result = append(result, c)
	}
	if rows.Err() != nil {
		problem(w, 503, "CLI credentials are unavailable.")
		return
	}
	jsonReply(w, 200, result)
}
func (s *Service) revokeCredential(w http.ResponseWriter, r *http.Request) {
	session, ok := s.Authorize(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !regexp.MustCompile(`^` + uuidPath + `$`).MatchString(id) {
		problem(w, 404, "Credential not found.")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM cli_credentials WHERE id=$1 AND owner_id=$2`, id, session.OwnerID)
	if err != nil {
		problem(w, 503, "Revocation is unavailable.")
		return
	}
	if tag.RowsAffected() != 1 {
		problem(w, 404, "Credential not found.")
		return
	}
	jsonReply(w, 200, map[string]bool{"revoked": true})
}
func (s *Service) cliLogout(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		problem(w, 401, "Use your CLI credential.")
		return
	}
	if _, ok := s.Authorize(w, r, true); !ok {
		return
	}
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM cli_credentials WHERE token_hash=$1`, hash(r.Header.Get("Authorization")[7:])); err != nil {
		problem(w, 503, "Revocation is unavailable.")
		return
	}
	jsonReply(w, 200, map[string]bool{"revoked": true})
}
func (s *Service) revokeOwnerDevices(ctx context.Context, owner string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Delete grants before credentials so a concurrent consumption cannot recreate a revoked credential.
	for _, table := range []string{"cli_device_grants", "cli_credentials", "management_sessions"} {
		if _, err = tx.Exec(ctx, `DELETE FROM `+table+` WHERE owner_id=$1`, owner); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
