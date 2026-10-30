package managedrouting

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
)

type RetirementRequest struct {
	DeploymentID           string `json:"deploymentId"`
	NodeID                 string `json:"nodeId"`
	Key                    string `json:"key"`
	NativeBackupSHA256     string `json:"nativeBackupSha256"`
	ManagementBackupSHA256 string `json:"managementBackupSha256"`
}
type Retirement struct {
	RetirementRequest
	Actor      string    `json:"actor"`
	Phase      string    `json:"phase"`
	PreparedAt time.Time `json:"preparedAt"`
}
type RetirementInspector interface {
	ManagedStatus(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error)
	Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error)
}

func digestBytes(s string) ([]byte, error) {
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 32 || hex.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}
func retirement(row pgx.Row) (Retirement, []byte, error) {
	var r Retirement
	var pin []byte
	e := row.Scan(&r.DeploymentID, &r.NodeID, &r.Key, &r.NativeBackupSHA256, &r.ManagementBackupSHA256, &r.Actor, &r.Phase, &r.PreparedAt, &pin)
	return r, pin, e
}

const retirementColumns = `deployment_id::text,node_id::text,request_key::text,encode(native_backup_sha256,'hex'),encode(management_backup_sha256,'hex'),actor,phase,prepared_at,node_key_hash`

