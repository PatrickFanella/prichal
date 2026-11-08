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

// HandleLabelLength is the PDS limit for a service-domain handle's first label.
const HandleLabelLength = 18

// Handle uses the retained account invitation identity, separate from the PDS deployment identity.
// The label is "at-" plus the first 15 hex characters (60 bits) of the invitation UUID, because the
// official PDS rejects service-domain handles longer than 18 characters (#245). Bindings are unique.
func Handle(invitationID string) (string, error) {
	if !uuid.MatchString(invitationID) {
		return "", errors.New("managed names require a canonical UUIDv4")
	}
	label := "at-" + strings.ReplaceAll(invitationID, "-", "")[:HandleLabelLength-3]
	return label + "." + Domain, nil
}

func name(prefix, id string) (string, error) {
	if !uuid.MatchString(id) {
		return "", errors.New("managed names require a canonical UUIDv4")
	}
	return prefix + strings.ReplaceAll(id, "-", "") + "." + Domain, nil
}
