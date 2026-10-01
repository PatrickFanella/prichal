package billing

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

// Settings enables only disposable local fixture delivery. Paid/live adapters remain closed.
type Settings struct {
	Enabled                                   bool
	WebhookSecret, FixtureURL, FixtureKeyFile string
}

func privateKey(path, prefix string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 512 {
		return "", errors.New("private billing key unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("private billing key unavailable")
	}
	key := strings.TrimSpace(string(data))
	if !strings.HasPrefix(key, prefix) || len(key) < len(prefix)+32 || len(key) > 256 || strings.ContainsAny(key, "\r\n\t ") {
		return "", errors.New("private billing key invalid")
	}
	return key, nil
}

func fixtureOrigin(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "http" && u.User == nil && u.Path == "" && u.RawPath == "" && u.RawQuery == "" && u.Fragment == "" && net.ParseIP(u.Hostname()).IsLoopback()
}

// LoadSettings uses explicit local mode and loopback fixture authority; no live billing fallback.
func LoadSettings(local bool) (Settings, error) {
	mode := os.Getenv("PDS_BILLING_MODE")
	if mode == "" || mode == "disabled" {
		return Settings{}, nil
	}
	if mode != "fixture" || !local {
		return Settings{}, errors.New("billing fixture requires explicit local mode")
	}
	origin := os.Getenv("PDS_BILLING_FIXTURE_URL")
	if !fixtureOrigin(origin) {
		return Settings{}, errors.New("billing fixture authority must be a loopback HTTP origin")
	}
	secret, err := privateKey(os.Getenv("PDS_BILLING_WEBHOOK_KEY_FILE"), "whsec_")
	if err != nil {
		return Settings{}, err
	}
	path := os.Getenv("PDS_BILLING_FIXTURE_KEY_FILE")
	if path == "" {
		return Settings{}, errors.New("billing fixture key file required")
	}
	return Settings{Enabled: true, WebhookSecret: secret, FixtureURL: origin, FixtureKeyFile: path}, nil
}
