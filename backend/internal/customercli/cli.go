// Package customercli is a host-independent customer client. It never imports the host agent.
package customercli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const Version = "0.1.0-local"
const help = `pds-ass: manage your hosted PDS servers

Commands (JSON results on stdout; diagnostics on stderr):
  login --api-url URL [--no-browser]      Approve access in your browser
  organizations list                    List your organizations
  deploy --organization UUID --name NAME --idempotency-key UUID [--kind agent|test] [--wait]
  list --organization UUID              List servers in an organization
  status --deployment UUID [--wait]      Inspect or wait for a deployment
  logout                                Revoke this CLI credential
  completion bash|zsh|fish               Print shell completions

Each command accepts --config-dir DIR and --help.
--version prints the client version. No public API endpoint is configured.
Deployments currently accept only --region local. Test deployments are simulations.
Ctrl+C ends the client wait; server work continues. Retry deploy with the same key.
Exit codes: 0 success, 1 request/auth failure, 2 arguments, 3 pending, 4 failed, 130 interrupted.
`

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var serverName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$|^[a-z]$`)

type config struct {
	APIURL       string    `json:"apiUrl"`
	Token        string    `json:"token"`
	CredentialID string    `json:"credentialId"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
type operation struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	State          string `json:"state"`
	Endpoint       string `json:"endpoint"`
	Message        string `json:"message"`
}
type request struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Key    string `json:"idempotencyKey"`
}
type savedRequest struct {
	APIURL       string  `json:"apiUrl"`
	Organization string  `json:"organization"`
	Request      request `json:"request"`
}
type runner struct {
	out, err io.Writer
	input    io.Reader
	client   *http.Client
	config   config
	dir      string
}
type httpError int

