package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Account requests fixed moderation on the sole native account, never a caller-provided DID.
func (c *Client) Account(ctx context.Context, peer Peer, id string, intent ServiceRequest) (AccountResult, error) {
	if !instanceID.MatchString(id) || !intent.Valid() {
		return AccountResult{}, errors.New("invalid account request")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return AccountResult{}, err
	}
	defer client.CloseIdleConnections()
	body, err := json.Marshal(intent)
	if err != nil {
		return AccountResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", peer.Endpoint+"/v1/instances/"+id+"/account", bytes.NewReader(body))
	if err != nil {
		return AccountResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return AccountResult{}, errors.New("registered moderation node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return AccountResult{}, errors.New("registered node could not confirm account transition")
	}
	var result AccountResult
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.InstanceID != id || result.ServiceRequest != intent || !accountDID.MatchString(result.DID) {
		return AccountResult{}, errors.New("invalid node account response")
	}
	return result, nil
}
