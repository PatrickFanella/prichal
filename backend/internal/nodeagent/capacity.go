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
	maximum := 2
	if memorySlots := int((info.MemTotal - (2 << 30)) / (896 << 20)); memorySlots < maximum {
		maximum = memorySlots
	}
	if cpuSlots := (info.NCPU - 1) * 4 / 5; cpuSlots < maximum {
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
		if container.Labels[roleLabel] == "ingress" {
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
