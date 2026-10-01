// Package hostingexit retains local owner exit intent independently of payment and native execution.
package hostingexit

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type Policy struct {
	Version       int `json:"version"`
	DelaySeconds  int `json:"delaySeconds"`
	RetentionDays int `json:"retentionDays"`
}

func (p Policy) valid() bool {
	return p.Version == 1 && p.DelaySeconds >= 30 && p.DelaySeconds <= 604800 && p.RetentionDays >= 1 && p.RetentionDays <= 30
}
func LoadPolicy(local bool) (*Policy, error) {
	path := os.Getenv("PDS_LOCAL_EXIT_POLICY_FILE")
	if path == "" {
		return nil, nil
	}
	if !local {
		return nil, errors.New("hosting exit qualification requires local mode")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1024 {
		return nil, errors.New("private local exit policy required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var policy Policy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF || !policy.valid() {
		return nil, errors.New("invalid bounded local exit policy")
	}
	return &policy, nil
}
