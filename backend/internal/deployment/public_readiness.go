package deployment

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedcheck"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const PublicRetryDelay = 15 * time.Second

var ErrPublicCheckPending = errors.New("public endpoint verification will retry")

type publicBinding struct {
	state, hostname, serving, basis string
	peer                            nodeagent.Peer
	converted                       bool
}

func lockPublicBinding(ctx context.Context, tx pgx.Tx, id string) (publicBinding, error) {
	var b publicBinding
	err := tx.QueryRow(ctx, `SELECT d.state,d.managed_hostname,coalesce(d.endpoint_url,''),e.reservation_basis,n.id::text,n.agent_endpoint,n.public_key_hash,EXISTS(SELECT 1 FROM managed_native_conversions c WHERE c.deployment_id=d.id AND c.node_id=n.id AND c.node_key_hash=n.public_key_hash) FROM deployments d JOIN managed_public_endpoints e ON e.deployment_id=d.id JOIN instance_placements p ON p.deployment_id=d.id JOIN hosting_nodes n ON n.id=p.node_id WHERE d.id=$1 AND d.kind='agent' AND e.hostname=d.managed_hostname AND n.state='active' AND n.enrolled_at IS NOT NULL FOR UPDATE OF d,e,p,n`, id).Scan(&b.state, &b.hostname, &b.serving, &b.basis, &b.peer.NodeID, &b.peer.Endpoint, &b.peer.PublicKeyHash, &b.converted)
	if err != nil {
		return b, err
	}
	expected, err := managednamespace.Endpoint(id)
	if err != nil || b.hostname != expected || len(b.peer.PublicKeyHash) != 32 || (b.basis != "queued" && !(b.basis == "empty_fixture" && b.converted)) {
		return b, ErrConflict
	}
	return b, nil
}

func publicMessage(code string) string {
	switch code {
	case "dns":
		return "Public DNS lookup could not be verified."
	case "tls":
		return "Public HTTPS certificate validation failed."
	case "health":
		return "The public PDS health response could not be verified."
	case "identity":
		return "The public PDS service or OAuth identity did not match."
	case "websocket":
		return "The public PDS WebSocket connection could not be verified."
	case "unknown_host":
		return "Unregistered managed hostname rejection could not be verified."
	case "registration":
		return "The staging PDS registration boundary could not be verified."
	default:
		return "The retained native PDS could not be verified."
	}
}

