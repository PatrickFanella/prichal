// Package managedrouting retains public endpoint identity and authenticated edge readback separately from native placement.
package managedrouting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConflict = errors.New("managed endpoint conflicts with retained state")
var ErrUnqualified = errors.New("managed endpoint inventory requires current qualified readback")
var ErrInvalid = errors.New("invalid managed endpoint operator request")

const ReadbackLifetime = managededge.InventoryLifetime

type Store struct{ Pool *pgxpool.Pool }
type Endpoint struct {
	DeploymentID string    `json:"deploymentId"`
	Hostname     string    `json:"hostname"`
	ForwardPort  int       `json:"forwardPort"`
	Basis        string    `json:"basis"`
	Actor        string    `json:"actor"`
	ReservedAt   time.Time `json:"reservedAt"`
}
type Observation struct {
	Sequence     int64     `json:"sequence"`
	DeploymentID string    `json:"deploymentId"`
	RequestKey   string    `json:"requestKey"`
	NodeID       string    `json:"nodeId"`
	Actor        string    `json:"actor"`
	ObservedAt   time.Time `json:"observedAt"`
}
type Inspector interface {
	IngressCertificate(context.Context, nodeagent.Peer, string) (nodeagent.IngressCertificate, error)
}
type EmptyInspector interface {
	Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error)
}

func actorValid(actor string) bool {
	return len(actor) > 0 && len(actor) <= 128 && strings.TrimSpace(actor) == actor && !strings.ContainsAny(actor, "\x00\r\n\t")
}
func gate(ctx context.Context, tx pgx.Tx) error {
	var database string
	var held, reconciled bool
	if err := tx.QueryRow(ctx, `SELECT current_database(),EXISTS(SELECT 1 FROM management_recovery WHERE state='held'),EXISTS(SELECT 1 FROM management_recovery WHERE state='reconciled')`).Scan(&database, &held, &reconciled); err != nil {
		return err
	}
	if held || strings.HasPrefix(database, "pds_recovery_") && !reconciled {
		return recovery.ErrHeld
	}
	return nil
}

// CheckTransaction rechecks routing authority within a caller-owned intent transaction.
func CheckTransaction(ctx context.Context, tx pgx.Tx) error { return gate(ctx, tx) }

func (s Store) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := gate(ctx, tx); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func scanEndpoint(row pgx.Row) (Endpoint, error) {
	var e Endpoint
	err := row.Scan(&e.DeploymentID, &e.Hostname, &e.ForwardPort, &e.Basis, &e.Actor, &e.ReservedAt)
	return e, err
}

const endpointColumns = `deployment_id::text,hostname,forward_port,reservation_basis,actor,reserved_at`

// Reserve permits only queued, unplaced native intent. It creates no runtime or route.
func (s Store) Reserve(ctx context.Context, actor, id string) (Endpoint, error) {
	return s.reserve(ctx, actor, id, nil)
}

// ReserveQueuedTx joins endpoint reservation to the caller's queued intent/job commit.
// Any error requires the caller to roll back the entire transaction.
func ReserveQueuedTx(ctx context.Context, tx pgx.Tx, actor, id string) (Endpoint, error) {
	return reserveTx(ctx, tx, actor, id, nil)
}

