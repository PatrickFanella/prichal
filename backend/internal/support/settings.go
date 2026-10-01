// Package support provides explicitly authorized, audited management inspection.
package support

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// Identity binds operator rights to a verified issuer/subject, never an email or display name.
type Identity struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

// Settings enables only explicit local qualification. Live operator policy remains unconfigured.
type Settings struct{ Identities []Identity }

// LoadSettings reads private local qualification identities. Empty configuration disables access.
func LoadSettings(local bool, issuer string) (Settings, error) {
	path := os.Getenv("PDS_LOCAL_OPERATOR_FILE")
	if path == "" {
		return Settings{}, nil
	}
	if !local {
		return Settings{}, errors.New("local operator policy requires local mode")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return Settings{}, errors.New("private local operator policy unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, errors.New("local operator policy unavailable")
	}
	var identities []Identity
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&identities) != nil || decoder.Decode(new(any)) != io.EOF || len(identities) == 0 || len(identities) > 100 {
		return Settings{}, errors.New("invalid local operator policy")
	}
	seen := map[string]bool{}
	for _, identity := range identities {
		key := identity.Issuer + "\x00" + identity.Subject
		if identity.Issuer != issuer || identity.Subject == "" || len(identity.Subject) > 255 || seen[key] || (identity.Role != "inspect" && identity.Role != "operate") {
			return Settings{}, errors.New("invalid local operator identity or role")
		}
		seen[key] = true
	}
	return Settings{Identities: identities}, nil
}
