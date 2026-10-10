package nodeagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// PDSImage is the service-managed release; callers cannot override it.
const PDSImage = "ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336"
const label = "tv.subcult.pds-ass.instance"

// Docker reconciles isolated local qualification runtimes through a Unix socket.
type Docker struct {
	client        *http.Client
	directory     string
	mutex         sync.Mutex
	healthProbe   func(context.Context, string) bool
	localIdentity bool
	maximumSlots  int
	memberCeiling int
	pilot         *PilotPolicy
}

// NewDocker accepts only an operator-configured local socket and private state directory.
func NewDocker(socket, directory string) (*Docker, error) {
	if !filepath.IsAbs(socket) || !filepath.IsAbs(directory) {
		return nil, errors.New("agent paths must be absolute")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("agent state must be a private directory")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	docker := &Docker{client: &http.Client{Transport: transport, Timeout: 20 * time.Second}, directory: directory}
	docker.healthProbe = docker.localIngressHealthy
	return docker, nil
}
func (d *Docker) call(ctx context.Context, method, path string, body, result any) (int, error) {
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			return 0, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://docker/v1.44"+path, &data)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(request)
	if err != nil {
		return 0, errors.New("Docker daemon unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return response.StatusCode, errors.New("Docker operation rejected")
	}
	if result != nil {
		if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}

type secrets struct {
	JWT      string `json:"jwt"`
	Admin    string `json:"admin"`
	Rotation string `json:"rotation"`
}

func (d *Docker) credentials(id string) (secrets, error) {
	path := filepath.Join(d.directory, id+".json")
	read := func() (secrets, error) { return d.readCredentials(id) }
	if value, err := read(); err == nil {
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return value, err
	}
	token := func() (string, error) {
		var value [32]byte
		_, err := rand.Read(value[:])
		return hex.EncodeToString(value[:]), err
	}
	var value secrets
	var err error
	if value.JWT, err = token(); err != nil {
		return value, err
	}
	if value.Admin, err = token(); err != nil {
		return value, err
	}
	order, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	key, err := rand.Int(rand.Reader, new(big.Int).Sub(order, big.NewInt(1)))
	if err != nil {
		return value, err
	}
	key.Add(key, big.NewInt(1))
	value.Rotation = fmt.Sprintf("%064x", key)
	data, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	temp, err := os.CreateTemp(d.directory, ".credential-")
	if err != nil {
		return value, err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return value, err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return value, err
	}
	if err = temp.Close(); err != nil {
		return value, err
	}
	// Hard-link creation is atomic and cannot replace credentials from another process.
	if err = os.Link(temp.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return value, err
	}
	directory, err := os.Open(d.directory)
	if err != nil {
		return value, err
	}
	err = directory.Sync()
	directory.Close()
	if err != nil {
		return value, err
	}
	return read()
}

type inspected struct {
	HostConfig struct {
		Memory         int64
		NanoCpus       int64
		PidsLimit      int64
		NetworkMode    string
		Privileged     bool
		ReadonlyRootfs bool
		CapDrop        []string
		CapAdd         []string
		SecurityOpt    []string
		Binds          []string
		PortBindings   map[string][]struct{ HostIP, HostPort string }
	}
	Mounts []struct {
		Type        string
		Source      string
		Name        string
		Destination string
		RW          bool
	}
	ID     string
	Config struct {
		User   string
		Image  string
		Labels map[string]string
		Env    []string
		Cmd    []string
	}
	State struct {
		Running bool
		Health  struct{ Status string }
	}
	NetworkSettings struct {
		Ports    map[string][]struct{ HostIP, HostPort string }
		Networks map[string]json.RawMessage
	}
}

// Ensure creates only fixed, labeled resources, then reconciles an existing container by ID.
func (d *Docker) Ensure(ctx context.Context, id string) (Instance, error) {
	if !d.allowedInstance(id) {
		return Instance{}, errors.New("instance is outside agent policy")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	held, holdErr := d.serviceHeld(id)
	if holdErr != nil {
		return Instance{}, holdErr
	}
	if held {
		return d.status(ctx, id)
	}
	name := "pds-ass-tenant-" + id
	var current inspected
	status, err := d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current)
	if err != nil && status != 404 {
		return Instance{}, err
	}
	if status == 404 {
		if err := d.fenceFreshPilot(ctx, id); err != nil {
			return Instance{}, err
		}
		capacity, capacityErr := d.Capacity(ctx)
		if capacityErr != nil {
			return Instance{}, capacityErr
		}
		if capacity.MaximumSlots < 1 || len(capacity.InstanceIDs) >= capacity.MaximumSlots && !slices.Contains(capacity.InstanceIDs, id) {
			return Instance{}, errors.New("hosting capacity exhausted")
		}
		if err := d.fenceRetainedNamespace(ctx, id); err != nil {
			return Instance{}, err
		}
		secret, err := d.credentials(id)
		if err != nil {
			return Instance{}, err
		}
		for _, suffix := range []string{"data", "blobs"} {
			volume := name + "-" + suffix
			var existing struct{ Labels map[string]string }
			status, err = d.call(ctx, "GET", "/volumes/"+volume, nil, &existing)
			if err == nil && existing.Labels[label] != id {
				return Instance{}, errors.New("foreign storage name conflict")
			}
			if err != nil && status != 404 {
				return Instance{}, err
			}
			if status == 404 {
				if _, err = d.call(ctx, "POST", "/volumes/create", map[string]any{"Name": volume, "Labels": map[string]string{label: id}}, nil); err != nil {
					return Instance{}, err
				}
			}
		}
		network := name + "-network"
		var existing struct {
			Labels   map[string]string
			Internal bool
		}
		status, err = d.call(ctx, "GET", "/networks/"+network, nil, &existing)
		if err == nil && (existing.Labels[label] != id || existing.Internal != (d.pilot == nil)) {
			return Instance{}, errors.New("foreign network name conflict")
		}
		if err != nil && status != 404 {
			return Instance{}, err
		}
		if status == 404 {
			if _, err = d.call(ctx, "POST", "/networks/create", map[string]any{"Name": network, "Internal": d.pilot == nil, "Labels": map[string]string{label: id}}, nil); err != nil {
				return Instance{}, err
			}
		}
		if d.localIdentity {
			if err := d.connectLocalIdentity(ctx, id); err != nil {
				return Instance{}, err
			}
			if err := d.enrollmentGuard(ctx, id); err != nil {
				return Instance{}, err
			}
		}
		payload := map[string]any{"Healthcheck": map[string]any{"Test": []string{"CMD", "node", "-e", "fetch('http://127.0.0.1:2583/xrpc/_health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"}, "Interval": int64(1e9), "Timeout": int64(2e9), "StartPeriod": int64(10e9), "Retries": 3}, "Image": PDSImage, "Labels": map[string]string{label: id}, "Env": d.runtimeEnvironment(id, secret), "ExposedPorts": map[string]any{"2583/tcp": map[string]any{}}, "HostConfig": map[string]any{"Mounts": []map[string]any{{"Type": "volume", "Source": name + "-data", "Target": "/pds"}, {"Type": "volume", "Source": name + "-blobs", "Target": "/blobs"}}, "NetworkMode": network, "Memory": int64(768 << 20), "NanoCpus": int64(1e9), "PidsLimit": 128, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "RestartPolicy": map[string]string{"Name": "no"}}}
		status, err = d.call(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), payload, nil)
		if err != nil && status != 409 {
			return Instance{}, err
		}
		if _, err = d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
			return Instance{}, err
		}
	}
	if err := d.validateRuntime(id, current); err != nil {
		return Instance{}, err
	}
	if err := d.validatePilotNetwork(ctx, id); err != nil {
		return Instance{}, err
	}
	if !current.State.Running {
		if _, err = d.call(ctx, "POST", "/containers/"+name+"/start", nil, nil); err != nil {
			return Instance{}, err
		}
	}
	if _, err = d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
		return Instance{}, err
	}
	if d.localIdentity {
		if err := d.connectLocalIdentity(ctx, id); err != nil {
			return Instance{}, err
		}
		if err := d.enrollmentGuard(ctx, id); err != nil {
			return Instance{}, err
		}
	}
	if err = d.ingress(ctx, id); err != nil {
		return Instance{}, err
	}
	result := Instance{ID: id, State: "starting", Endpoint: d.endpoint(id)}
	if current.State.Running {
		result.State = "running"
		result.Healthy = current.State.Health.Status == "healthy" && d.healthProbe != nil && d.healthProbe(ctx, id)
	}
	ports := current.NetworkSettings.Ports["2583/tcp"]
	if len(ports) == 1 && ports[0].HostIP == "127.0.0.1" {
		result.Port = ports[0].HostPort
	}
	return result, nil
}