// ReserveEmptyFixture is an explicit operator conversion reservation, not authority to rename the runtime.
// Its caller must constrain execution to designated disposable infrastructure.
func (s Store) ReserveEmptyFixture(ctx context.Context, actor, id string, inspector EmptyInspector) (Endpoint, error) {
	if inspector == nil {
		return Endpoint{}, ErrInvalid
	}
	return s.reserve(ctx, actor, id, inspector)
}
func (s Store) reserve(ctx context.Context, actor, id string, inspector EmptyInspector) (Endpoint, error) {
	if _, err := managednamespace.Endpoint(id); err != nil || !actorValid(actor) {
		return Endpoint{}, ErrInvalid
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	defer tx.Rollback(ctx)
	result, err := reserveTx(ctx, tx, actor, id, inspector)
	if err != nil {
		return Endpoint{}, err
	}
	return result, tx.Commit(ctx)
}

func reserveTx(ctx context.Context, tx pgx.Tx, actor, id string, inspector EmptyInspector) (Endpoint, error) {
	hostname, err := managednamespace.Endpoint(id)
	if err != nil || !actorValid(actor) {
		return Endpoint{}, ErrInvalid
	}
	if err := gate(ctx, tx); err != nil {
		return Endpoint{}, err
	}
	var kind, state, recordedHostname, recordedEndpoint string
	if err := tx.QueryRow(ctx, `SELECT kind,state,managed_hostname,coalesce(endpoint_url,'') FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&kind, &state, &recordedHostname, &recordedEndpoint); err != nil {
		return Endpoint{}, err
	}
	if err := gate(ctx, tx); err != nil {
		return Endpoint{}, err
	}
	existing, err := scanEndpoint(tx.QueryRow(ctx, `SELECT `+endpointColumns+` FROM managed_public_endpoints WHERE deployment_id=$1`, id))
	if err == nil {
		if err := gate(ctx, tx); err != nil {
			return Endpoint{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, err
	}
	if kind != "agent" {
		return Endpoint{}, ErrConflict
	}
	basis := "queued"
	if inspector == nil {
		var placed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM instance_placements WHERE deployment_id=$1)`, id).Scan(&placed); err != nil {
			return Endpoint{}, err
		}
		if state != "queued" || placed {
			return Endpoint{}, ErrConflict
		}
	} else {
		if state != "ready" || recordedHostname != id+".localhost" {
			return Endpoint{}, ErrConflict
		}
		peer, err := lockedPeer(ctx, tx, id)
		if err != nil {
			return Endpoint{}, err
		}
		status, err := inspector.Status(ctx, peer, id)
		if err != nil || status.ID != id || status.Endpoint != nodeagent.LocalEndpoint(id) || status.Endpoint != recordedEndpoint || status.State != "running" || !status.Healthy {
			return Endpoint{}, ErrUnqualified
		}
		usage, err := inspector.Usage(ctx, peer, id)
		if err != nil || usage.Validate(id) != nil || usage.Accounts != 0 {
			return Endpoint{}, ErrUnqualified
		}
		basis = "empty_fixture"
	}
	if err := gate(ctx, tx); err != nil {
		return Endpoint{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(73647304)`); err != nil {
		return Endpoint{}, err
	}
	if err := gate(ctx, tx); err != nil {
		return Endpoint{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM managed_public_endpoints`).Scan(&count); err != nil {
		return Endpoint{}, err
	}
	if count >= 100 {
		return Endpoint{}, ErrConflict
	}
	var port int
	if err := tx.QueryRow(ctx, `SELECT candidate FROM generate_series(35000,35999) candidate WHERE NOT EXISTS(SELECT 1 FROM managed_public_endpoints WHERE forward_port=candidate) ORDER BY candidate LIMIT 1`).Scan(&port); err != nil {
		return Endpoint{}, err
	}
	result, err := scanEndpoint(tx.QueryRow(ctx, `INSERT INTO managed_public_endpoints(deployment_id,hostname,forward_port,reservation_basis,actor) VALUES($1,$2,$3,$4,$5) RETURNING `+endpointColumns, id, hostname, port, basis, actor))
	if err != nil {
		return Endpoint{}, err
	}
	if err := gate(ctx, tx); err != nil {
		return Endpoint{}, err
	}
	return result, nil
}
func lockedPeer(ctx context.Context, tx pgx.Tx, id string) (nodeagent.Peer, error) {
	var peer nodeagent.Peer
	err := tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining') AND n.enrolled_at IS NOT NULL AND octet_length(n.public_key_hash)=32 FOR UPDATE OF p,n`, id).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return peer, ErrUnqualified
	}
	return peer, err
}
func scanObservation(row pgx.Row) (Observation, error) {
	var o Observation
	err := row.Scan(&o.Sequence, &o.DeploymentID, &o.RequestKey, &o.NodeID, &o.Actor, &o.ObservedAt)
	return o, err
}

