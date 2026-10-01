package endpointplan

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

// ErrConflict means a changed request, retained native host or pending choice prevents selection.
var ErrConflict = errors.New("endpoint choice conflicts with retained intent")
var choiceKey = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// ChoiceRequest records explicit review under a stable owner request key.
type ChoiceRequest struct {
	Hostname         string   `json:"hostname"`
	ApplicationHosts []string `json:"applicationHosts"`
	Key              string   `json:"idempotencyKey"`
	Confirmed        bool     `json:"confirmed"`
}

// Choice is an unverified owner intent. Its TXT instruction is not activation authority.
type Choice struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	Hostname         string    `json:"hostname"`
	ApplicationHosts []string  `json:"applicationHosts"`
	State            string    `json:"state"`
	ProofName        string    `json:"proofName"`
	ProofValue       string    `json:"proofValue"`
	CreatedAt        time.Time `json:"createdAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
	CanActivate      bool      `json:"canActivate"`
}

const choiceColumns = `c.id,c.organization_id,c.hostname,c.application_hosts,c.state,c.proof_value,c.created_at,c.expires_at`

func scanChoice(row pgx.Row) (Choice, error) {
	var c Choice
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Hostname, &c.ApplicationHosts, &c.State, &c.ProofValue, &c.CreatedAt, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	c.ProofName = "_pdsaas." + c.Hostname
	return c, err
}

func (s *Store) beginChoice(ctx context.Context, actor, org string) (pgx.Tx, error) {
	if !s.Local || !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return nil, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2 FOR UPDATE`, org, actor).Scan(&id)
	if err == nil {
		var held bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held)
		if err == nil && held {
			err = recovery.ErrHeld
		}
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return tx, nil
}

// SaveChoice serializes with deployment and handover under the organization lock.
// Identical retries retain their proof and state; new choices cannot alter a native endpoint.
func (s *Store) SaveChoice(ctx context.Context, actor, org string, r ChoiceRequest) (Choice, error) {
	if !r.Confirmed || !choiceKey.MatchString(r.Key) {
		return Choice{}, ErrInvalid
	}
	tx, err := s.beginChoice(ctx, actor, org)
	if err != nil {
		return Choice{}, err
	}
	defer tx.Rollback(ctx)
	result, err := Validate(s.Local, s.ManagementHost, Request{r.Hostname, r.ApplicationHosts})
	if err != nil {
		return Choice{}, err
	}
	// Leave room for the TXT label within the DNS wire-name limit.
	if len(result.Hostname) > 244 {
		return Choice{}, ErrInvalid
	}
	prior, err := scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.organization_id=$1 AND c.actor_id=$2 AND c.idempotency_key=$3`, org, actor, r.Key))
	if err == nil {
		if prior.Hostname != result.Hostname || !slices.Equal(prior.ApplicationHosts, result.ApplicationHosts) {
			return Choice{}, ErrConflict
		}
		return prior, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Choice{}, err
	}
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE organization_id=$1 AND kind='agent') OR EXISTS(SELECT 1 FROM endpoint_choices WHERE organization_id=$1 AND state='awaiting_ownership') OR EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM endpoint_choices WHERE organization_id=$1 AND idempotency_key=$2)`, org, r.Key).Scan(&blocked)
	if err != nil {
		return Choice{}, err
	}
	if blocked {
		return Choice{}, ErrConflict
	}
	// Bound retained requests without deleting receipts or expired choices.
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM endpoint_choices WHERE organization_id=$1`, org).Scan(&count); err != nil {
		return Choice{}, err
	}
	if count >= 30 {
		return Choice{}, ErrConflict
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Choice{}, err
	}
	proof := "pdsaas-domain-v1=" + base64.RawURLEncoding.EncodeToString(token)
	c, err := scanChoice(tx.QueryRow(ctx, `INSERT INTO endpoint_choices AS c(organization_id,actor_id,idempotency_key,hostname,application_hosts,proof_value) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+choiceColumns, org, actor, r.Key, result.Hostname, result.ApplicationHosts, proof))
	if err != nil {
		return Choice{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO endpoint_choice_events(choice_id,actor_id,action) VALUES($1,$2,'save')`, c.ID, actor); err != nil {
		return Choice{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Choice{}, err
	}
	return c, nil
}

// GetChoice rechecks both current ownership and original request custody.
func (s *Store) GetChoice(ctx context.Context, actor, org, id string) (Choice, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(org) || !uuid.MatchString(id) {
		return Choice{}, ErrNotFound
	}
	return scanChoice(s.Pool.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c JOIN organizations o ON o.id=c.organization_id WHERE c.id=$1 AND o.id=$2 AND o.owner_id=$3 AND c.actor_id=$3`, id, org, actor))
}

// CurrentChoice returns only the current owner's pending selection, including expired proof.
// Expiry alone does not silently opt the organization into default hosting.
func (s *Store) CurrentChoice(ctx context.Context, actor, org string) (*Choice, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return nil, ErrNotFound
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE id=$1 AND owner_id=$2`, org, actor).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c, err := scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.organization_id=$1 AND c.actor_id=$2 AND c.state='awaiting_ownership'`, org, actor))
	if errors.Is(err, ErrNotFound) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &c, tx.Commit(ctx)
}

// CancelChoice retains the receipt and proof, and records one effective cancellation audit.
// It queues no deployment; opting back into default hosting requires a new deployment request.
func (s *Store) CancelChoice(ctx context.Context, actor, org, id, key string, confirmed bool) (Choice, error) {
	if !uuid.MatchString(id) || !choiceKey.MatchString(key) || !confirmed {
		return Choice{}, ErrInvalid
	}
	tx, err := s.beginChoice(ctx, actor, org)
	if err != nil {
		return Choice{}, err
	}
	defer tx.Rollback(ctx)
	c, err := scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.id=$1 AND c.organization_id=$2 AND c.actor_id=$3 FOR UPDATE`, id, org, actor))
	if err != nil {
		return Choice{}, err
	}
	if c.State == "cancelled" {
		var prior string
		if err = tx.QueryRow(ctx, `SELECT cancellation_key FROM endpoint_choices WHERE id=$1`, id).Scan(&prior); err != nil {
			return Choice{}, err
		}
		if prior != key {
			return Choice{}, ErrConflict
		}
		return c, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE endpoint_choices SET state='cancelled',cancelled_at=clock_timestamp(),cancellation_key=$2 WHERE id=$1`, id, key); err != nil {
		return Choice{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO endpoint_choice_events(choice_id,actor_id,action) VALUES($1,$2,'cancel')`, id, actor); err != nil {
		return Choice{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Choice{}, err
	}
	c.State = "cancelled"
	return c, nil
}
