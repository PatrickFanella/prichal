package deployment

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/fleet"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedcheck"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrHealthPending requests another bounded readiness check without recreating resources.
var ErrHealthPending = errors.New("PDS health is pending")

// ErrManagedEndpointPending prevents public intent from falling back to a private runtime.
var ErrManagedEndpointPending = errors.New("managed public endpoint activation is pending")

// AgentClient authenticates the registered node before reconciling one instance.
type AgentClient interface {
	Ensure(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error)
}

// AgentProvisioner runs outside the API and owns only worker-to-agent credentials.
type AgentProvisioner struct {
	Pool           *pgxpool.Pool
	Client         AgentClient
	ManagedRuntime bool
	PublicCheck    managedcheck.Checker
}

func (p AgentProvisioner) Step(ctx context.Context, id string) error {
	var state, hostname, reservedHostname, basis, serving string
	var converted bool
	var created, updated time.Time
	if err := p.Pool.QueryRow(ctx, `SELECT d.state,d.created_at,d.updated_at,d.managed_hostname,coalesce(e.hostname,''),coalesce(e.reservation_basis,''),coalesce(d.endpoint_url,''),EXISTS(SELECT 1 FROM managed_native_conversions c JOIN instance_placements p ON p.deployment_id=c.deployment_id JOIN hosting_nodes n ON n.id=p.node_id WHERE c.deployment_id=d.id AND c.node_id=p.node_id AND c.node_key_hash=n.public_key_hash) FROM deployments d LEFT JOIN managed_public_endpoints e ON e.deployment_id=d.id WHERE d.id=$1`, id).Scan(&state, &created, &updated, &hostname, &reservedHostname, &basis, &serving, &converted); err != nil {
		return err
	}
	if state == "ready" || state == "failed" {
		return nil
	}
	managed := basis == "queued" || hostname != id+".localhost"
	var ensure func(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	if p.Client != nil {
		ensure = p.Client.Ensure
	}
	if managed {
		provider, ok := p.Client.(interface {
			EnsureManaged(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
		})
		if !p.ManagedRuntime || !ok {
			return ErrManagedEndpointPending
		}
		expected, err := managednamespace.Endpoint(id)
		if err != nil || (basis != "queued" && !(basis == "empty_fixture" && converted)) || reservedHostname != expected || hostname != expected || serving != "" {
			return errors.New("retained managed intent differs from generated policy")
		}
		if err := p.checkManagedAuthority(ctx, id); err != nil {
			return err
		}
		ensure = provider.EnsureManaged
	}
	store := fleet.Store{Pool: p.Pool}
	if state == "queued" && p.Client != nil {
		nodes, err := store.Nodes(ctx, "local")
		if err != nil {
			return err
		}
		for _, node := range nodes {
			measurements, err := p.Client.Capacity(ctx, nodeagent.Peer{NodeID: node.NodeID, Endpoint: node.Endpoint, PublicKeyHash: node.PublicKeyHash})
			if err != nil {
				if _, invalidateErr := p.Pool.Exec(ctx, `UPDATE hosting_nodes SET last_verified_at=NULL WHERE id=$1`, node.NodeID); invalidateErr != nil {
					return invalidateErr
				}
				continue
			}
			if err = store.RecordCapacity(ctx, node.NodeID, measurements.MaximumSlots, measurements.CPUs, measurements.MemoryBytes, measurements.InstanceIDs); err != nil {
				return err
			}
		}
	}
	placement, err := store.Reserve(ctx, id)
	if err != nil {
		return err
	}
	if state == "queued" {
		if managed {
			if err := p.checkManagedAuthority(ctx, id); err != nil {
				return err
			}
			return p.transition(ctx, id, "queued", "configuring", "Reserved a hosting slot. Configuring managed PDS identity.")
		}
		return p.transition(ctx, id, "queued", "configuring", "Reserved a hosting slot. Configuring the local PDS.")
	}
	if p.Client == nil {
		return errors.New("local agent provisioner is not configured")
	}
	if managed {
		if err := p.checkManagedAuthority(ctx, id); err != nil {
			return err
		}
	}
	instance, err := ensure(ctx, nodeagent.Peer{NodeID: placement.NodeID, Endpoint: placement.Endpoint, PublicKeyHash: placement.PublicKeyHash}, id)
	if managed {
		if err := p.checkManagedAuthority(ctx, id); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	if managed {
		if instance.ID != id || instance.Endpoint != "https://"+hostname {
			return errors.New("native managed identity differs from retained intent")
		}
		if state == "configuring" {
			return p.transition(ctx, id, "configuring", "checking", "Managed PDS resources exist. Public readiness is pending.")
		}
		if state != "checking" {
			return errors.New("unsupported managed deployment checkpoint")
		}
		if !instance.Healthy {
			if time.Since(updated) > 2*time.Minute {
				return errors.New("managed PDS native health did not become ready")
			}
			return ErrHealthPending
		}
		if p.PublicCheck != nil {
			return p.checkPublic(ctx, id)
		}
		return ErrManagedEndpointPending
	}
	if instance.Endpoint != nodeagent.LocalEndpoint(id) {
		return errors.New("managed endpoint identity differs from policy")
	}
	result, err := p.Pool.Exec(ctx, `UPDATE deployments SET endpoint_url=$2 WHERE id=$1 AND managed_hostname=$3 AND (endpoint_url IS NULL OR endpoint_url=$2)`, id, instance.Endpoint, id+".localhost")
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("managed endpoint conflict")
	}
	if state == "configuring" {
		return p.transition(ctx, id, "configuring", "checking", "Local PDS resources exist. Checking internal health.")
	}
	if state == "checking" {
		if !instance.Healthy {
			if time.Since(created) > 2*time.Minute {
				return errors.New("local PDS health did not become ready")
			}
			return ErrHealthPending
		}
		return p.transition(ctx, id, "checking", "ready", "Local PDS and ingress are healthy. Public TLS, identity and federation are unverified.")
	}
	return errors.New("unsupported deployment checkpoint")
}

func (p AgentProvisioner) checkManagedAuthority(ctx context.Context, id string) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := managedrouting.CheckActiveTransaction(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p AgentProvisioner) transition(ctx context.Context, id, previous, next, message string) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE deployments SET state=$3,message=$4,updated_at=now() WHERE id=$1 AND state=$2`, id, previous, next, message)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,$2,$3)`, id, next, message); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Router prevents queued test operations from changing behavior when a real agent is configured.
type Router struct {
	Pool           *pgxpool.Pool
	Agent          AgentClient
	AgentOnly      bool
	ManagedRuntime bool
	PublicCheck    managedcheck.Checker
}

func (p Router) Step(ctx context.Context, id string) error {
	var kind string
	if err := p.Pool.QueryRow(ctx, `SELECT kind FROM deployments WHERE id=$1`, id).Scan(&kind); err != nil {
		return err
	}
	switch kind {
	case "test":
		if p.AgentOnly {
			return errors.New("staging worker refuses simulation jobs")
		}
		return (TestProvisioner{Pool: p.Pool}).Step(ctx, id)
	case "agent":
		if p.Agent == nil {
			return errors.New("local agent provisioner is not configured")
		}
		return (AgentProvisioner{Pool: p.Pool, Client: p.Agent, ManagedRuntime: p.ManagedRuntime, PublicCheck: p.PublicCheck}).Step(ctx, id)
	default:
		return errors.New("unsupported deployment kind")
	}
}
