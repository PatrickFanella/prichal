package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CaddyImage is a pinned, service-managed local ingress release.
const CaddyImage = "caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d"
const roleLabel = "tv.subcult.pds-ass.role"

// Private bind mounts belong to the agent, so their readers use its numeric identity.
func privateFileUser() string { return fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()) }

func ingressEnvironment() []string {
	return []string{"XDG_CONFIG_HOME=/tmp/caddy-config", "XDG_DATA_HOME=/tmp/caddy-data"}
}

func validIngressEnvironment(environment []string) bool {
	for _, expected := range ingressEnvironment() {
		key := strings.SplitN(expected, "=", 2)[0] + "="
		count := 0
		for _, value := range environment {
			if strings.HasPrefix(value, key) {
				if value != expected {
					return false
				}
				count++
			}
		}
		if count != 1 {
			return false
		}
	}
	return true
}

// LocalEndpoint derives a stable loopback qualification hostname and port from the operation ID.
func LocalEndpoint(id string) string {
	if !instanceID.MatchString(id) {
		return ""
	}
	prefix, err := strconv.ParseUint(id[:4], 16, 16)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("https://%s.localhost:%d", id, 36000+prefix%10000)
}
func (d *Docker) ingress(ctx context.Context, id string) error {
	pdsName := "pds-ass-tenant-" + id
	name := "pds-ass-edge-" + id
	network := pdsName + "-network"
	var current inspected
	status, err := d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current)
	if err != nil && status != 404 {
		return err
	}
	configDirectory := filepath.Join(d.directory, id+"-edge")
	configPath := filepath.Join(configDirectory, "Caddyfile")
	tlsPath := filepath.Join(d.directory, id+"-tls.pem")
	if status != 404 {
		if _, err := os.Lstat(tlsPath); err != nil {
			return errors.New("existing ingress TLS identity is missing")
		}
	}
	if _, err = d.localCertificate(id); err != nil {
		return err
	}
	if status == 404 {
		if err = os.MkdirAll(configDirectory, 0700); err != nil {
			return err
		}
		info, err := os.Lstat(configDirectory)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return errors.New("unsafe ingress configuration directory")
		}
		configuration := d.ingressConfiguration(id)

		if existing, err := os.ReadFile(configPath); err == nil {
			info, err := os.Lstat(configPath)
			if err != nil || !info.Mode().IsRegular() || string(existing) != configuration {
				return errors.New("ingress configuration differs from policy")
			}
		} else if errors.Is(err, os.ErrNotExist) {
			file, err := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			if _, err = file.WriteString(configuration); err != nil {
				file.Close()
				return err
			}
			if err = file.Sync(); err != nil {
				file.Close()
				return err
			}
			if err = file.Close(); err != nil {
				return err
			}
		} else {
			return err
		}
		const ingressNetwork = "pds-ass-agent-ingress"
		var existing struct {
			Labels   map[string]string
			Internal bool
		}
		networkStatus, err := d.call(ctx, "GET", "/networks/"+ingressNetwork, nil, &existing)
		if err == nil && (existing.Internal || existing.Labels[roleLabel] != "ingress-network") {
			return errors.New("foreign ingress network conflict")
		}
		if err != nil && networkStatus != 404 {
			return err
		}
		if networkStatus == 404 {
			if _, err = d.call(ctx, "POST", "/networks/create", map[string]any{"Name": ingressNetwork, "Internal": false, "Labels": map[string]string{roleLabel: "ingress-network"}}, nil); err != nil {
				return err
			}
		}
		endpoint := LocalEndpoint(id)
		port := endpoint[len(endpoint)-5:]
		if _, err = strconv.Atoi(port); err != nil {
			return errors.New("invalid local endpoint port")
		}
		// The pinned executable has a NET_BIND_SERVICE file capability. Keep the
		// existing bounding allowance so exec succeeds with no-new-privileges.
		payload := map[string]any{"Image": CaddyImage, "User": privateFileUser(), "Env": ingressEnvironment(), "Labels": map[string]string{label: id, roleLabel: "ingress"}, "ExposedPorts": map[string]any{"8443/tcp": map[string]any{}}, "HostConfig": map[string]any{"Binds": []string{configPath + ":/etc/caddy/Caddyfile:ro", tlsPath + ":/etc/pds-tls.pem:ro"}, "NetworkMode": network, "Memory": int64(128 << 20), "NanoCpus": int64(250000000), "PidsLimit": 64, "CapDrop": []string{"ALL"}, "CapAdd": []string{"NET_BIND_SERVICE"}, "SecurityOpt": []string{"no-new-privileges:true"}, "PortBindings": map[string]any{"8443/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": port}}}, "RestartPolicy": map[string]string{"Name": "no"}}, "NetworkingConfig": map[string]any{"EndpointsConfig": map[string]any{network: map[string]any{}, ingressNetwork: map[string]any{}}}}
		status, err = d.call(ctx, "POST", "/containers/create?name="+name, payload, nil)
		if err != nil && status != 409 {
			return err
		}
		if _, err = d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
			return err
		}
	}
	if err := d.validateIngress(id, current); err != nil {
		return err
	}
	if !current.State.Running {
		if _, err = d.call(ctx, "POST", "/containers/"+name+"/start", nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func (d *Docker) localIngressHealthy(ctx context.Context, id string) bool {
	endpoint, err := url.Parse(LocalEndpoint(id))
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, "GET", "https://127.0.0.1:"+endpoint.Port()+"/xrpc/_health", nil)
	if err != nil {
		return false
	}
	request.Host = endpoint.Host
	if _, err := os.Lstat(filepath.Join(d.directory, id+"-tls.pem")); err != nil {
		return false
	}
	certificate, err := d.readLocalCertificate(id)
	if err != nil {
		return false
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		return false
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: id + ".localhost"}}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("ingress redirects rejected") }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == 200
}

