package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// Retained storage may be reused only after every preceding writer is stopped.
func (d *Docker) fenceRetainedNamespace(ctx context.Context, id string) error {
	filters, _ := json.Marshal(map[string][]string{"label": {label + "=" + id}})
	var containers []struct {
		Labels map[string]string
		State  string
	}
	if _, err := d.call(ctx, "GET", "/containers/json?all=true&filters="+url.QueryEscape(string(filters)), nil, &containers); err != nil {
		return err
	}
	for _, container := range containers {
		if container.Labels[label] != id {
			return errors.New("namespace inventory differs from policy")
		}
		if container.Labels[roleLabel] == "ingress" || container.Labels[roleLabel] == "enrollment-guard" {
			continue
		}
		if container.State != "exited" && container.State != "created" && container.State != "dead" {
			return errors.New("preceding namespace writer is not stopped")
		}
	}
	return nil
}
