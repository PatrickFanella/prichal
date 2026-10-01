package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ServiceRequest carries a management-assigned generation, never host configuration.
type ServiceRequest struct {
	Ticket     string `json:"ticket"`
	Generation int64  `json:"generation"`
	State      string `json:"state"`
}

// Valid requires a bounded generation and a fixed service action.
func (r ServiceRequest) Valid() bool {
	return instanceID.MatchString(r.Ticket) && r.Generation > 0 && r.Generation <= 1<<53 && (r.State == "running" || r.State == "suspended")
}

// ServiceResult reports only the confirmed public lifecycle state.
type ServiceResult struct {
	InstanceID string `json:"instanceId"`
	ServiceRequest
}

type serviceLease struct {
	ServiceResult
	Phase string `json:"phase"`
}

func (d *Docker) readServiceLease(id string) (serviceLease, error) {
	var result serviceLease
	path := filepath.Join(d.directory, id+"-service.json")
	info, err := os.Lstat(path)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1024 {
		return result, errors.New("unsafe service lease")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if json.Unmarshal(data, &result) != nil || result.InstanceID != id || !result.ServiceRequest.Valid() || (result.Phase != "pending" && result.Phase != "applied") {
		return result, errors.New("invalid service lease")
	}
	return result, nil
}

func (d *Docker) writeServiceLease(lease serviceLease) error {
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(d.directory, ".service-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
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
	if err = os.Rename(file.Name(), filepath.Join(d.directory, lease.InstanceID+"-service.json")); err != nil {
		return err
	}
	folder, err := os.Open(d.directory)
	if err != nil {
		return err
	}
	err = folder.Sync()
	folder.Close()
	return err
}

func (d *Docker) serviceHeld(id string) (bool, error) {
	lease, err := d.readServiceLease(id)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	return lease.State == "suspended" || lease.Phase == "pending", nil
}

// Service stops or resumes retained resources only. It never creates or replaces storage.
// The private lease fences stale work and prevents Ensure from undoing suspension.
func (d *Docker) Service(ctx context.Context, id string, request ServiceRequest) (ServiceResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result := ServiceResult{InstanceID: id, ServiceRequest: request}
	if !instanceID.MatchString(id) || !request.Valid() {
		return result, errors.New("invalid service request")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	previous, err := d.readServiceLease(id)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if previous.Generation > request.Generation || (previous.Generation == request.Generation && previous.ServiceRequest != request) {
		return result, errors.New("stale or conflicting service generation")
	}
	// Validate every retained resource before any mutation, including stopped ones.
	if _, err = d.runtimeStatus(ctx, id); err != nil {
		return result, err
	}
	lease := serviceLease{ServiceResult: result, Phase: "pending"}
	if err = d.writeServiceLease(lease); err != nil {
		return result, err
	}
	names := []string{"pds-ass-edge-" + id}
	if d.localIdentity {
		names = append(names, "pds-ass-enrollment-"+id)
	}
	names = append(names, "pds-ass-tenant-"+id)
	if request.State == "suspended" {
		for _, name := range names {
			if _, err = d.call(ctx, "POST", "/containers/"+name+"/stop?t=5", nil, nil); err != nil {
				return result, err
			}
		}
		if err = d.serviceStopped(ctx, id); err != nil {
			return result, err
		}
	} else {
		// Start the native process first. All identities and mounts were validated above.
		for i := len(names) - 1; i >= 0; i-- {
			if _, err = d.call(ctx, "POST", "/containers/"+names[i]+"/start", nil, nil); err != nil {
				return result, err
			}
		}
		for {
			current, checkErr := d.runtimeStatus(ctx, id)
			if checkErr != nil {
				return result, checkErr
			}
			if current.Healthy {
				break
			}
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	lease.Phase = "applied"
	return result, d.writeServiceLease(lease)
}

func (d *Docker) serviceStopped(ctx context.Context, id string) error {
	names := []string{"pds-ass-edge-" + id, "pds-ass-tenant-" + id}
	if d.localIdentity {
		names = append(names, "pds-ass-enrollment-"+id)
	}
	for _, name := range names {
		var current inspected
		if _, err := d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
			return err
		}
		if current.State.Running {
			return errors.New("service stop could not be confirmed")
		}
	}
	return nil
}
