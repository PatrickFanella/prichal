package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// Capacity reports the Docker host and all retained service-owned instance namespaces.
type Capacity struct {
	MaximumSlots int      `json:"maximumSlots"`
	CPUs         int      `json:"cpus"`
	MemoryBytes  int64    `json:"memoryBytes"`
	InstanceIDs  []string `json:"instanceIds"`
}

// Capacity reserves host overhead and clamps the operator's local limit to measured CPU and memory.
func (d *Docker) Capacity(ctx context.Context) (Capacity, error) {
	var info struct {
		NCPU     int
		MemTotal int64
	}
	if _, err := d.call(ctx, "GET", "/info", nil, &info); err != nil {
		return Capacity{}, err
	}
	maximum := d.maximumSlots
	if d.pilot != nil {
		maximum = 1
	}
	if maximum == 0 {
		maximum = 2
	}
	memoryPerSlot := int64(896 << 20)
	cpuUnits, cpuCost := 4, 5
	if d.localIdentity {
		memoryPerSlot = 960 << 20
		cpuUnits, cpuCost = 8, 11
	}
	if memorySlots := int((info.MemTotal - (2 << 30)) / memoryPerSlot); memorySlots < maximum {
		maximum = memorySlots
	}
	if cpuSlots := (info.NCPU - 1) * cpuUnits / cpuCost; cpuSlots < maximum {
		maximum = cpuSlots
	}
	if maximum < 0 {
		maximum = 0
	}
	filters, _ := json.Marshal(map[string][]string{"label": {label}})
	var containers []struct{ Labels map[string]string }
	if _, err := d.call(ctx, "GET", "/containers/json?all=true&filters="+url.QueryEscape(string(filters)), nil, &containers); err != nil {
		return Capacity{}, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, container := range containers {
		if container.Labels[roleLabel] == "ingress" || container.Labels[roleLabel] == "enrollment-guard" {
			continue
		}
		if id := container.Labels[label]; instanceID.MatchString(id) {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		} else {
			return Capacity{}, errors.New("invalid managed namespace on host")
		}
	}
	return Capacity{MaximumSlots: maximum, CPUs: info.NCPU, MemoryBytes: info.MemTotal, InstanceIDs: ids}, nil
}

// SetCapacityCeiling applies an operator ceiling before serving requests. Measured resource limits still apply.
func (d *Docker) SetCapacityCeiling(maximum int) error {
	if maximum < 1 || maximum > 8 {
		return errors.New("local capacity ceiling must be between 1 and 8")
	}
	d.maximumSlots = maximum
	return nil
}