const observationColumns = `sequence,deployment_id::text,request_key::text,node_id::text,actor,observed_at`

// Observe authenticates current placement and appends immutable public-certificate evidence under a stable request key.
func (s Store) Observe(ctx context.Context, actor, id, node, key string, inspector Inspector) (Observation, error) {
	for _, identity := range []string{id, node, key} {
		if _, err := managednamespace.Endpoint(identity); err != nil {
			return Observation{}, ErrInvalid
		}
	}
	if !actorValid(actor) || inspector == nil {
		return Observation{}, ErrInvalid
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Observation{}, err
	}
	defer tx.Rollback(ctx)
	var state, hostname, basis string
	var converted bool
	if err := tx.QueryRow(ctx, `SELECT d.state,e.hostname,e.reservation_basis,EXISTS(SELECT 1 FROM managed_native_conversions WHERE deployment_id=d.id) FROM deployments d JOIN managed_public_endpoints e ON e.deployment_id=d.id WHERE d.id=$1 AND d.kind='agent' FOR UPDATE OF d,e`, id).Scan(&state, &hostname, &basis, &converted); err != nil {
		return Observation{}, err
	}
	if err := gate(ctx, tx); err != nil {
		return Observation{}, err
	}
	previous, err := scanObservation(tx.QueryRow(ctx, `SELECT `+observationColumns+` FROM managed_edge_observations WHERE deployment_id=$1 AND request_key=$2`, id, key))
	if err == nil {
		if previous.NodeID != node || previous.Actor != actor {
			return Observation{}, ErrConflict
		}
		if err := gate(ctx, tx); err != nil {
			return Observation{}, err
		}
		return previous, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, err
	}
	if state != "checking" && state != "ready" {
		return Observation{}, ErrUnqualified
	}
	peer, err := lockedPeer(ctx, tx, id)
	if err != nil {
		return Observation{}, err
	}
	if peer.NodeID != node {
		return Observation{}, ErrConflict
	}
	value, err := inspector.IngressCertificate(ctx, peer, id)
	if err != nil || value.Validate(id) != nil || (basis == "queued" || converted) && value.Endpoint != "https://"+hostname {
		return Observation{}, ErrUnqualified
	}
	if err := gate(ctx, tx); err != nil {
		return Observation{}, err
	}
	hash := sha256.Sum256([]byte(value.Certificate))
	result, err := scanObservation(tx.QueryRow(ctx, `INSERT INTO managed_edge_observations(deployment_id,request_key,node_id,node_key_hash,certificate_pem,certificate_sha256,actor) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+observationColumns, id, key, node, peer.PublicKeyHash, []byte(value.Certificate), hash[:], actor))
	if err != nil {
		return Observation{}, err
	}
	if err := gate(ctx, tx); err != nil {
		return Observation{}, err
	}
	return result, tx.Commit(ctx)
}

type Bundle struct {
	Inventory    managededge.Inventory
	Certificates map[string][]byte
	Forwards     []Forward
}

// Export refuses incomplete/stale inventories and never omits a retained endpoint to produce a partial candidate.
func (s Store) Export(ctx context.Context) (Bundle, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Bundle{}, err
	}
	defer tx.Rollback(ctx)
	if err := gate(ctx, tx); err != nil {
		return Bundle{}, err
	}
	rows, err := tx.Query(ctx, `SELECT e.deployment_id::text,e.hostname,e.forward_port,d.kind,d.state,coalesce(o.sequence,0),coalesce(o.node_id::text,''),o.node_key_hash,o.certificate_pem,o.certificate_sha256,o.observed_at,coalesce(p.node_id::text,''),coalesce(n.state,''),n.public_key_hash,n.enrolled_at,(SELECT coalesce(max(sequence),0) FROM managed_edge_observations) FROM managed_public_endpoints e JOIN deployments d ON d.id=e.deployment_id LEFT JOIN LATERAL(SELECT * FROM managed_edge_observations WHERE deployment_id=e.deployment_id ORDER BY sequence DESC LIMIT 1) o ON true LEFT JOIN instance_placements p ON p.deployment_id=e.deployment_id LEFT JOIN hosting_nodes n ON n.id=p.node_id WHERE NOT EXISTS(SELECT 1 FROM managed_endpoint_retirements r WHERE r.deployment_id=e.deployment_id AND r.phase='released') ORDER BY e.deployment_id LIMIT 101`)
	if err != nil {
		return Bundle{}, err
	}
	result := Bundle{Inventory: managededge.Inventory{Version: 1, Endpoints: []managededge.Endpoint{}, Handles: []managededge.Handle{}}, Certificates: map[string][]byte{}}
	now := time.Now()
	for rows.Next() {
		var id, host, kind, state, observedNode, currentNode, nodeState string
		var port int
		var sequence, generation int64
		var observedPin, currentPin, certificate, hash []byte
		var observedAt, enrolledAt *time.Time
		if err := rows.Scan(&id, &host, &port, &kind, &state, &sequence, &observedNode, &observedPin, &certificate, &hash, &observedAt, &currentNode, &nodeState, &currentPin, &enrolledAt, &generation); err != nil {
			rows.Close()
			return Bundle{}, err
		}
		expected, nameErr := managednamespace.Endpoint(id)
		if nameErr != nil || host != expected || kind != "agent" || (state != "checking" && state != "ready") || sequence == 0 || observedAt == nil || now.Sub(*observedAt) > ReadbackLifetime || observedAt.After(now.Add(time.Minute)) || enrolledAt == nil || observedNode != currentNode || !bytes.Equal(observedPin, currentPin) || len(currentPin) != 32 || (nodeState != "active" && nodeState != "draining") || len(hash) != 32 || port < 35000 || port > 35999 || managededge.ValidateCertificate(id, certificate, hex.EncodeToString(hash), now) != nil {
			rows.Close()
			return Bundle{}, ErrUnqualified
		}
		deadline := observedAt.Add(ReadbackLifetime)
		block, _ := pem.Decode(certificate)
		parsed, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			rows.Close()
			return Bundle{}, ErrUnqualified
		}
		if parsed.NotAfter.Before(deadline) {
			deadline = parsed.NotAfter
		}
		if result.Inventory.ExpiresAt == nil || deadline.Before(*result.Inventory.ExpiresAt) {
			copyDeadline := deadline
			result.Inventory.ExpiresAt = &copyDeadline
		}
		result.Inventory.Generation = generation
		result.Inventory.Endpoints = append(result.Inventory.Endpoints, managededge.Endpoint{DeploymentID: id, Hostname: host, ForwardPort: port, CertificateSHA256: hex.EncodeToString(hash)})
		result.Certificates[id] = append([]byte(nil), certificate...)
		result.Forwards = append(result.Forwards, Forward{DeploymentID: id, NodeID: currentNode, NodeKeySHA256: hex.EncodeToString(currentPin), NativeEndpoint: nodeagent.LocalEndpoint(id), ObservationSequence: sequence, ObservedAt: *observedAt})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Bundle{}, err
	}
	if len(result.Inventory.Endpoints) == 0 || len(result.Inventory.Endpoints) > 100 {
		return Bundle{}, ErrUnqualified
	}
	if err := gate(ctx, tx); err != nil {
		return Bundle{}, err
	}
	return result, tx.Commit(ctx)
}
