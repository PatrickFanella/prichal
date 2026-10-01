package endpointplan

import (
	"context"
	"errors"
	"slices"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

var ErrProofUnconfirmed = errors.New("current endpoint proof is not confirmed")

// Claim reserves only a hosting hostname. It is never routing, TLS or activation authority.
type Claim struct {
	ChoiceID    string    `json:"choiceId"`
	Hostname    string    `json:"hostname"`
	State       string    `json:"state"`
	Reserved    bool      `json:"reserved"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CanActivate bool      `json:"canActivate"`
}

func claimRow(ctx context.Context, tx pgx.Tx, id string) (Claim, error) {
	var c Claim
	var sameProof bool
	err := tx.QueryRow(ctx, `SELECT choice_id,hostname,CASE WHEN state='reserved' AND expires_at<=statement_timestamp() THEN 'expired' ELSE state END,state='reserved' AND expires_at>statement_timestamp(),expires_at,proof_sha256=sha256(convert_to((SELECT proof_value FROM endpoint_choices WHERE id=choice_id),'UTF8')) FROM endpoint_claims WHERE choice_id=$1`, id).Scan(&c.ChoiceID, &c.Hostname, &c.State, &c.Reserved, &c.ExpiresAt, &sameProof)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err == nil && !sameProof {
		return Claim{}, ErrConflict
	}
	return c, err
}

func (s *Store) GetClaim(ctx context.Context, actor, org, id string) (Claim, error) {
	if !uuid.MatchString(id) {
		return Claim{}, ErrNotFound
	}
	tx, err := s.beginChoice(ctx, actor, org)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.id=$1 AND c.organization_id=$2 AND c.actor_id=$3 FOR SHARE`, id, org, actor)); err != nil {
		return Claim{}, err
	}
	c, err := claimRow(ctx, tx, id)
	if err != nil {
		return Claim{}, err
	}
	return c, tx.Commit(ctx)
}

// ReserveClaim reuses an exact retained receipt without a new DNS query. New
// reservations require bounded DNS observation and a locked post-lookup check.
func (s *Store) ReserveClaim(ctx context.Context, actor, org, id string, dns TXTResolver) (Claim, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if !uuid.MatchString(id) {
		return Claim{}, ErrNotFound
	}
	tx, err := s.beginChoice(ctx, actor, org)
	if err != nil {
		return Claim{}, err
	}
	c, err := scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.id=$1 AND c.organization_id=$2 AND c.actor_id=$3 FOR SHARE`, id, org, actor))
	if err != nil {
		_ = tx.Rollback(ctx)
		return Claim{}, err
	}
	prior, priorErr := claimRow(ctx, tx, id)
	if priorErr == nil {
		return prior, tx.Commit(ctx)
	}
	_ = tx.Rollback(ctx)
	if !errors.Is(priorErr, ErrNotFound) {
		return Claim{}, priorErr
	}
	observation, err := s.Observe(ctx, actor, org, id, dns)
	if err != nil {
		return Claim{}, err
	}
	if !observation.Matched || observation.State != "challenge_match" {
		return Claim{}, ErrProofUnconfirmed
	}
	tx, err = s.beginChoice(ctx, actor, org)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx)
	current, err := scanChoice(tx.QueryRow(ctx, `SELECT `+choiceColumns+` FROM endpoint_choices c WHERE c.id=$1 AND c.organization_id=$2 AND c.actor_id=$3 FOR UPDATE`, id, org, actor))
	if err != nil {
		return Claim{}, err
	}
	if current.Hostname != c.Hostname || current.ProofValue != c.ProofValue || !slices.Equal(current.ApplicationHosts, c.ApplicationHosts) {
		return Claim{}, ErrConflict
	}
	// All claimants of a hostname serialize, including expiry reclamation.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,73647302))`, c.Hostname); err != nil {
		return Claim{}, err
	}
	var held bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return Claim{}, err
	}
	if held {
		return Claim{}, recovery.ErrHeld
	}
	if prior, err = claimRow(ctx, tx, id); err == nil {
		return prior, tx.Commit(ctx)
	} else if !errors.Is(err, ErrNotFound) {
		return Claim{}, err
	}
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT $2::timestamptz<=clock_timestamp() OR EXISTS(SELECT 1 FROM deployments WHERE organization_id=$1 AND kind='agent') OR EXISTS(SELECT 1 FROM organization_transfers WHERE organization_id=$1 AND state='pending' AND expires_at>clock_timestamp())`, org, current.ExpiresAt).Scan(&blocked); err != nil {
		return Claim{}, err
	}
	if blocked || current.State != "awaiting_ownership" {
		return Claim{}, ErrConflict
	}
	// Expiry is a system event. No foreign owner or claim identifier is exposed.
	if _, err = tx.Exec(ctx, `WITH expired AS (UPDATE endpoint_claims SET state='expired' WHERE hostname=$1 AND state='reserved' AND expires_at<=clock_timestamp() RETURNING id) INSERT INTO endpoint_claim_events(claim_id,action) SELECT id,'expire' FROM expired`, c.Hostname); err != nil {
		return Claim{}, err
	}
	var occupied bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM endpoint_claims WHERE hostname=$1 AND state='reserved')`, c.Hostname).Scan(&occupied); err != nil {
		return Claim{}, err
	}
	if occupied {
		return Claim{}, ErrConflict
	}
	var claimID string
	if err = tx.QueryRow(ctx, `INSERT INTO endpoint_claims(choice_id,organization_id,actor_id,hostname,expires_at,proof_sha256,verified_at) VALUES($1,$2,$3,$4,$5,sha256(convert_to($6,'UTF8')),$7) RETURNING id`, id, org, actor, c.Hostname, current.ExpiresAt, current.ProofValue, observation.CheckedAt).Scan(&claimID); err != nil {
		return Claim{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO endpoint_claim_events(claim_id,actor_id,action) VALUES($1,$2,'reserve')`, claimID, actor); err != nil {
		return Claim{}, err
	}
	result, err := claimRow(ctx, tx, id)
	if err != nil {
		return Claim{}, err
	}
	return result, tx.Commit(ctx)
}
