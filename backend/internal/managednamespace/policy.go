// Package managednamespace fixes the owner-selected public namespace independently of node placement.
package managednamespace

import (
	"errors"
	"regexp"
	"strings"
)

const Domain = "subcult.tv"

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

// Endpoint allocates only generated first-level labels; customer names never select host routing.
func Endpoint(deploymentID string) (string, error) { return name("pds-", deploymentID) }

// Handle uses the retained account invitation identity, separate from the PDS deployment identity.
func Handle(invitationID string) (string, error) { return name("at-", invitationID) }

func name(prefix, id string) (string, error) {
	if !uuid.MatchString(id) {
		return "", errors.New("managed names require a canonical UUIDv4")
	}
	return prefix + strings.ReplaceAll(id, "-", "") + "." + Domain, nil
}
