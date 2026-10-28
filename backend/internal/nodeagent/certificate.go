package nodeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

// IngressCertificate contains public ingress identity only, never its private key.
type IngressCertificate struct {
	InstanceID  string `json:"instanceId"`
	Endpoint    string `json:"endpoint"`
	Certificate string `json:"certificate"`
	SHA256      string `json:"sha256"`
}

func (value IngressCertificate) Validate(id string) error {
	if value.InstanceID != id {
		return errors.New("ingress certificate instance differs")
	}
	publicHost, _ := managednamespace.Endpoint(id)
	if value.Endpoint != LocalEndpoint(id) && (publicHost == "" || value.Endpoint != "https://"+publicHost) {
		return errors.New("ingress certificate endpoint differs")
	}
	return managededge.ValidateCertificate(id, []byte(value.Certificate), value.SHA256, time.Now())
}

// IngressCertificate verifies retained resources before reading their public certificate. It never reconciles them.
func (d *Docker) IngressCertificate(ctx context.Context, id string) (IngressCertificate, error) {
	if !d.allowedInstance(id) {
		return IngressCertificate{}, errors.New("invalid ingress instance")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	status, err := d.status(ctx, id)
	if err != nil || status.State != "running" || !status.Healthy {
		return IngressCertificate{}, errors.New("healthy retained ingress required")
	}
	data, err := d.readLocalCertificate(id)
	if err != nil {
		return IngressCertificate{}, err
	}
	sum := sha256.Sum256(data)
	value := IngressCertificate{InstanceID: id, Endpoint: status.Endpoint, Certificate: string(data), SHA256: hex.EncodeToString(sum[:])}
	if err := value.Validate(id); err != nil {
		return IngressCertificate{}, err
	}
	return value, nil
}

// IngressCertificate authenticates the registered node and revalidates the public-only response.
func (c *Client) IngressCertificate(ctx context.Context, peer Peer, id string) (IngressCertificate, error) {
	if !instanceID.MatchString(id) {
		return IngressCertificate{}, errors.New("invalid ingress instance")
	}
	client, err := c.httpClient(peer)
	if err != nil {
		return IngressCertificate{}, err
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "GET", peer.Endpoint+"/v1/instances/"+id+"/certificate", nil)
	if err != nil {
		return IngressCertificate{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return IngressCertificate{}, errors.New("registered ingress certificate unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return IngressCertificate{}, errors.New("registered ingress certificate unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 20001))
	if err != nil || len(data) > 20000 {
		return IngressCertificate{}, errors.New("ingress certificate response exceeds scope")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result IngressCertificate
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Validate(id) != nil {
		return IngressCertificate{}, errors.New("invalid public ingress certificate response")
	}
	return result, nil
}
