package managedrouting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
)

var plcDID = regexp.MustCompile(`^did:plc:[a-z2-7]{24}$`)

// HandleBinding is a bidirectionally verified default handle for one managed deployment.
type HandleBinding struct {
	InvitationID string    `json:"invitationId"`
	DeploymentID string    `json:"deploymentId"`
	Hostname     string    `json:"hostname"`
	DID          string    `json:"did"`
	NodeID       string    `json:"nodeId"`
	Actor        string    `json:"actor"`
	VerifiedAt   time.Time `json:"verifiedAt"`
}

// HandleInspector reads the native first account through the enrollment ticket replay.
type HandleInspector interface {
	Enroll(context.Context, nodeagent.Peer, string, string) (nodeagent.Enrollment, error)
}

// DIDDocument holds only the fields a handle binding checks.
type DIDDocument struct {
	AlsoKnownAs []string
	PDSEndpoint string
}

// DIDResolver returns the public identity document for a did:plc.
type DIDResolver interface {
	Resolve(context.Context, string) (DIDDocument, error)
}

// PLCResolver reads did:plc documents from a PLC directory.
type PLCResolver struct {
	Client  *http.Client
	BaseURL string
}

func (r PLCResolver) Resolve(ctx context.Context, did string) (DIDDocument, error) {
	if !plcDID.MatchString(did) || r.Client == nil || r.BaseURL == "" {
		return DIDDocument{}, ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/"+did, nil)
	if err != nil {
		return DIDDocument{}, err
	}
	// Cloudflare refuses some default client user agents before the directory answers.
	request.Header.Set("User-Agent", "prichal-managed-route-admin/1")
	request.Header.Set("Accept", "application/json")
	response, err := r.Client.Do(request)
	if err != nil {
		return DIDDocument{}, errors.New("PLC directory unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 || response.StatusCode != http.StatusOK {
		return DIDDocument{}, errors.New("PLC document unavailable")
	}
	var document struct {
		ID          string   `json:"id"`
		AlsoKnownAs []string `json:"alsoKnownAs"`
		Service     []struct {
			ID              string `json:"id"`
			Type            string `json:"type"`
			ServiceEndpoint string `json:"serviceEndpoint"`
		} `json:"service"`
	}
	if json.NewDecoder(bytes.NewReader(data)).Decode(&document) != nil || document.ID != did {
		return DIDDocument{}, errors.New("PLC document differs from the requested DID")
	}
	result := DIDDocument{AlsoKnownAs: document.AlsoKnownAs}
	for _, service := range document.Service {
		if service.ID == "#atproto_pds" && service.Type == "AtprotoPersonalDataServer" {
			result.PDSEndpoint = service.ServiceEndpoint
		}
	}
	return result, nil
}

const bindingColumns = `invitation_id::text,deployment_id::text,hostname,did,node_id::text,actor,verified_at`

func scanBinding(row pgx.Row) (HandleBinding, error) {
	var b HandleBinding
	err := row.Scan(&b.InvitationID, &b.DeploymentID, &b.Hostname, &b.DID, &b.NodeID, &b.Actor, &b.VerifiedAt)
	return b, err
}

// BindHandle records a default handle only after the native PDS and the public PLC document agree.
// The PDS must report the invitation's generated handle for its first account, and the DID document
// must claim that handle first and name this deployment's public endpoint. Replay returns the record.
func (s Store) BindHandle(ctx context.Context, actor, id, node, invitation string, inspector HandleInspector, resolver DIDResolver) (HandleBinding, error) {
	for _, identity := range []string{id, node, invitation} {
		if _, err := managednamespace.Endpoint(identity); err != nil {
			return HandleBinding{}, ErrInvalid
		}
	}
	hostname, err := managednamespace.Handle(invitation)
	if err != nil || !actorValid(actor) || inspector == nil || resolver == nil {
		return HandleBinding{}, ErrInvalid
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return HandleBinding{}, err
	}
	defer tx.Rollback(ctx)
	var state, endpoint string
	if err := tx.QueryRow(ctx, `SELECT d.state,e.hostname FROM deployments d JOIN managed_public_endpoints e ON e.deployment_id=d.id WHERE d.id=$1 AND d.kind='agent' AND NOT EXISTS(SELECT 1 FROM managed_endpoint_retirements r WHERE r.deployment_id=d.id) FOR UPDATE OF d,e`, id).Scan(&state, &endpoint); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HandleBinding{}, ErrUnqualified
		}
		return HandleBinding{}, err
	}
	expected, _ := managednamespace.Endpoint(id)
	if state != "ready" || endpoint != expected {
		return HandleBinding{}, ErrUnqualified
	}
	previous, err := scanBinding(tx.QueryRow(ctx, `SELECT `+bindingColumns+` FROM managed_handle_bindings WHERE invitation_id=$1`, invitation))
	if err == nil {
		if previous.DeploymentID != id || previous.Hostname != hostname {
			return HandleBinding{}, ErrConflict
		}
		return previous, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return HandleBinding{}, err
	}
	peer, err := lockedPeer(ctx, tx, id)
	if err != nil {
		return HandleBinding{}, err
	}
	if peer.NodeID != node {
		return HandleBinding{}, ErrConflict
	}
	native, err := inspector.Enroll(ctx, peer, id, invitation)
	if err != nil || native.InstanceID != id || (native.State != "account_created" && native.State != "verified") || !plcDID.MatchString(native.DID) || native.Handle != hostname {
		return HandleBinding{}, ErrUnqualified
	}
	document, err := resolver.Resolve(ctx, native.DID)
	if err != nil || len(document.AlsoKnownAs) == 0 || document.AlsoKnownAs[0] != "at://"+hostname || document.PDSEndpoint != "https://"+endpoint {
		return HandleBinding{}, ErrUnqualified
	}
	if err := gate(ctx, tx); err != nil {
		return HandleBinding{}, err
	}
	result, err := scanBinding(tx.QueryRow(ctx, `INSERT INTO managed_handle_bindings(invitation_id,deployment_id,hostname,did,node_id,node_key_hash,actor) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+bindingColumns, invitation, id, hostname, native.DID, node, peer.PublicKeyHash, actor))
	if err != nil {
		return HandleBinding{}, ErrConflict
	}
	return result, tx.Commit(ctx)
}

// exportHandles adds verified bindings for exported endpoints only.
func exportHandles(ctx context.Context, tx pgx.Tx, endpoints []managededge.Endpoint) ([]managededge.Handle, error) {
	ids := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ids = append(ids, endpoint.DeploymentID)
	}
	rows, err := tx.Query(ctx, `SELECT invitation_id::text,deployment_id::text,hostname,did FROM managed_handle_bindings WHERE deployment_id=ANY($1::uuid[]) ORDER BY invitation_id LIMIT 2501`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	handles := []managededge.Handle{}
	for rows.Next() {
		var handle managededge.Handle
		if err := rows.Scan(&handle.InvitationID, &handle.DeploymentID, &handle.Hostname, &handle.DID); err != nil {
			return nil, err
		}
		if expected, err := managednamespace.Handle(handle.InvitationID); err != nil || expected != handle.Hostname || !plcDID.MatchString(handle.DID) {
			return nil, ErrUnqualified
		}
		handles = append(handles, handle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(handles) > 2500 {
		return nil, ErrUnqualified
	}
	return handles, nil
}
