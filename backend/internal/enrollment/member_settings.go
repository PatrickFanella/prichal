package enrollment

import (
	"errors"
	"os"
)

// LoadMemberAdministration requires an independent opt-in because native account
// admission exceeds the ordinary collaboration metadata permission.
func LoadMemberAdministration(local, collaboration bool) (bool, error) {
	value := os.Getenv("PDS_LOCAL_MEMBER_ADMINISTRATION")
	if value == "" || value == "false" {
		return false, nil
	}
	if value != "true" || !local || !collaboration {
		return false, errors.New("member administration requires explicit local mode and organization access")
	}
	return true, nil
}
