package nodeagent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

//go:embed enrollment-guard.mjs
var enrollmentGuardScript string

// EnableLocalIdentityFixtures permits only the fixed, isolated qualification services.
// Production identity and SMTP configuration are deliberately unavailable.
func (d *Docker) EnableLocalIdentityFixtures() { d.localIdentity = true }

func (d *Docker) connectLocalIdentity(ctx context.Context, id string) error {
	var fixture inspected
	if _, err := d.call(ctx, "GET", "/containers/pds-ass-runtime-protocol-fixtures-1/json", nil, &fixture); err != nil {
		return errors.New("local protocol fixture unavailable")
	}
	if fixture.Config.Image != PDSImage || fixture.Config.Labels["com.docker.compose.project"] != "pds-ass-runtime" || fixture.Config.Labels["com.docker.compose.service"] != "protocol-fixtures" || !slices.Equal(fixture.Config.Cmd, []string{"node", "/fixtures/protocol-services.mjs"}) || !fixture.State.Running {
		return errors.New("local protocol fixture differs from policy")
	}
	network := "pds-ass-tenant-" + id + "-network"
	if value := fixture.NetworkSettings.Networks[network]; value != nil {
		var endpoint struct{ Aliases []string }
		if json.Unmarshal(value, &endpoint) != nil || !slices.Contains(endpoint.Aliases, "pds-ass-protocol-fixtures") {
			return errors.New("local protocol alias differs from policy")
		}
		return nil
	}
	_, err := d.call(ctx, "POST", "/networks/"+network+"/connect", map[string]any{"Container": fixture.ID, "EndpointConfig": map[string]any{"Aliases": []string{"pds-ass-protocol-fixtures"}}}, nil)
	return err
}

func (d *Docker) enrollmentGuard(ctx context.Context, id string) error {
	name := "pds-ass-enrollment-" + id
	directory := filepath.Join(d.directory, id+"-enrollment")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("private enrollment directory required")
	}
	var current inspected
	status, err := d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current)
	if err != nil && status != 404 {
		return err
	}
	if status == 404 {
		payload := map[string]any{"Image": PDSImage, "User": privateFileUser(), "Cmd": []string{"node", "--input-type=module", "-e", enrollmentGuardScript}, "Env": []string{"PDS_GUARD_INSTANCE=" + id}, "Labels": map[string]string{label: id, roleLabel: "enrollment-guard"}, "HostConfig": map[string]any{"Binds": []string{directory + ":/enrollment:ro"}, "NetworkMode": "pds-ass-tenant-" + id + "-network", "Memory": int64(64 << 20), "NanoCpus": int64(125000000), "PidsLimit": 32, "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "RestartPolicy": map[string]string{"Name": "no"}}}
		if _, err = d.call(ctx, "POST", "/containers/create?name="+name, payload, nil); err != nil {
			return err
		}
		if _, err = d.call(ctx, "GET", "/containers/"+name+"/json", nil, &current); err != nil {
			return err
		}
	}
	if err = d.validateEnrollmentGuard(id, current); err != nil {
		return err
	}
	if !current.State.Running {
		_, err = d.call(ctx, "POST", "/containers/"+name+"/start", nil, nil)
	}
	return err
}

func (d *Docker) validateEnrollmentGuard(id string, current inspected) error {
	network := "pds-ass-tenant-" + id + "-network"
	directory := filepath.Join(d.directory, id+"-enrollment")
	if current.Config.Image != PDSImage || current.Config.User != privateFileUser() || current.Config.Labels[label] != id || current.Config.Labels[roleLabel] != "enrollment-guard" || !slices.Equal(current.Config.Cmd, []string{"node", "--input-type=module", "-e", enrollmentGuardScript}) || !slices.Contains(current.Config.Env, "PDS_GUARD_INSTANCE="+id) || current.HostConfig.Privileged || !current.HostConfig.ReadonlyRootfs || current.HostConfig.Memory != 64<<20 || current.HostConfig.NanoCpus != 125000000 || current.HostConfig.PidsLimit != 32 || current.HostConfig.NetworkMode != network || !slices.Equal(current.HostConfig.CapDrop, []string{"ALL"}) || len(current.HostConfig.CapAdd) != 0 || !slices.Equal(current.HostConfig.SecurityOpt, []string{"no-new-privileges:true"}) || !slices.Equal(current.HostConfig.Binds, []string{directory + ":/enrollment:ro"}) {
		return errors.New("enrollment guard differs from policy")
	}
	if len(current.NetworkSettings.Networks) != 1 || current.NetworkSettings.Networks[network] == nil || len(current.Mounts) != 1 {
		return errors.New("enrollment guard isolation differs from policy")
	}
	mount := current.Mounts[0]
	if mount.Type != "bind" || mount.Source != directory || mount.Destination != "/enrollment" || mount.RW {
		return errors.New("enrollment guard storage differs from policy")
	}
	for _, bindings := range current.NetworkSettings.Ports {
		if len(bindings) != 0 {
			return errors.New("enrollment guard has public ports")
		}
	}
	return nil
}