func (e httpError) Error() string {
	switch e {
	case 401:
		return "Sign-in expired or was revoked. Run login again."
	case 403:
		return "Operation denied. Check CLI permissions and your hosting entitlement."
	case 404:
		return "Organization or deployment not found."
	case 409:
		return "Deployment is blocked or conflicts with existing work. Check allowance and its original request key."
	case 429:
		return "Too many requests. Wait before retrying."
	}
	return fmt.Sprintf("Management request failed (HTTP %d). Retry with the same deployment key.", e)
}
func ValidateAPI(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.RawPath != "" {
		return errors.New("Use an API origin without a path, credentials, query or fragment.")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		h := u.Hostname()
		ip := net.ParseIP(h)
		if h == "localhost" || (ip != nil && ip.IsLoopback()) {
			return nil
		}
	}
	return errors.New("Use verified HTTPS, or HTTP on a disposable loopback service.")
}
func (r *runner) call(ctx context.Context, method, path string, body, result any, anonymous bool) (int, error) {
	var data io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, errors.New("Invalid request payload.")
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.config.APIURL+path, data)
	if err != nil {
		return 0, errors.New("Invalid request URL.")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !anonymous {
		req.Header.Set("Authorization", "Bearer "+r.config.Token)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return 0, errors.New("Management request could not be confirmed. Retry with the same deployment key.")
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 1<<20)
	d := json.NewDecoder(limited)
	if res.StatusCode < 200 || res.StatusCode > 299 {
		if anonymous && res.StatusCode == 400 {
			if d.Decode(result) != nil {
				return res.StatusCode, errors.New("Invalid sign-in response.")
			}
			return res.StatusCode, nil
		}
		return res.StatusCode, httpError(res.StatusCode)
	}
	if result != nil {
		if d.Decode(result) != nil || d.Decode(new(any)) != io.EOF {
			return res.StatusCode, errors.New("Invalid management response.")
		}
	}
	return res.StatusCode, nil
}
func privateDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("Cannot create private CLI directory.")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("CLI directory must be a private directory (mode 0700).")
	}
	return nil
}
func readPrivate(path string, v any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return errors.New("No saved CLI sign-in. Run login first.")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return errors.New("CLI file must be private (mode 0600) and regular.")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return errors.New("Cannot read private CLI file.")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("Invalid private CLI file.")
	}
	return nil
}
func writeExclusive(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
func (r *runner) prompt(ctx context.Context, label string) (string, error) {
	f, ok := r.input.(*os.File)
	if !ok {
		return "", errors.New("Supply all required flags for noninteractive use.")
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return "", errors.New("Supply all required flags for noninteractive use.")
	}
	fmt.Fprint(r.err, label+": ")
	c := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(r.input)
		if scanner.Scan() {
			c <- strings.TrimSpace(scanner.Text())
		} else {
			c <- ""
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-c:
		return value, nil
	}
}
func wait(ctx context.Context, seconds int) error {
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (r *runner) login(ctx context.Context, noBrowser bool) error {
	if _, err := os.Lstat(filepath.Join(r.dir, "credentials.json")); !os.IsNotExist(err) {
		return errors.New("A sign-in file already exists. Run logout before signing in again.")
	}
	var grant struct {
		DeviceCode      string `json:"deviceCode"`
		UserCode        string `json:"userCode"`
		VerificationURI string `json:"verificationUri"`
		ExpiresIn       int    `json:"expiresIn"`
		Interval        int    `json:"interval"`
	}
	if _, err := r.call(ctx, "POST", "/api/cli/device", map[string]string{"clientId": "pds-ass-cli"}, &grant, true); err != nil {
		return err
	}
	if len(grant.DeviceCode) != 43 || !regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}$`).MatchString(grant.UserCode) || grant.VerificationURI != r.config.APIURL+"/device" || grant.Interval < 5 || grant.Interval > 600 || grant.ExpiresIn <= 0 || grant.ExpiresIn > 600 {
		return errors.New("Unsafe sign-in response.")
	}
	fmt.Fprintf(r.err, "Open %s and enter %s. Approve only this request.\n", grant.VerificationURI, grant.UserCode)
	if !noBrowser {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.CommandContext(ctx, "open", grant.VerificationURI)
		case "linux":
			command = exec.CommandContext(ctx, "xdg-open", grant.VerificationURI)
		case "windows":
			command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", grant.VerificationURI)
		}
		if command != nil {
			if err := command.Run(); err != nil {
				fmt.Fprintln(r.err, "Open the verification URL manually to continue.")
			}
		}
	}
	deadline := time.Now().Add(time.Duration(grant.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		if err := wait(ctx, grant.Interval); err != nil {
			return err
		}
		var result struct {
			Error        string    `json:"error"`
			Interval     int       `json:"interval"`
			AccessToken  string    `json:"accessToken"`
			CredentialID string    `json:"credentialId"`
			ExpiresAt    time.Time `json:"expiresAt"`
			Scope        string    `json:"scope"`
			TokenType    string    `json:"tokenType"`
		}
		status, err := r.call(ctx, "POST", "/api/cli/token", map[string]string{"clientId": "pds-ass-cli", "deviceCode": grant.DeviceCode}, &result, true)
		if err != nil {
			return err
		}
		if status == 400 {
			switch result.Error {
			case "authorization_pending":
				continue
			case "slow_down":
				if result.Interval < grant.Interval+5 || result.Interval > 600 {
					return errors.New("Invalid polling interval.")
				}
				grant.Interval = result.Interval
				continue
			case "access_denied":
				return errors.New("CLI access was denied.")
			case "expired_token":
				return errors.New("Sign-in expired. Run login again.")
			default:
				return errors.New("Sign-in was already used or invalid. Run login again.")
			}
		}
		if len(result.AccessToken) != 43 || !uuid.MatchString(result.CredentialID) || result.Scope != "deployments" || result.TokenType != "Bearer" || !result.ExpiresAt.After(time.Now()) || result.ExpiresAt.After(time.Now().Add(25*time.Hour)) {
			return errors.New("Invalid CLI credential response.")
		}
		r.config.Token = result.AccessToken
		r.config.CredentialID = result.CredentialID
		r.config.ExpiresAt = result.ExpiresAt
		if err := writeExclusive(filepath.Join(r.dir, "credentials.json"), r.config); err != nil { // Do not leave a newly minted credential active if storage fails.
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, revokeErr := r.call(cleanup, "POST", "/api/cli/logout", nil, &map[string]any{}, false)
			if revokeErr != nil {
				return errors.New("Credential storage failed and revocation could not be confirmed. Revoke CLI access in your browser.")
			}
			return errors.New("Cannot save private CLI credentials; new access was revoked.")
		}
		return json.NewEncoder(r.out).Encode(map[string]any{"signedIn": true, "credentialId": result.CredentialID, "expiresAt": result.ExpiresAt})
	}
	return errors.New("Sign-in expired. Run login again.")
}
func (r *runner) operation(ctx context.Context, op operation, waiting bool) (int, error) {
	for waiting && op.State != "ready" && op.State != "failed" {
		fmt.Fprintf(r.err, "Deployment %s: %s\n", op.ID, op.State)
		if err := wait(ctx, 2); err != nil {
			return 130, err
		}
		if _, err := r.call(ctx, "GET", "/api/deployments/"+op.ID, nil, &op, false); err != nil {
			return 1, err
		}
	}
	if err := json.NewEncoder(r.out).Encode(op); err != nil {
		return 1, err
	}
	switch op.State {
	case "ready":
		return 0, nil
	case "failed":
		return 4, nil
	default:
		return 3, nil
	}
}
func Run(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) int {
	code, err := run(ctx, args, input, out, diagnostics)
	if ctx.Err() != nil {
		fmt.Fprintln(diagnostics, "Interrupted. Server work continues; retry deploy with the same key or inspect status.")
		return 130
	}
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		if code == 0 {
			code = 1
		}
	}
	return code
}
func run(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, help)
		return 0, nil
	}
	if args[0] == "--version" {
		fmt.Fprintln(out, Version)
		return 0, nil
	}
	command := args[0]
	args = args[1:]
	if command == "completion" {
		if len(args) == 1 && args[0] == "--help" {
			fmt.Fprintln(out, "pds-ass completion bash|zsh|fish")
			return 0, nil
		}
		if len(args) != 1 {
			return 2, errors.New("Use completion bash, zsh or fish.")
		}
		return completion(out, args[0])
	}
	if command == "organizations" {
		if len(args) == 0 || args[0] != "list" {
			if len(args) > 0 && args[0] == "--help" {
				fmt.Fprintln(out, "pds-ass organizations list [--config-dir DIR]")
				return 0, nil
			}
			return 2, errors.New("Use organizations list.")
		}
		args = args[1:]
	}
	switch command {
	case "login", "logout", "organizations", "deploy", "list", "status":
	default:
		return 2, errors.New("Unknown command. Run pds-ass --help.")
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	var dir, api, organization, name, key, kind, region, id string
	var waiting, noBrowser bool
	fs.StringVar(&dir, "config-dir", "", "Private configuration directory (default: user config/pds-ass)")
	if command == "login" {
		fs.StringVar(&api, "api-url", "", "Management origin (HTTPS; loopback HTTP for local checks)")
		fs.BoolVar(&noBrowser, "no-browser", false, "Print the browser URL without opening it")
	}
	if command == "deploy" || command == "list" {
		fs.StringVar(&organization, "organization", "", "Owned organization UUID")
	}
	if command == "deploy" {
		fs.StringVar(&name, "name", "", "Lowercase server name")
		fs.StringVar(&key, "idempotency-key", "", "Stable request UUID; reuse it after interruption")
		fs.StringVar(&kind, "kind", "agent", "agent native server or test simulation")
		fs.StringVar(&region, "region", "local", "Provisioning region (local only)")
	}
	if command == "status" {
		fs.StringVar(&id, "deployment", "", "Deployment UUID")
	}
	if command == "deploy" || command == "status" {
		fs.BoolVar(&waiting, "wait", false, "Wait for ready or failed; Ctrl+C leaves server work running")
	}
	fs.Usage = func() {
		fmt.Fprintf(out, "pds-ass %s [flags]\n", command)
		old := fs.Output()
		fs.SetOutput(out)
		fs.PrintDefaults()
		fs.SetOutput(old)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, errors.New("Invalid arguments. Use --help.")
	}
	if len(fs.Args()) > 0 {
		return 2, errors.New("Unexpected positional arguments.")
	}
	if dir == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return 1, errors.New("Supply --config-dir.")
		}
		dir = filepath.Join(root, "pds-ass")
	}
	if err := privateDirectory(dir); err != nil {
		return 1, err
	}
	r := runner{out: out, err: diagnostics, input: input, dir: dir, client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if command == "login" {
		if api == "" {
			value, err := r.prompt(ctx, "Management API URL")
			if err != nil {
				return 2, err
			}
			api = value
		}
		if err := ValidateAPI(api); err != nil {
			return 2, err
		}
		r.config.APIURL = api
		if err := r.login(ctx, noBrowser); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if err := readPrivate(filepath.Join(dir, "credentials.json"), &r.config); err != nil {
		return 1, err
	}
	if err := ValidateAPI(r.config.APIURL); err != nil {
		return 1, err
	}
	if len(r.config.Token) != 43 || !uuid.MatchString(r.config.CredentialID) {
		return 1, errors.New("Invalid saved credential.")
	}
	if command == "logout" {
		_, err := r.call(ctx, "POST", "/api/cli/logout", nil, &map[string]any{}, false)
		if err != nil && !errors.Is(err, httpError(401)) {
			return 1, err
		}
		if err := os.Remove(filepath.Join(dir, "credentials.json")); err != nil {
			return 1, errors.New("Access revoked; remove the private credentials file manually.")
		}
		return 0, json.NewEncoder(out).Encode(map[string]bool{"signedOut": true})
	}
	if !r.config.ExpiresAt.After(time.Now()) {
		return 1, errors.New("Saved sign-in expired. Run logout, then login.")
	}
	if command == "organizations" {
		var organizations []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if _, err := r.call(ctx, "GET", "/api/organizations", nil, &organizations, false); err != nil {
			return 1, err
		}
		return 0, json.NewEncoder(out).Encode(organizations)
	}
	if command == "deploy" || command == "list" {
		if organization == "" {
			v, err := r.prompt(ctx, "Organization UUID")
			if err != nil {
				return 2, err
			}
			organization = v
		}
		if !uuid.MatchString(organization) {
			return 2, errors.New("Use a lowercase organization UUID.")
		}
	}
	if command == "list" {
		var ops []operation
		if _, err := r.call(ctx, "GET", "/api/organizations/"+organization+"/deployments", nil, &ops, false); err != nil {
			return 1, err
		}
		return 0, json.NewEncoder(out).Encode(ops)
	}
	if command == "status" {
		if !uuid.MatchString(id) {
			return 2, errors.New("Supply --deployment with a lowercase UUID.")
		}
		var op operation
		if _, err := r.call(ctx, "GET", "/api/deployments/"+id, nil, &op, false); err != nil {
			return 1, err
		}
		if op.ID != id {
			return 1, errors.New("Mismatched deployment response.")
		}
		return r.operation(ctx, op, waiting)
	}
	for _, value := range []struct {
		label  string
		target *string
	}{{"Server name", &name}, {"Stable request UUID", &key}} {
		if *value.target == "" {
			v, err := r.prompt(ctx, value.label)
			if err != nil {
				return 2, err
			}
			*value.target = v
		}
	}
	if !serverName.MatchString(name) || !uuid.MatchString(key) || (kind != "agent" && kind != "test") || region != "local" {
		return 2, errors.New("Use a lowercase server name, UUID request key, agent/test kind and local region.")
	}
	req := savedRequest{r.config.APIURL, organization, request{kind, name, region, key}}
	path := filepath.Join(dir, "request-"+key+".json")
	if err := writeExclusive(path, req); err != nil {
		if !os.IsExist(err) {
			return 1, errors.New("Cannot persist deployment request before submission.")
		}
		var prior savedRequest
		if err := readPrivate(path, &prior); err != nil {
			return 1, err
		}
		if prior != req {
			return 2, errors.New("This key is already saved with a different request. Use its original payload.")
		}
	}
	fmt.Fprintf(diagnostics, "Submitting request %s. Retry with this same key if interrupted.\n", key)
	var op operation
	if _, err := r.call(ctx, "POST", "/api/organizations/"+organization+"/deployments", req.Request, &op, false); err != nil {
		return 1, err
	}
	if !uuid.MatchString(op.ID) || op.OrganizationID != organization || op.Name != name || op.Kind != kind {
		return 1, errors.New("Mismatched deployment response. Retry with the same key.")
	}
	return r.operation(ctx, op, waiting)
}
func completion(out io.Writer, shell string) (int, error) {
	const words = "login organizations deploy list status logout completion --help --version --config-dir --api-url --no-browser --organization --name --idempotency-key --kind --region --deployment --wait"
	switch shell {
	case "bash":
		fmt.Fprintf(out, "complete -W '%s' pds-ass\n", words)
	case "zsh":
		fmt.Fprintf(out, "#compdef pds-ass\n_arguments '*:command or flag:(%s)'\n", words)
	case "fish":
		for _, word := range strings.Fields(words) {
			if !strings.HasPrefix(word, "--") {
				fmt.Fprintf(out, "complete -c pds-ass -f -a '%s'\n", word)
			} else {
				fmt.Fprintf(out, "complete -c pds-ass -l '%s'\n", word[2:])
			}
		}
	default:
		return 2, errors.New("Use completion bash, zsh or fish.")
	}
	return 0, nil
}
