package support

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var ErrNotFound = errors.New("support reference not found")

// Store projects named management fields only; it has no native credentials or host transport.
type Store struct {
	Pool     *pgxpool.Pool
	Settings Settings
}

func (s *Store) role(ctx context.Context, owner string) (string, error) {
	if len(s.Settings.Identities) == 0 {
		return "", nil
	}
	var issuer, subject string
	if err := s.Pool.QueryRow(ctx, `SELECT issuer,subject FROM owners WHERE id=$1`, owner).Scan(&issuer, &subject); err != nil {
		return "", err
	}
	for _, identity := range s.Settings.Identities {
		if identity.Issuer == issuer && identity.Subject == subject {
			return identity.Role, nil
		}
	}
	return "", nil
}

// Inspect commits an inspection audit before returning an explicit safe tenant projection.
func (s *Store) Inspect(ctx context.Context, owner, role, id string) (json.RawMessage, error) {
	if !uuid.MatchString(id) || (role != "inspect" && role != "operate") {
		return nil, ErrNotFound
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return nil, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var org string
	err = tx.QueryRow(ctx, `SELECT organization_id FROM deployments WHERE id=$1`, id).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var view []byte
	err = tx.QueryRow(ctx, `SELECT jsonb_build_object(
 'instanceId',d.id,'organizationId',d.organization_id,'name',d.name,'kind',d.kind,'region',d.region,'state',d.state,'managedHostname',d.managed_hostname,'endpoint',d.endpoint_url,
 'domainProof',(SELECT jsonb_build_object('handle',h.handle,'state',h.state,'checkedAt',h.checked_at) FROM handle_configurations h WHERE h.deployment_id=d.id),
 'usage',(SELECT jsonb_build_object('dataBytes',u.data_bytes,'blobBytes',u.blob_bytes,'accounts',u.accounts,'measuredAt',u.measured_at) FROM instance_usage u WHERE u.deployment_id=d.id),
 'billing',(SELECT jsonb_build_object('state',b.subscription_state,'source',b.source,'checkedAt',b.checked_at,'paidEntitlement',false) FROM billing_subscription_observations b WHERE b.organization_id=d.organization_id),
 'hostingGrant',(SELECT jsonb_build_object('state',CASE WHEN p.state='active' AND p.expires_at<=clock_timestamp() THEN 'expired' ELSE p.state END,'plan',p.plan,'expiresAt',p.expires_at) FROM pilot_entitlement_operations p WHERE p.organization_id=d.organization_id ORDER BY p.generation DESC LIMIT 1),
 'incidents',COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',i.kind,'active',i.active,'lastObservedAt',i.last_observed_at) ORDER BY i.kind) FROM service_observations i WHERE i.deployment_id=d.id),'[]'::jsonb),
 'managementRecovery',COALESCE((SELECT state FROM management_recovery),'unconfigured'),
 'lifecycle',COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',l.kind,'state',l.state,'desiredState',l.desired_state,'generation',l.generation,'reason',l.reason)) FROM service_operations l WHERE l.deployment_id=d.id AND l.generation=(SELECT max(x.generation) FROM service_operations x WHERE x.deployment_id=d.id AND x.kind=l.kind)),'[]'::jsonb)
 ) FROM deployments d WHERE d.id=$1`, id).Scan(&view)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO support_inspections(actor_owner_id,organization_id,deployment_id,role) VALUES($1,$2,$3,$4)`, owner, org, id, role); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return view, nil
}

// Activity returns bounded lifecycle history after either owner scope or explicit operator authorization.
func (s *Store) Activity(ctx context.Context, owner, id string, operator bool, before int64) (json.RawMessage, error) {
	if !uuid.MatchString(id) || !uuid.MatchString(owner) {
		return nil, ErrNotFound
	}
	if before < 0 || before > (1<<53)-1 {
		return nil, ErrCursor
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND ($3 OR o.owner_id=$2) FOR SHARE OF d,o`, id, owner, operator).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	checkRecovery := func() error {
		var held bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
			return err
		}
		if held {
			return recovery.ErrHeld
		}
		return nil
	}
	if err = checkRecovery(); err != nil {
		return nil, err
	}
	var result []byte
	err = tx.QueryRow(ctx, `WITH page AS (SELECT * FROM service_operations WHERE deployment_id=$1 AND ($2::bigint=0 OR generation<$2) ORDER BY generation DESC LIMIT 51), visible AS (SELECT * FROM page ORDER BY generation DESC LIMIT 50)
 SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',v.id,'kind',v.kind,'generation',v.generation,'state',v.state,'desiredState',v.desired_state,'priorState',v.prior_state,'reason',v.reason,'updatedAt',v.updated_at,'actor',CASE WHEN $3 THEN v.actor ELSE 'operator' END,'events',COALESCE((SELECT jsonb_agg(jsonb_build_object('state',e.state,'createdAt',e.created_at) ORDER BY e.id) FROM service_operation_events e WHERE e.operation_id=v.id),'[]'::jsonb)) ORDER BY v.generation DESC) FROM visible v),'[]'::jsonb),'hasMore',(SELECT count(*)>50 FROM page),'nextBefore',CASE WHEN (SELECT count(*)>50 FROM page) THEN (SELECT min(generation) FROM visible) ELSE NULL END)`, id, before, operator).Scan(&result)
	if err != nil {
		return nil, err
	}
	if err = checkRecovery(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// Organization finds bounded instance references and audits the organization inspection.
func (s *Store) Organization(ctx context.Context, owner, role, id string) (json.RawMessage, error) {
	if !uuid.MatchString(id) || (role != "inspect" && role != "operate") {
		return nil, ErrNotFound
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return nil, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var result []byte
	err = tx.QueryRow(ctx, `WITH page AS (SELECT id,name,state,kind FROM deployments WHERE organization_id=$1 ORDER BY created_at DESC,id LIMIT 101), visible AS (SELECT * FROM page LIMIT 100) SELECT jsonb_build_object('organizationId',$1::text,'items',COALESCE((SELECT jsonb_agg(jsonb_build_object('instanceId',id,'name',name,'state',state,'kind',kind)) FROM visible),'[]'::jsonb),'hasMore',(SELECT count(*)>100 FROM page))`, id).Scan(&result)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO support_inspections(actor_owner_id,organization_id,role) VALUES($1,$2,$3)`, owner, id, role); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