func (d *Docker) runtimeEnvironment(id string, secret secrets) []string {
	environment := []string{"PDS_HOSTNAME=" + strings.TrimPrefix(LocalEndpoint(id), "https://"), "PDS_SERVICE_DID=did:web:" + id + ".localhost%3A" + strings.Split(LocalEndpoint(id), ":")[2], "PDS_PORT=2583", "PDS_DEV_MODE=true", "PDS_DATA_DIRECTORY=/pds", "PDS_BLOBSTORE_DISK_LOCATION=/blobs", "PDS_BLOBSTORE_DISK_TMP_LOCATION=/blobs/tmp", "PDS_INVITE_REQUIRED=true", "PDS_RATE_LIMITS_ENABLED=true", "PDS_BLOB_UPLOAD_LIMIT=5242880", "PDS_SERVICE_HANDLE_DOMAINS=.test", "PDS_DID_PLC_URL=http://127.0.0.1:9", "PDS_CRAWLERS=", "LOG_ENABLED=false", "PDS_JWT_SECRET=" + secret.JWT, "PDS_ADMIN_PASSWORD=" + secret.Admin, "PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX=" + secret.Rotation}
	if d.localIdentity {
		for i, entry := range environment {
			if strings.HasPrefix(entry, "PDS_DID_PLC_URL=") {
				environment[i] = "PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582"
			}
		}
		environment = append(environment, "PDS_EMAIL_FROM_ADDRESS=noreply@fixture.invalid", "PDS_EMAIL_SMTP_URL=smtp://pds-ass-protocol-fixtures:2525")
	}
	if d.pilot != nil {
		values := map[string]string{
			"PDS_HOSTNAME":               d.pilot.Hostname,
			"PDS_SERVICE_DID":            "did:web:" + d.pilot.Hostname,
			"PDS_DEV_MODE":               "false",
			"PDS_SERVICE_HANDLE_DOMAINS": "." + d.pilot.Hostname,
			"PDS_DID_PLC_URL":            "https://plc.directory",
			"PDS_CRAWLERS":               "https://bsky.network",
		}
		for i, entry := range environment {
			key, _, _ := strings.Cut(entry, "=")
			if value, ok := values[key]; ok {
				environment[i] = key + "=" + value
			}
		}
		environment = append(environment, "PDS_EMAIL_FROM_ADDRESS="+d.pilot.EmailFrom, "PDS_EMAIL_SMTP_URL="+d.pilot.SMTPURL)
		environment = append(environment,
			"PDS_BSKY_APP_VIEW_URL=https://api.bsky.app", "PDS_BSKY_APP_VIEW_DID=did:web:api.bsky.app",
			"PDS_REPORT_SERVICE_URL=https://mod.bsky.app", "PDS_REPORT_SERVICE_DID=did:plc:ar7c4by46qjdydhdevvrndac")
	}
	return environment
}