func (p AgentProvisioner) checkPublic(ctx context.Context, id string) error {
	inspector, ok := p.Client.(interface {
		ManagedStatus(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	})
	if !ok || p.PublicCheck == nil {
		return ErrManagedEndpointPending
	}
	ctx, cancel := context.WithTimeout(ctx, managedcheck.Timeout+10*time.Second)
	defer cancel()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return err
	}
	binding, err := lockPublicBinding(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return err
	}
	if binding.state == "ready" || binding.state == "failed" {
		return tx.Commit(ctx)
	}
	if binding.state != "checking" || binding.serving != "" {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO managed_public_readiness(deployment_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return err
	}
	var cycle, attempts int
	var next time.Time
	if err = tx.QueryRow(ctx, `SELECT cycle,attempts,next_check_at FROM managed_public_readiness WHERE deployment_id=$1 FOR UPDATE`, id).Scan(&cycle, &attempts, &next); err != nil {
		return err
	}
	if attempts >= 3 {
		return ErrConflict
	}
	if time.Now().Before(next) {
		if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return ErrPublicCheckPending
	}
	native, nativeErr := inspector.ManagedStatus(ctx, binding.peer, id)
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return err
	}
	result := managedcheck.Result{Hostname: binding.hostname, Code: "native", ObservedAt: time.Now().UTC(), Addresses: []string{}}
	if nativeErr == nil && native.ID == id && native.Endpoint == "https://"+binding.hostname && native.State == "running" && native.Healthy {
		result = p.PublicCheck.Check(ctx, id)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = result.Validate(id, time.Now()); err != nil {
		return err
	}
	if result.Addresses == nil {
		result.Addresses = []string{}
	}
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return err
	}
	hash, _ := hex.DecodeString(result.CertificateSHA256)
	if len(hash) == 0 {
		hash = nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO managed_public_checks(deployment_id,cycle,attempt,node_id,node_key_hash,hostname,code,addresses,certificate_sha256,certificate_expires_at,native_version,unknown_hostname,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id, cycle, attempts+1, binding.peer.NodeID, binding.peer.PublicKeyHash, binding.hostname, result.Code, result.Addresses, hash, result.CertificateExpiresAt, result.Version, result.UnknownHost, result.ObservedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE managed_public_readiness SET attempts=attempts+1,next_check_at=clock_timestamp()+$2::interval WHERE deployment_id=$1`, id, PublicRetryDelay.String()); err != nil {
		return err
	}
	state, message, endpoint := "checking", publicMessage(result.Code)+" Retrying the same reserved endpoint.", ""
	if result.Code == "ready" {
		state, message, endpoint = "ready", "Public PDS HTTPS, service identity and WebSocket checks passed. Account enrollment is a separate step.", "https://"+binding.hostname
	} else if attempts+1 == 3 {
		state, message = "failed", publicMessage(result.Code)+" Verification stopped after three checks. Resources are retained; an operator can retry this endpoint."
	}
	if _, err = tx.Exec(ctx, `UPDATE deployments SET state=$2,message=$3,endpoint_url=nullif($4,''),updated_at=now() WHERE id=$1`, id, state, message, endpoint); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,$2,$3)`, id, state, message); err != nil {
		return err
	}
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if state == "checking" {
		return ErrPublicCheckPending
	}
	return nil
}

type PublicRetry struct {
	DeploymentID string    `json:"deploymentId"`
	Key          string    `json:"requestKey"`
	NodeID       string    `json:"nodeId"`
	Actor        string    `json:"actor"`
	Cycle        int       `json:"cycle"`
	RequestedAt  time.Time `json:"requestedAt"`
}

// RetryManagedPublic queues the same failed public identity; the operator command constrains its staging scope.
func (s *Store) RetryManagedPublic(ctx context.Context, actor, id, node, key string) (PublicRetry, error) {
	if !s.Staging || actor == "" || len(actor) > 128 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\x00\n\r\t") {
		return PublicRetry{}, ErrInvalid
	}
	for _, v := range []string{id, node, key} {
		if _, err := managednamespace.Endpoint(v); err != nil {
			return PublicRetry{}, ErrInvalid
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PublicRetry{}, err
	}
	defer tx.Rollback(ctx)
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return PublicRetry{}, err
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return PublicRetry{}, err
	}
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return PublicRetry{}, err
	}
	var previous PublicRetry
	err = tx.QueryRow(ctx, `SELECT deployment_id::text,request_key::text,node_id::text,actor,cycle,requested_at FROM managed_public_retries WHERE deployment_id=$1 AND request_key=$2`, id, key).Scan(&previous.DeploymentID, &previous.Key, &previous.NodeID, &previous.Actor, &previous.Cycle, &previous.RequestedAt)
	if err == nil {
		if previous.NodeID != node || previous.Actor != actor {
			return PublicRetry{}, ErrConflict
		}
		if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
			return PublicRetry{}, err
		}
		return previous, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PublicRetry{}, err
	}
	binding, err := lockPublicBinding(ctx, tx, id)
	if err != nil {
		return PublicRetry{}, err
	}
	if binding.state != "failed" || binding.serving != "" || binding.peer.NodeID != node {
		return PublicRetry{}, ErrConflict
	}
	var cycle, attempts int
	if err = tx.QueryRow(ctx, `SELECT cycle,attempts FROM managed_public_readiness WHERE deployment_id=$1 FOR UPDATE`, id).Scan(&cycle, &attempts); err != nil {
		return PublicRetry{}, err
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM managed_public_checks WHERE deployment_id=$1::uuid AND cycle=$2 AND attempt=3 AND code<>'ready' AND node_id=$3 AND node_key_hash=$4) AND NOT EXISTS(SELECT 1 FROM river_job WHERE args->>'deploymentId'=$1::uuid::text AND state NOT IN ('completed','cancelled','discarded'))`, id, cycle, node, binding.peer.PublicKeyHash).Scan(&eligible); err != nil {
		return PublicRetry{}, err
	}
	if attempts != 3 || !eligible {
		return PublicRetry{}, ErrConflict
	}
	result := PublicRetry{DeploymentID: id, Key: key, NodeID: node, Actor: actor, Cycle: cycle + 1}
	if err = tx.QueryRow(ctx, `INSERT INTO managed_public_retries(deployment_id,request_key,node_id,actor,cycle) VALUES($1,$2,$3,$4,$5) RETURNING requested_at`, id, key, node, actor, result.Cycle).Scan(&result.RequestedAt); err != nil {
		return PublicRetry{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE managed_public_readiness SET cycle=cycle+1,attempts=0,next_check_at=clock_timestamp() WHERE deployment_id=$1`, id); err != nil {
		return PublicRetry{}, err
	}
	message := "Retrying public verification for the same retained PDS endpoint."
	if _, err = tx.Exec(ctx, `UPDATE deployments SET state='checking',message=$2,updated_at=now() WHERE id=$1`, id, message); err != nil {
		return PublicRetry{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'checking',$2)`, id, message); err != nil {
		return PublicRetry{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{Queue: ManagedIntentQueue, MaxAttempts: 3}); err != nil {
		return PublicRetry{}, err
	}
	if err = managedrouting.CheckTransaction(ctx, tx); err != nil {
		return PublicRetry{}, err
	}
	return result, tx.Commit(ctx)
}
