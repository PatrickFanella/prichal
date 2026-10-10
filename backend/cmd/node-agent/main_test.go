package main

import (
	"strings"
	"testing"
)

func TestStartupRefusesAmbiguousPilotConfiguration(t *testing.T) {
	for _, entry := range []struct {
		name    string
		env     map[string]string
		message string
	}{
		{"production is not pilot", map[string]string{"PDS_MODE": "production"}, "explicit local or pilot"},
		{"local policy", map[string]string{"PDS_MODE": "local", "PDS_AGENT_PILOT_POLICY_FILE": "/private/policy.json"}, "explicit pilot"},
		{"fixture mixing", map[string]string{"PDS_MODE": "pilot", "PDS_AGENT_LOCAL_PROTOCOL_FIXTURES": "false"}, "local fixtures or capacity"},
		{"capacity override", map[string]string{"PDS_MODE": "pilot", "PDS_AGENT_MAXIMUM_SLOTS": "2"}, "local fixtures or capacity"},
		{"local enrollment opt-in", map[string]string{"PDS_MODE": "local", "PDS_AGENT_PILOT_ENROLLMENT": "true"}, "true opt-in"},
		{"invalid pilot opt-in", map[string]string{"PDS_MODE": "pilot", "PDS_AGENT_PILOT_ENROLLMENT": "false"}, "true opt-in"},
		{"LAN agent", map[string]string{"PDS_MODE": "pilot", "PDS_AGENT_LISTEN": "0.0.0.0:9091"}, "loopback"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			for _, key := range []string{"PDS_MODE", "PDS_AGENT_PILOT_POLICY_FILE", "PDS_AGENT_LOCAL_PROTOCOL_FIXTURES", "PDS_AGENT_MAXIMUM_SLOTS", "PDS_AGENT_MEMBER_CEILING", "PDS_AGENT_LISTEN", "PDS_AGENT_PILOT_ENROLLMENT"} {
				t.Setenv(key, "")
			}
			for key, value := range entry.env {
				t.Setenv(key, value)
			}
			if err := run(); err == nil || !strings.Contains(err.Error(), entry.message) {
				t.Fatalf("startup refusal: %v", err)
			}
		})
	}
}
