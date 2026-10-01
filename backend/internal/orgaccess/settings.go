// Package orgaccess rehearses identity-bound management collaboration without native authority.
package orgaccess

import (
	"errors"
	"os"
)

func LoadEnabled(local bool) (bool, error) {
	value := os.Getenv("PDS_LOCAL_ORGANIZATION_ACCESS")
	if value == "" || value == "false" {
		return false, nil
	}
	if value != "true" || !local {
		return false, errors.New("organization access rehearsal requires explicit local mode")
	}
	return true, nil
}
