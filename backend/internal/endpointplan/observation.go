package endpointplan

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net"
	"slices"
	"strings"
	"time"
)

// TXTResolver reads DNS records without customer URLs or customer-selected servers.
type TXTResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}

// Observation is a transient DNS answer. It grants no verified claim or deployment authority.
type Observation struct {
	ChoiceID      string    `json:"choiceId"`
	Hostname      string    `json:"hostname"`
	State         string    `json:"state"`
	Matched       bool      `json:"matched"`
	ClaimReserved bool      `json:"claimReserved"`
	CanActivate   bool      `json:"canActivate"`
	CheckedAt     time.Time `json:"checkedAt"`
	Message       string    `json:"message"`
}

type observationInput struct {
	Choice
	expired bool
	now     time.Time
}

func (s *Store) observationInput(ctx context.Context, actor, org, id string) (observationInput, error) {
	var c observationInput
	if !s.Local || !uuid.MatchString(actor) || !uuid.MatchString(org) || !uuid.MatchString(id) {
		return c, ErrNotFound
	}
	// Keep ownership and recovery inspection in the same short scoped transaction.
	// It ends before DNS; the post-lookup read repeats both checks.
	tx, err := s.beginChoice(ctx, actor, org)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT `+choiceColumns+`,c.expires_at<=statement_timestamp(),statement_timestamp() FROM endpoint_choices c WHERE c.id=$1 AND c.organization_id=$2 AND c.actor_id=$3 FOR SHARE`, id, org, actor).Scan(&c.ID, &c.OrganizationID, &c.Hostname, &c.ApplicationHosts, &c.State, &c.ProofValue, &c.CreatedAt, &c.ExpiresAt, &c.expired, &c.now)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, err
	}
	c.ProofName = "_pdsaas." + c.Hostname
	return c, nil
}
func terminalObservation(c observationInput) (string, string) {
	if c.State == "cancelled" {
		return "cancelled", "This choice is cancelled. Its retained proof is not current hosting authority."
	}
	if c.expired {
		return "expired", "This proof has expired. Review cancellation and prepare a new choice; default native hosting remains paused."
	}
	return "", ""
}

// Observe reads only the scoped saved challenge. Results are never persisted as a
// claim. Scope, cancellation and database-clock expiry are checked after DNS too.
func (s *Store) Observe(ctx context.Context, actor, org, id string, dns TXTResolver) (Observation, error) {
	c, err := s.observationInput(ctx, actor, org, id)
	if err != nil {
		return Observation{}, err
	}
	result := Observation{ChoiceID: c.ID, Hostname: c.Hostname, CheckedAt: c.now}
	if state, message := terminalObservation(c); state != "" {
		result.State = state
		result.Message = message
		return result, nil
	}
	normalized, err := Validate(s.Local, s.ManagementHost, Request{c.Hostname, c.ApplicationHosts})
	if err != nil || normalized.Hostname != c.Hostname || len(c.Hostname) > 244 {
		return Observation{}, ErrInvalid
	}
	if strings.HasSuffix(c.Hostname, ".test") {
		result.State = "local_namespace"
		result.Message = "A local test namespace cannot establish public domain ownership. No DNS query was sent."
		return result, nil
	}
	result.State = "unavailable"
	result.Message = "DNS could not be checked. Retry later; no domain claim or activation was recorded."
	if dns == nil {
		return result, nil
	}
	lookup, cancel := context.WithTimeout(ctx, 4*time.Second)
	records, lookupErr := dns.LookupTXT(lookup, c.ProofName)
	timedOut := lookup.Err() != nil
	cancel()
	if !timedOut && lookupErr == nil {
		bytes := 0
		for _, r := range records {
			bytes += len(r)
		}
		switch {
		case len(records) > 16 || bytes > 4096:
			result.State = "answer_too_large"
			result.Message = "The TXT answer exceeds the supported record bound. No claim or activation was recorded."
		case slices.Contains(records, c.ProofValue):
			result.State = "challenge_match"
			result.Matched = true
			result.Message = "DNS returned this choice's exact unexpired TXT challenge. A verified exclusive claim, routing, TLS and activation still require separate qualification."
		default:
			result.State = "pending_dns"
			result.Message = "This choice's exact TXT challenge was not found. Publish it on the domain you control and retry after propagation."
		}
	} else if !timedOut {
		var e *net.DNSError
		if errors.As(lookupErr, &e) && e.IsNotFound {
			result.State = "pending_dns"
			result.Message = "This choice's exact TXT challenge was not found. Publish it on the domain you control and retry after propagation."
		}
	}
	current, err := s.observationInput(ctx, actor, org, id)
	if err != nil {
		return Observation{}, err
	}
	result.CheckedAt = current.now
	if current.Hostname != c.Hostname || current.ProofValue != c.ProofValue || !slices.Equal(current.ApplicationHosts, c.ApplicationHosts) {
		return Observation{}, ErrConflict
	}
	if state, message := terminalObservation(current); state != "" {
		result.State = state
		result.Message = message
		result.Matched = false
	}
	return result, nil
}
