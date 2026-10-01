// Package endpointplan checks custom-address planning without changing native hosting.
package endpointplan

import (
	"context"
	"errors"
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

var ErrInvalid = errors.New("invalid endpoint planning request")
var ErrDomainSeparation = errors.New("PDS and application domains must be separate")
var ErrNotFound = errors.New("organization not found")
var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var hostnameSyntax = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Request declares a proposed PDS address and all known application/OAuth hosts.
// An explicitly empty list means the owner declares no additional app hosts.
type Request struct {
	Hostname         string   `json:"hostname"`
	ApplicationHosts []string `json:"applicationHosts"`
}

// Result is planning evidence only, never a saved endpoint or activation authority.
type Result struct {
	Hostname         string   `json:"hostname"`
	ApplicationHosts []string `json:"applicationHosts"`
	State            string   `json:"state"`
	CanActivate      bool     `json:"canActivate"`
	ChoiceSaved      bool     `json:"choiceSaved"`
	RemainingGates   []string `json:"remainingGates"`
	Message          string   `json:"message"`
}

// Store reads current organization custody; it has no DNS, HTTP or host executor.
type Store struct {
	Pool           *pgxpool.Pool
	Local          bool
	ManagementHost string
}

// Normalize accepts ASCII DNS names, including validated A-labels. It rejects
// URLs, ports, IP literals, public suffixes, unknown and reserved namespaces.
func Normalize(value string, local bool) (string, error) {
	value = strings.ToLower(value)
	if len(value) > 253 || !hostnameSyntax.MatchString(value) || net.ParseIP(value) != nil {
		return "", ErrInvalid
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil || ascii != value {
		return "", ErrInvalid
	}
	labels := strings.Split(value, ".")
	switch labels[len(labels)-1] {
	case "alt", "arpa", "example", "internal", "invalid", "local", "localhost", "onion":
		return "", ErrInvalid
	case "test":
		if !local {
			return "", ErrInvalid
		}
	default:
		suffix, icann := publicsuffix.PublicSuffix(value)
		if !icann && !strings.Contains(suffix, ".") {
			return "", ErrInvalid
		}
	}
	if _, err = publicsuffix.EffectiveTLDPlusOne(value); err != nil {
		return "", ErrInvalid
	}
	return value, nil
}

// Validate uses the pinned public-suffix snapshot instead of guessing the final
// two labels. Sibling app/PDS subdomains under the same registrable site fail.
func Validate(local bool, managementHost string, request Request) (Result, error) {
	result := Result{ApplicationHosts: []string{}, RemainingGates: []string{"domain_ownership", "routing_dns", "certificate", "persisted_choice", "supported_activation"}}
	if request.ApplicationHosts == nil || len(request.ApplicationHosts) > 8 {
		return result, ErrInvalid
	}
	host, err := Normalize(request.Hostname, local)
	if err != nil {
		return result, err
	}
	site, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return result, ErrInvalid
	}
	protected := append([]string{}, request.ApplicationHosts...)
	// Local management origins have no registrable public site and cannot pass
	// hostname validation as a proposed PDS. Public origins always participate.
	if !(local && (managementHost == "localhost" || net.ParseIP(managementHost).IsLoopback())) {
		protected = append(protected, managementHost)
	}
	for i, application := range protected {
		application, err = Normalize(application, local)
		if err != nil {
			return result, ErrInvalid
		}
		appSite, err := publicsuffix.EffectiveTLDPlusOne(application)
		if err != nil {
			return result, ErrInvalid
		}
		if appSite == site {
			return result, ErrDomainSeparation
		}
		if i < len(request.ApplicationHosts) {
			result.ApplicationHosts = append(result.ApplicationHosts, application)
		}
	}
	slices.Sort(result.ApplicationHosts)
	result.ApplicationHosts = slices.Compact(result.ApplicationHosts)
	result.Hostname = host
	result.State = "planning_only"
	result.Message = "Hostname and declared application separation passed. Ownership, routing, TLS and a saved endpoint choice are still required before activation."
	return result, nil
}

// Preflight checks current ownership and retained native hosting in one read-only query.
func (s *Store) Preflight(ctx context.Context, actor, org string, request Request) (Result, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(org) {
		return Result{}, ErrNotFound
	}
	var existing bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments d WHERE d.organization_id=o.id AND d.kind='agent') FROM organizations o WHERE o.id=$1 AND o.owner_id=$2`, org, actor).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	result, err := Validate(s.Local, s.ManagementHost, request)
	if err != nil {
		return result, err
	}
	if existing {
		result.State = "existing_native_requires_review"
		result.RemainingGates = append([]string{"existing_native_review"}, result.RemainingGates...)
		result.Message = "This organization already has native hosting. Keep its current endpoint; changing it requires a separately reviewed migration or replacement plan."
	}
	return result, nil
}