func (d *Docker) readCredentials(id string) (secrets, error) {
	path := filepath.Join(d.directory, id+".json")
	var value secrets
	info, err := os.Lstat(path)
	if err != nil {
		return value, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return value, errors.New("unsafe credential file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	if err == nil && (len(value.JWT) != 64 || len(value.Admin) != 64 || len(value.Rotation) != 64) {
		err = errors.New("invalid credential file")
	}
	return value, err
}

func (d *Docker) validateRuntime(id string, current inspected) error {
	name := "pds-ass-tenant-" + id
	if current.Config.Labels[label] != id || current.Config.Image != PDSImage {
		return errors.New("foreign runtime name conflict")
	}
	secret, err := d.readCredentials(id)
	if err != nil {
		return err
	}
	environment := map[string]string{}
	for _, entry := range current.Config.Env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			if _, duplicate := environment[key]; duplicate && strings.HasPrefix(key, "PDS_") {
				return errors.New("duplicate runtime environment setting")
			}
			environment[key] = value
		}
	}
	for _, entry := range d.runtimeEnvironment(id, secret) {
		key, value, _ := strings.Cut(entry, "=")
		if environment[key] != value {
			return errors.New("runtime environment differs from policy")
		}
	}
	for _, bindings := range current.NetworkSettings.Ports {
		if len(bindings) != 0 {
			return errors.New("PDS bypasses managed ingress")
		}
	}
	if current.HostConfig.Privileged || len(current.HostConfig.CapAdd) != 0 || len(current.HostConfig.Binds) != 0 || current.HostConfig.Memory != 768<<20 || current.HostConfig.NanoCpus != 1e9 || current.HostConfig.PidsLimit != 128 || current.HostConfig.NetworkMode != name+"-network" || len(current.HostConfig.CapDrop) != 1 || current.HostConfig.CapDrop[0] != "ALL" || len(current.HostConfig.SecurityOpt) != 1 || current.HostConfig.SecurityOpt[0] != "no-new-privileges:true" {
		return errors.New("runtime security configuration differs from policy")
	}
	if len(current.NetworkSettings.Networks) != 1 || current.NetworkSettings.Networks[name+"-network"] == nil {
		return errors.New("runtime network configuration differs from policy")
	}
	if len(current.Mounts) != 2 {
		return errors.New("runtime storage configuration differs from policy")
	}
	seen := map[string]bool{}
	for _, mount := range current.Mounts {
		suffix := ""
		switch mount.Destination {
		case "/pds":
			suffix = "data"
		case "/blobs":
			suffix = "blobs"
		default:
			return errors.New("unexpected runtime mount")
		}
		if seen[suffix] {
			return errors.New("duplicate runtime mount")
		}
		seen[suffix] = true
		if mount.Type != "volume" || mount.Name != name+"-"+suffix || !mount.RW {
			return errors.New("runtime storage configuration differs from policy")
		}
	}
	return nil
}
