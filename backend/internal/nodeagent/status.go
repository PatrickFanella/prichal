package nodeagent

import (
	"context"
	"errors"
)

// Status inspects retained resources without creating credentials or starting containers.
func (d *Docker) Status(ctx context.Context, id string) (Instance, error) {
	if !instanceID.MatchString(id) {
		return Instance{}, errors.New("invalid instance ID")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.status(ctx, id)
}

func (d *Docker) status(ctx context.Context, id string) (Instance, error) {
	var current, edge inspected
	if _, err := d.call(ctx, "GET", "/containers/pds-ass-tenant-"+id+"/json", nil, &current); err != nil {
		return Instance{}, err
	}
	if err := d.validateRuntime(id, current); err != nil {
		return Instance{}, err
	}
	if _, err := d.call(ctx, "GET", "/containers/pds-ass-edge-"+id+"/json", nil, &edge); err != nil {
		return Instance{}, err
	}
	if err := d.validateIngress(id, edge); err != nil {
		return Instance{}, err
	}
	if d.localIdentity {
		var guard inspected
		if _, err := d.call(ctx, "GET", "/containers/pds-ass-enrollment-"+id+"/json", nil, &guard); err != nil {
			return Instance{}, err
		}
		if err := d.validateEnrollmentGuard(id, guard); err != nil {
			return Instance{}, err
		}
		if !guard.State.Running {
			return Instance{ID: id, State: "starting", Endpoint: LocalEndpoint(id)}, nil
		}
	}
	result := Instance{ID: id, State: "starting", Endpoint: LocalEndpoint(id)}
	if current.State.Running && edge.State.Running {
		result.State = "running"
		result.Healthy = current.State.Health.Status == "healthy" && d.healthProbe != nil && d.healthProbe(ctx, id)
	}
	return result, nil
}
