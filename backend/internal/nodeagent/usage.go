package nodeagent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

//go:embed usage-helper.cjs
var usageHelperScript string

// Usage contains logical file bytes and shared-filesystem headroom, never record contents or credentials.
type Usage struct {
	InstanceID               string `json:"instanceId"`
	DataBytes                int64  `json:"dataBytes"`
	BlobBytes                int64  `json:"blobBytes"`
	DataFiles                int64  `json:"dataFiles"`
	BlobFiles                int64  `json:"blobFiles"`
	Accounts                 int64  `json:"accounts"`
	FilesystemBytes          int64  `json:"filesystemBytes"`
	FilesystemAvailableBytes int64  `json:"filesystemAvailableBytes"`
	ObservedAtMillis         int64  `json:"observedAtMillis"`
}

// Validate rejects mismatched, unbounded and stale measurements before persistence.
func (u Usage) Validate(id string) error {
	if !instanceID.MatchString(id) || u.InstanceID != id || u.DataBytes < 0 || u.BlobBytes < 0 || u.DataBytes > 1<<50 || u.BlobBytes > 1<<50 || u.DataFiles < 0 || u.BlobFiles < 0 || u.DataFiles > 100000 || u.BlobFiles > 100000 || u.DataFiles+u.BlobFiles > 100000 || u.Accounts < 0 || u.Accounts > 1000 || u.FilesystemBytes <= 0 || u.FilesystemBytes > 1<<53 || u.FilesystemAvailableBytes < 0 || u.FilesystemAvailableBytes > u.FilesystemBytes || u.ObservedAtMillis < time.Now().Add(-time.Minute).UnixMilli() || u.ObservedAtMillis > time.Now().Add(5*time.Second).UnixMilli() {
		return errors.New("invalid native usage measurement")
	}
	return nil
}

// Usage reads an already healthy policy-validated runtime without starting or creating resources.
func (d *Docker) Usage(ctx context.Context, id string) (Usage, error) {
	if !instanceID.MatchString(id) {
		return Usage{}, errors.New("invalid instance ID")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	status, err := d.status(ctx, id)
	if err != nil || status.State != "running" || !status.Healthy {
		return Usage{}, errors.New("healthy retained runtime required for usage")
	}
	var result Usage
	var args []string
	if d.managedRuntime(id) {
		identity, _ := d.readManagedIdentity(id)
		args = []string{"managed", identity.Hostname}
	}
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, usageHelperScript, args, &result); err != nil {
		return Usage{}, errors.New("native usage unavailable")
	}
	result.InstanceID = id
	return result, result.Validate(id)
}

// Usage authenticates and pins the registered node before fetching read-only counters.
func (c *Client) Usage(ctx context.Context, peer Peer, id string) (Usage, error) {
	if !instanceID.MatchString(id) {
		return Usage{}, errors.New("invalid instance ID")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return Usage{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "GET", peer.Endpoint+"/v1/instances/"+id+"/usage", nil)
	if err != nil {
		return Usage{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return Usage{}, errors.New("registered usage node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Usage{}, errors.New("registered usage unavailable")
	}
	var result Usage
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return Usage{}, errors.New("invalid node usage response")
	}
	return result, result.Validate(id)
}