// CheckActiveTransaction prevents managed reconciliation after retirement preparation.
func CheckActiveTransaction(ctx context.Context, tx pgx.Tx, id string) error {
	if e := gate(ctx, tx); e != nil {
		return e
	}
	var retired bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM managed_endpoint_retirements WHERE deployment_id=$1)`, id).Scan(&retired); e != nil {
		return e
	}
	if retired {
		return ErrConflict
	}
	return nil
}

// PrepareRetirement fences management readiness without freeing a slot or deleting evidence.
// Backup digests are operator custody assertions, not proof that a remote archive exists.
func (s Store) PrepareRetirement(ctx context.Context, actor string, request RetirementRequest, inspector RetirementInspector) (Retirement, error) {
	for _, id := range []string{request.DeploymentID, request.NodeID, request.Key} {
		if _, e := managednamespace.Endpoint(id); e != nil {
			return Retirement{}, ErrInvalid
		}
	}
	native, e := digestBytes(request.NativeBackupSHA256)
	if e != nil {
		return Retirement{}, e
	}
	management, e := digestBytes(request.ManagementBackupSHA256)
	if e != nil {
		return Retirement{}, e
	}
	if !actorValid(actor) || inspector == nil {
		return Retirement{}, ErrInvalid
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return Retirement{}, e
	}
	defer tx.Rollback(ctx)
	var state, host, kind string
	if e = tx.QueryRow(ctx, `SELECT d.state,d.managed_hostname,d.kind FROM deployments d JOIN managed_public_endpoints e ON e.deployment_id=d.id WHERE d.id=$1 FOR UPDATE OF d,e`, request.DeploymentID).Scan(&state, &host, &kind); e != nil {
		return Retirement{}, e
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	old, _, e := retirement(tx.QueryRow(ctx, `SELECT `+retirementColumns+` FROM managed_endpoint_retirements WHERE deployment_id=$1`, request.DeploymentID))
	if e == nil {
		if old.RetirementRequest != request || old.Actor != actor {
			return Retirement{}, ErrConflict
		}
		return old, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Retirement{}, e
	}
	expected, _ := managednamespace.Endpoint(request.DeploymentID)
	if kind != "agent" || state != "ready" || host != expected {
		return Retirement{}, ErrConflict
	}
	peer, e := lockedPeer(ctx, tx, request.DeploymentID)
	if e != nil {
		return Retirement{}, e
	}
	if peer.NodeID != request.NodeID {
		return Retirement{}, ErrConflict
	}
	var busy bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM river_job WHERE args->>'deploymentId'=$1 AND state NOT IN ('completed','cancelled','discarded')) OR EXISTS(SELECT 1 FROM enrollment_operations WHERE deployment_id=$1::uuid)`, request.DeploymentID).Scan(&busy); e != nil {
		return Retirement{}, e
	}
	if busy {
		return Retirement{}, ErrConflict
	}
	status, e := inspector.ManagedStatus(ctx, peer, request.DeploymentID)
	if e != nil || status.ID != request.DeploymentID || !status.Healthy || status.State != "running" || status.Endpoint != "https://"+host {
		return Retirement{}, ErrUnqualified
	}
	usage, e := inspector.Usage(ctx, peer, request.DeploymentID)
	if e != nil || usage.Validate(request.DeploymentID) != nil || usage.Accounts != 0 {
		return Retirement{}, ErrUnqualified
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	result, _, e := retirement(tx.QueryRow(ctx, `INSERT INTO managed_endpoint_retirements(deployment_id,node_id,request_key,native_backup_sha256,management_backup_sha256,actor,node_key_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+retirementColumns, request.DeploymentID, request.NodeID, request.Key, native, management, actor, peer.PublicKeyHash))
	if e != nil {
		return Retirement{}, e
	}
	message := "Empty managed fixture retirement prepared; serving is disabled and capacity remains reserved."
	if _, e = tx.Exec(ctx, `UPDATE deployments SET state='failed',endpoint_url=NULL,message=$2,updated_at=now() WHERE id=$1`, request.DeploymentID, message); e != nil {
		return Retirement{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'failed',$2)`, request.DeploymentID, message); e != nil {
		return Retirement{}, e
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	return result, tx.Commit(ctx)
}

// ReleaseRetirement requires independently delivered withdrawal/native receipts and authenticated absence.
// It retains all endpoint reservations and history; host deletion is a separate restricted operation.
func (s Store) ReleaseRetirement(ctx context.Context, actor, id, key, withdrawal, nativeReceipt string, inspector RetirementInspector) (Retirement, error) {
	w, e := digestBytes(withdrawal)
	if e != nil {
		return Retirement{}, e
	}
	n, e := digestBytes(nativeReceipt)
	if e != nil {
		return Retirement{}, e
	}
	if !actorValid(actor) || inspector == nil {
		return Retirement{}, ErrInvalid
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return Retirement{}, e
	}
	defer tx.Rollback(ctx)
	var state, serving string
	if e = tx.QueryRow(ctx, `SELECT state,coalesce(endpoint_url,'') FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&state, &serving); e != nil {
		return Retirement{}, e
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	r, pin, e := retirement(tx.QueryRow(ctx, `SELECT `+retirementColumns+` FROM managed_endpoint_retirements WHERE deployment_id=$1 FOR UPDATE`, id))
	if e != nil {
		return Retirement{}, e
	}
	if r.Actor != actor || r.Key != key || state != "failed" || serving != "" {
		return Retirement{}, ErrConflict
	}
	if r.Phase == "released" {
		var priorW, priorN []byte
		if e = tx.QueryRow(ctx, `SELECT withdrawal_sha256,native_retirement_sha256 FROM managed_endpoint_retirements WHERE deployment_id=$1`, id).Scan(&priorW, &priorN); e != nil {
			return Retirement{}, e
		}
		if !bytes.Equal(w, priorW) || !bytes.Equal(n, priorN) {
			return Retirement{}, ErrConflict
		}
		return r, tx.Commit(ctx)
	}
	peer, e := lockedPeer(ctx, tx, id)
	if e != nil {
		return Retirement{}, e
	}
	if peer.NodeID != r.NodeID || !bytes.Equal(peer.PublicKeyHash, pin) {
		return Retirement{}, ErrConflict
	}
	var busy bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM river_job WHERE args->>'deploymentId'=$1 AND state NOT IN ('completed','cancelled','discarded'))`, id).Scan(&busy); e != nil {
		return Retirement{}, e
	}
	if busy {
		return Retirement{}, ErrConflict
	}
	capacity, e := inspector.Capacity(ctx, peer)
	if e != nil || capacity.MaximumSlots != 1 || capacity.CPUs != 3 || len(capacity.InstanceIDs) != 0 {
		return Retirement{}, ErrUnqualified
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE managed_endpoint_retirements SET phase='released',released_at=clock_timestamp(),withdrawal_sha256=$2,native_retirement_sha256=$3 WHERE deployment_id=$1`, id, w, n); e != nil {
		return Retirement{}, e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM instance_placements WHERE deployment_id=$1`, id); e != nil {
		return Retirement{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE deployments SET message='Empty managed fixture retired; backup and endpoint history retained, placement released.',updated_at=now() WHERE id=$1`, id); e != nil {
		return Retirement{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'failed','Empty managed fixture retired; backup and endpoint history retained, placement released.')`, id); e != nil {
		return Retirement{}, e
	}
	if e = gate(ctx, tx); e != nil {
		return Retirement{}, e
	}
	r.Phase = "released"
	return r, tx.Commit(ctx)
}
