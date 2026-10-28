package deployment

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// EmptyConversionRequest contains operator custody digests only, never native secrets.
type EmptyConversionRequest struct {
	Key                    string `json:"requestKey"`
	NativeBackupSHA256     string `json:"nativeBackupSha256"`
	ManagementBackupSHA256 string `json:"managementBackupSha256"`
	OperatorReceiptSHA256  string `json:"operatorReceiptSha256"`
	CredentialsSHA256      string `json:"credentialsSha256"`
	CertificateSHA256      string `json:"certificateSha256"`
	NativeReceiptSHA256    string `json:"nativeReceiptSha256"`
}

func (r EmptyConversionRequest) Validate() error {
	if _, err := managednamespace.Endpoint(r.Key); err != nil {
		return ErrInvalid
	}
	for _, v := range []string{r.NativeBackupSHA256, r.ManagementBackupSHA256, r.OperatorReceiptSHA256, r.CredentialsSHA256, r.CertificateSHA256, r.NativeReceiptSHA256} {
		b, err := hex.DecodeString(v)
		if err != nil || len(b) != 32 || strings.ToLower(v) != v {
			return ErrInvalid
		}
	}
	return nil
}

type EmptyConversion struct {
	DeploymentID string    `json:"deploymentId"`
	NodeID       string    `json:"nodeId"`
	Key          string    `json:"requestKey"`
	Actor        string    `json:"actor"`
	ConfirmedAt  time.Time `json:"confirmedAt"`
}
type ManagedConversionInspector interface {
	ManagedStatus(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error)
	IngressCertificate(context.Context, nodeagent.Peer, string) (nodeagent.IngressCertificate, error)
}

