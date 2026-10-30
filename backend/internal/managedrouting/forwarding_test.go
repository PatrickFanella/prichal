package managedrouting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

func forwardingFixture(t *testing.T) (storeFixture, Bundle, Observation) {
	t.Helper()
	f := routingFixture(t)
	ctx := context.Background()
	if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
		t.Fatal(err)
	}
	f.place(t)
	observation, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := f.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return f, bundle, observation
}

func TestExportBindsPrivateForwardingHandoffToExactInventory(t *testing.T) {
	f, bundle, observation := forwardingFixture(t)
	directory := filepath.Join(t.TempDir(), "bundle")
	if err := WriteBundle(directory, bundle); err != nil {
		t.Fatal(err)
	}
	inventory, err := os.ReadFile(filepath.Join(directory, "inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "forwarding.json"))
	if err != nil {
		t.Fatal(err)
	}
	var handoff forwardingManifest
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(inventory)
	if handoff.Version != 1 || handoff.InventorySHA256 != hex.EncodeToString(sum[:]) || handoff.Generation != bundle.Inventory.Generation || !handoff.ExpiresAt.Equal(*bundle.Inventory.ExpiresAt) || len(handoff.Endpoints) != 1 {
		t.Fatal("handoff does not bind the exact inventory bytes and deadline")
	}
	forward := handoff.Endpoints[0]
	endpoint := bundle.Inventory.Endpoints[0]
	if forward.DeploymentID != f.id || forward.NodeID != f.node || forward.NodeKeySHA256 != strings.Repeat("00", 32) || forward.NativeEndpoint != nodeagent.LocalEndpoint(f.id) || forward.ObservationSequence != observation.Sequence || !forward.ObservedAt.Equal(observation.ObservedAt) || forward.Hostname != endpoint.Hostname || forward.ForwardPort != endpoint.ForwardPort || forward.CertificateSHA256 != endpoint.CertificateSHA256 {
		t.Fatal("handoff changed current placement, readback, or forward identity")
	}
	for _, path := range []string{directory, filepath.Join(directory, "forwarding.json")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("handoff custody is not private", err)
		}
	}
	if err := WriteBundle(directory, bundle); err == nil {
		t.Fatal("existing handoff was replaced")
	}
	again, err := os.ReadFile(filepath.Join(directory, "forwarding.json"))
	if err != nil || string(again) != string(data) {
		t.Fatal("refused replay changed retained handoff", err)
	}
}

func TestForwardingHandoffRefusesUnboundTargetsBeforeCreatingOutput(t *testing.T) {
	_, original, _ := forwardingFixture(t)
	for _, change := range []string{"missing", "duplicate", "foreign-deployment", "invalid-node", "invalid-pin", "uppercase-pin", "public-native-target", "changed-private-port", "zero-sequence", "future-sequence", "stale", "future", "deadline"} {
		t.Run(change, func(t *testing.T) {
			bundle := original
			bundle.Forwards = append([]Forward(nil), original.Forwards...)
			forward := &bundle.Forwards[0]
			switch change {
			case "missing":
				bundle.Forwards = nil
			case "duplicate":
				bundle.Forwards = append(bundle.Forwards, *forward)
			case "foreign-deployment":
				forward.DeploymentID = requestTwo
				forward.NativeEndpoint = nodeagent.LocalEndpoint(requestTwo)
			case "invalid-node":
				forward.NodeID = "node; command"
			case "invalid-pin":
				forward.NodeKeySHA256 = "not-a-node-pin"
			case "uppercase-pin":
				forward.NodeKeySHA256 = strings.Repeat("AB", 32)
			case "public-native-target":
				forward.NativeEndpoint = "https://" + bundle.Inventory.Endpoints[0].Hostname
			case "changed-private-port":
				forward.NativeEndpoint += "0"
			case "zero-sequence":
				forward.ObservationSequence = 0
			case "future-sequence":
				forward.ObservationSequence = bundle.Inventory.Generation + 1
			case "stale":
				forward.ObservedAt = time.Now().Add(-ReadbackLifetime - time.Second)
			case "future":
				forward.ObservedAt = time.Now().Add(2 * time.Minute)
			case "deadline":
				forward.ObservedAt = forward.ObservedAt.Add(-time.Second)
			}
			directory := filepath.Join(t.TempDir(), "refused")
			if err := WriteBundle(directory, bundle); !errors.Is(err, ErrUnqualified) {
				t.Fatal("unbound handoff accepted", err)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("refused handoff produced output", err)
			}
		})
	}
}

func TestForwardingHandoffRefusesSameNodeListenerCollision(t *testing.T) {
	_, bundle, _ := forwardingFixture(t)
	other := bundle.Forwards[0]
	// Preserve the first four UUID digits, which determine the native listener.
	last := "0"
	if strings.HasSuffix(other.DeploymentID, last) {
		last = "1"
	}
	other.DeploymentID = other.DeploymentID[:len(other.DeploymentID)-1] + last
	other.NativeEndpoint = nodeagent.LocalEndpoint(other.DeploymentID)
	bundle.Forwards = append(bundle.Forwards, other)
	bundle.Inventory.Endpoints = append(bundle.Inventory.Endpoints, managededge.Endpoint{DeploymentID: other.DeploymentID})
	if _, err := forwardingBytes(bundle, nil, time.Now()); !errors.Is(err, ErrUnqualified) {
		t.Fatal("same node/listener assigned to multiple deployments", err)
	}
}
