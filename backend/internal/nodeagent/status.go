package nodeagent

import (
	"context"
	"errors"
)

// Status inspects retained resources without creating credentials or starting containers.
func (d *Docker) Status(ctx context.Context, id string) (Instance, error) {
	if !d.allowedInstance(id) {
		return Instance{}, errors.New("invalid instance ID")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.status(ctx, id)
}

func (d *Docker) status(ctx context.Context, id string) (Instance, error) {
	held, err := d.serviceHeld(id)
	if err != nil {
		return Instance{}, err
	}
	result, err := d.runtimeStatus(ctx, id)
	if err != nil {
		return result, err
	}
	if held {
		result.State = "suspended"
		result.Healthy = false
	}
	lease, leaseErr := d.readServiceLease(id)
	if leaseErr == nil {
		result.ServiceGeneration = lease.Generation
		result.ServiceTicket = lease.Ticket
		result.ServicePhase = lease.Phase
		if lease.State == "suspended" && lease.Phase == "applied" {
			if err := d.serviceStopped(ctx, id); err != nil {
				return Instance{}, err
			}
		}
	}
	return result, nil
}

func (d *Docker) runtimeStatus(ctx context.Context, id string) (Instance, error) {
	var current, edge inspected
	if _, err := d.call(ctx, "GET", "/containers/pds-ass-tenant-"+id+"/json", nil, &current); err != nil {
		return Instance{}, err
	}
	if err := d.validateRuntime(id, current); err != nil {
		return Instance{}, err
	}
	if err := d.validatePilotNetwork(ctx, id); err != nil {
		return Instance{}, err
	}
	if _, err := d.call(ctx, "GET", "/containers/pds-ass-edge-"+id+"/json", nil, &edge); err != nil {
		return Instance{}, err
	}
	if err := d.validateIngress(id, edge); err != nil {
		return Instance{}, err
	}
	if d.guardEnabled() {
		var guard inspected
		if _, err := d.call(ctx, "GET", "/containers/pds-ass-enrollment-"+id+"/json", nil, &guard); err != nil {
			return Instance{}, err
		}
		if err := d.validateEnrollmentGuard(id, guard); err != nil {
			return Instance{}, err
		}
		if !guard.State.Running {
			return Instance{ID: id, State: "starting", Endpoint: d.endpoint(id)}, nil
		}
	}
	result := Instance{ID: id, State: "starting", Endpoint: d.endpoint(id)}
	if current.State.Running && edge.State.Running {
		result.State = "running"
		result.Healthy = current.State.Health.Status == "healthy" && d.healthProbe != nil && d.healthProbe(ctx, id)
	}
	return result, nil
}