// ConfirmEmptyManagedConversion records an operator-performed disposable conversion.
// It is deliberately absent from customer HTTP handlers. The command must enforce its designated staging scope.
func (s *Store) ConfirmEmptyManagedConversion(ctx context.Context, actor, id, node string, r EmptyConversionRequest, inspector ManagedConversionInspector) (EmptyConversion, error) {
	host, err := managednamespace.Endpoint(id)
	if err != nil || r.Validate() != nil || !s.Staging || inspector == nil || actor == "" || len(actor) > 128 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\x00\r\n\t") {
		return EmptyConversion{}, ErrInvalid
	}
	for _, v := range []string{node, r.Key} {
		if _, err := managednamespace.Endpoint(v); err != nil {
			return EmptyConversion{}, ErrInvalid
		}
	}
	var hashes [][]byte
	for _, v := range []string{r.NativeBackupSHA256, r.ManagementBackupSHA256, r.OperatorReceiptSHA256, r.CredentialsSHA256, r.CertificateSHA256, r.NativeReceiptSHA256} {
		b, err := hex.DecodeString(v)
		if err != nil || len(b) != 32 || v != strings.ToLower(v) {
			return EmptyConversion{}, ErrInvalid
		}
		hashes = append(hashes, b)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return EmptyConversion{}, err
	}
	defer tx.Rollback(ctx)
	if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
		return EmptyConversion{}, err
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM deployments WHERE id=$1 AND kind='agent' FOR UPDATE`, id).Scan(&locked); err != nil {
		return EmptyConversion{}, err
	}
	if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
		return EmptyConversion{}, err
	}
	var previous EmptyConversion
	var storedHashes [][]byte = make([][]byte, 6)
	err = tx.QueryRow(ctx, `SELECT deployment_id::text,node_id::text,request_key::text,actor,confirmed_at,native_backup_sha256,management_backup_sha256,operator_receipt_sha256,credentials_sha256,certificate_sha256,native_receipt_sha256 FROM managed_native_conversions WHERE deployment_id=$1`, id).Scan(&previous.DeploymentID, &previous.NodeID, &previous.Key, &previous.Actor, &previous.ConfirmedAt, &storedHashes[0], &storedHashes[1], &storedHashes[2], &storedHashes[3], &storedHashes[4], &storedHashes[5])
	if err == nil {
		if previous.NodeID != node || previous.Key != r.Key || previous.Actor != actor {
			return EmptyConversion{}, ErrConflict
		}
		for i := range hashes {
			if !bytes.Equal(hashes[i], storedHashes[i]) {
				return EmptyConversion{}, ErrConflict
			}
		}
		if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
			return EmptyConversion{}, err
		}
		return previous, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EmptyConversion{}, err
	}
	var state, hostname, endpoint, basis, publicHost string
	if err := tx.QueryRow(ctx, `SELECT d.state,d.managed_hostname,coalesce(d.endpoint_url,''),e.reservation_basis,e.hostname FROM deployments d JOIN managed_public_endpoints e ON e.deployment_id=d.id WHERE d.id=$1 AND d.kind='agent' FOR UPDATE OF d,e`, id).Scan(&state, &hostname, &endpoint, &basis, &publicHost); err != nil {
		return EmptyConversion{}, err
	}
	if state != "ready" || hostname != id+".localhost" || endpoint != nodeagent.LocalEndpoint(id) || basis != "empty_fixture" || publicHost != host {
		return EmptyConversion{}, ErrConflict
	}
	var unfinished bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM river_job WHERE args->>'deploymentId'=$1 AND state NOT IN ('completed','cancelled','discarded'))`, id).Scan(&unfinished); err != nil {
		return EmptyConversion{}, err
	}
	if unfinished {
		return EmptyConversion{}, ErrConflict
	}
	var peer nodeagent.Peer
	if err := tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.id=$2 AND n.state='active' AND n.enrolled_at IS NOT NULL FOR UPDATE OF p,n`, id, node).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return EmptyConversion{}, err
	}
	var retainedCertificate []byte
	if err := tx.QueryRow(ctx, `SELECT certificate_sha256 FROM managed_edge_observations WHERE deployment_id=$1 AND node_id=$2 AND node_key_hash=$3 ORDER BY sequence DESC LIMIT 1`, id, node, peer.PublicKeyHash).Scan(&retainedCertificate); err != nil || !bytes.Equal(retainedCertificate, hashes[4]) {
		return EmptyConversion{}, ErrConflict
	}
	if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
		return EmptyConversion{}, err
	}
	status, err := inspector.ManagedStatus(ctx, peer, id)
	if err != nil || status.ID != id || status.State != "running" || !status.Healthy || status.Endpoint != "https://"+host {
		return EmptyConversion{}, ErrConflict
	}
	usage, err := inspector.Usage(ctx, peer, id)
	if err != nil || usage.Validate(id) != nil || usage.Accounts != 0 {
		return EmptyConversion{}, ErrConflict
	}
	certificate, err := inspector.IngressCertificate(ctx, peer, id)
	if err != nil || certificate.Validate(id) != nil || certificate.Endpoint != "https://"+host || certificate.SHA256 != r.CertificateSHA256 {
		return EmptyConversion{}, ErrConflict
	}
	if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
		return EmptyConversion{}, err
	}
	result := EmptyConversion{DeploymentID: id, NodeID: node, Key: r.Key, Actor: actor}
	if err := tx.QueryRow(ctx, `INSERT INTO managed_native_conversions(deployment_id,request_key,node_id,node_key_hash,previous_hostname,previous_endpoint,native_backup_sha256,management_backup_sha256,operator_receipt_sha256,credentials_sha256,certificate_sha256,native_receipt_sha256,actor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING confirmed_at`, id, r.Key, node, peer.PublicKeyHash, hostname, endpoint, hashes[0], hashes[1], hashes[2], hashes[3], hashes[4], hashes[5], actor).Scan(&result.ConfirmedAt); err != nil {
		return EmptyConversion{}, err
	}
	message := "Empty staging fixture converted. Confirming managed resources; public readiness is pending."
	if _, err := tx.Exec(ctx, `UPDATE deployments SET managed_hostname=$2,endpoint_url=NULL,state='configuring',message=$3,updated_at=now() WHERE id=$1`, id, host, message); err != nil {
		return EmptyConversion{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'configuring',$2)`, id, message); err != nil {
		return EmptyConversion{}, err
	}
	if _, err := s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3, Queue: ManagedIntentQueue}); err != nil {
		return EmptyConversion{}, err
	}
	if err := managedrouting.CheckTransaction(ctx, tx); err != nil {
		return EmptyConversion{}, err
	}
	return result, tx.Commit(ctx)
}
