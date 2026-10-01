package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Service sends only a fixed lifecycle intent to the authenticated registered node.
func (c *Client) Service(ctx context.Context, peer Peer, id string, intent ServiceRequest) (ServiceResult, error) {
	if !instanceID.MatchString(id) || !intent.Valid() {
		return ServiceResult{}, errors.New("invalid service request")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return ServiceResult{}, err
	}
	defer client.CloseIdleConnections()
	body, err := json.Marshal(intent)
	if err != nil {
		return ServiceResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", peer.Endpoint+"/v1/instances/"+id+"/service", bytes.NewReader(body))
	if err != nil {
		return ServiceResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ServiceResult{}, errors.New("registered service node unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ServiceResult{}, errors.New("registered node could not confirm service transition")
	}
	var result ServiceResult
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.InstanceID != id || result.ServiceRequest != intent {
		return ServiceResult{}, errors.New("invalid node service response")
	}
	return result, nil
}

func validServiceMetadata(result Instance) bool {
	if result.ServiceGeneration == 0 {
		return result.ServiceTicket == "" && result.ServicePhase == "" && result.State != "suspended"
	}
	if result.ServiceGeneration < 1 || result.ServiceGeneration > 1<<53 || !instanceID.MatchString(result.ServiceTicket) || (result.ServicePhase != "pending" && result.ServicePhase != "applied") {
		return false
	}
	return result.State != "suspended" || !result.Healthy
}
