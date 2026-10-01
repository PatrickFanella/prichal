package auth

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

type Settings struct{ Issuer, ClientID, ClientSecret, Origin, Mode string }

func (s Settings) Validate() error {
	if s.ClientID == "" || len(s.ClientID) > 255 {
		return errors.New("PDS_OIDC_CLIENT_ID is required and must fit 255 characters")
	}
	if s.Mode != "production" && s.Mode != "local" {
		return errors.New("PDS_MODE must be production or local")
	}
	for _, entry := range []struct {
		value, name string
		origin      bool
	}{{s.Origin, "PDS_PUBLIC_ORIGIN", true}, {s.Issuer, "PDS_OIDC_ISSUER", false}} {
		u, err := url.Parse(entry.value)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(entry.value, "#") || entry.origin && strings.HasSuffix(entry.value, "/") || len(entry.value) > 2048 {
			return errors.New(entry.name + " must be an absolute URL without credentials, query, fragment or trailing slash")
		}
		if entry.origin && u.Path != "" {
			return errors.New("PDS_PUBLIC_ORIGIN must have no path")
		}
		if s.Mode == "production" && u.Scheme != "https" {
			return errors.New(entry.name + " requires HTTPS in production")
		}
		if s.Mode == "local" && (u.Scheme != "http" || !loopback(u.Hostname())) {
			return errors.New("local authentication requires HTTP loopback issuer and origin")
		}
	}
	return nil
}
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func (s Settings) callback() string { return s.Origin + "/api/auth/callback" }
func (s Settings) cookie(suffix string) string {
	if s.Mode == "production" {
		return "__Host-pds-" + suffix
	}
	return "pds-local-" + suffix
}
func (s Settings) endpointAllowed(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	if s.Mode == "local" {
		return u.Scheme == "http" && loopback(u.Hostname())
	}
	return u.Scheme == "https"
}
