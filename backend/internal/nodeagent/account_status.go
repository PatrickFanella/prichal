package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
)

// AccountStatus reports confirmed native moderation and its protected operation fence.
type AccountStatus struct {
	AccountResult
	Phase string `json:"phase,omitempty"`
}

// AccountStatus inspects only the retained native account and never changes it.
func (d *Docker) AccountStatus(ctx context.Context, id string) (AccountStatus, error) {
	if !d.localIdentity || !instanceID.MatchString(id) {
		return AccountStatus{}, errors.New("fixed local account required")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	service, err := d.status(ctx, id)
	if err != nil || service.State != "running" || !service.Healthy {
		return AccountStatus{}, errors.New("native account inspection unavailable")
	}
	lease, err := d.readAccountLease(id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AccountStatus{}, err
	}
	if lease.Phase == "pending" {
		return AccountStatus{}, errors.New("native account transition remains uncertain")
	}
	var native nativeAccount
	if err = d.executeFixed(ctx, "pds-ass-tenant-"+id, accountHelperScript, []string{"native_status"}, &native); err != nil {
		return AccountStatus{}, errors.New("native account inspection unavailable")
	}
	if !accountDID.MatchString(native.DID) || (native.State != "running" && native.State != "suspended") {
		return AccountStatus{}, errors.New("invalid native account status")
	}
	result := AccountStatus{AccountResult: lease.AccountResult, Phase: lease.Phase}
	if lease.Generation == 0 {
		if native.State != "running" || native.AppliedRef != "" {
			return result, errors.New("unowned native moderation requires inspection")
		}
		result.InstanceID = id
		result.State = "running"
		result.DID = native.DID
	} else if native.DID != lease.DID || native.State != lease.State || (native.State == "suspended" && native.AppliedRef != lease.AppliedRef) || (native.State == "running" && native.AppliedRef != "") {
		return result, errors.New("native moderation differs from retained intent")
	}
	result.Deactivated = native.Deactivated
	return result, nil
}

// AccountStatus authenticates the registered node before inspecting its retained native account.
func (c *Client) AccountStatus(ctx context.Context, peer Peer, id string) (AccountStatus, error) {
	if !instanceID.MatchString(id) {
		return AccountStatus{}, errors.New("invalid native account ID")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return AccountStatus{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "GET", peer.Endpoint+"/v1/instances/"+id+"/account", nil)
	if err != nil {
		return AccountStatus{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return AccountStatus{}, errors.New("registered moderation node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return AccountStatus{}, errors.New("registered moderation inspection unavailable")
	}
	var result AccountStatus
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.InstanceID != id || !accountDID.MatchString(result.DID) {
		return AccountStatus{}, errors.New("invalid native moderation response")
	}
	if result.Generation == 0 {
		if result.Ticket != "" || result.State != "running" || result.Phase != "" {
			return AccountStatus{}, errors.New("invalid unowned native moderation response")
		}
	} else if !result.ServiceRequest.Valid() || result.Phase != "applied" {
		return AccountStatus{}, errors.New("invalid native moderation fence")
	}
	return result, nil
}