func validIngressMounts(current inspected, configPath, tlsPath string) bool {
	if len(current.Mounts) != 2 {
		return false
	}
	expected := map[string]string{"/etc/caddy/Caddyfile": configPath, "/etc/pds-tls.pem": tlsPath}
	for _, mount := range current.Mounts {
		if source, ok := expected[mount.Destination]; !ok || source != mount.Source || mount.Type != "bind" || mount.RW {
			return false
		}
		delete(expected, mount.Destination)
	}
	return len(expected) == 0
}

func (d *Docker) validateIngress(id string, current inspected) error {
	network := "pds-ass-tenant-" + id + "-network"
	configPath := filepath.Join(d.directory, id+"-edge", "Caddyfile")
	tlsPath := filepath.Join(d.directory, id+"-tls.pem")
	configInfo, err := os.Lstat(configPath)
	if err != nil || !configInfo.Mode().IsRegular() || configInfo.Mode().Perm()&0077 != 0 {
		return errors.New("ingress policy file unavailable")
	}
	configuration, err := os.ReadFile(configPath)
	if err != nil || string(configuration) != d.ingressConfiguration(id) {
		return errors.New("ingress policy differs from configuration")
	}
	if len(current.NetworkSettings.Networks) != 2 || current.NetworkSettings.Networks[network] == nil || current.NetworkSettings.Networks["pds-ass-agent-ingress"] == nil {
		return errors.New("ingress networks differ from policy")
	}
	if current.Config.Image != CaddyImage || current.Config.User != privateFileUser() || !validIngressEnvironment(current.Config.Env) || current.Config.Labels[label] != id || current.Config.Labels[roleLabel] != "ingress" || current.HostConfig.Privileged || current.HostConfig.Memory != 128<<20 || current.HostConfig.NanoCpus != 250000000 || current.HostConfig.PidsLimit != 64 || current.HostConfig.NetworkMode != network || len(current.HostConfig.CapDrop) != 1 || current.HostConfig.CapDrop[0] != "ALL" || !slices.Equal(current.HostConfig.CapAdd, []string{"NET_BIND_SERVICE"}) || len(current.HostConfig.SecurityOpt) != 1 || current.HostConfig.SecurityOpt[0] != "no-new-privileges:true" || len(current.HostConfig.Binds) != 2 || !slices.Contains(current.HostConfig.Binds, configPath+":/etc/caddy/Caddyfile:ro") || !slices.Contains(current.HostConfig.Binds, tlsPath+":/etc/pds-tls.pem:ro") || !validIngressMounts(current, configPath, tlsPath) {
		return errors.New("ingress runtime differs from policy")
	}
	return nil
}

func (d *Docker) ingressConfiguration(id string) string {
	guard := ""
	if d.localIdentity {
		guard = fmt.Sprintf(`  @registration path_regexp (?i)^/(xrpc/com\.atproto\.server\.createAccount|@atproto/oauth-provider/~api/sign-up)/?$
  forward_auth @registration pds-ass-enrollment-%s:2727 {
   uri /registration
  }
`, id)
	}
	return fmt.Sprintf(`{
 auto_https off
 admin off
}
:8443 {
 tls /etc/pds-tls.pem /etc/pds-tls.pem
 @allowed host %s.localhost
 handle @allowed {
%s  reverse_proxy pds-ass-tenant-%s:2583
 }
 handle {
  respond "Unknown PDS host" 421
 }
}
`, id, guard, id)
}
