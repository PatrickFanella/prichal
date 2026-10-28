package config

import (
	"errors"
	"net"
	"os"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

type Config struct {
	StagingProvisioning bool
	DatabaseURL         string
	ListenAddress       string
	Auth                auth.Settings
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("PDS_DATABASE_URL"), ListenAddress: os.Getenv("PDS_LISTEN_ADDRESS")}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("PDS_DATABASE_URL is required")
	}
	if c.ListenAddress == "" {
		c.ListenAddress = "127.0.0.1:8080"
	}
	if _, _, err := net.SplitHostPort(c.ListenAddress); err != nil {
		return Config{}, errors.New("PDS_LISTEN_ADDRESS must be host:port")
	}
	c.Auth = auth.Settings{Mode: os.Getenv("PDS_MODE"), Issuer: os.Getenv("PDS_OIDC_ISSUER"), ClientID: os.Getenv("PDS_OIDC_CLIENT_ID"), ClientSecret: os.Getenv("PDS_OIDC_CLIENT_SECRET"), Origin: os.Getenv("PDS_PUBLIC_ORIGIN")}
	if c.Auth.Mode == "" {
		c.Auth.Mode = "production"
	}
	var err error
	c.StagingProvisioning, err = LoadStagingProvisioning(os.Getenv("PDS_MODE"))
	if err != nil {
		return Config{}, err
	}
	if err := c.Auth.Validate(); err != nil {
		return Config{}, err
	}
	if c.Auth.Mode == "local" {
		host, _, _ := net.SplitHostPort(c.ListenAddress)
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return Config{}, errors.New("local API listener must bind to loopback")
		}
	}

	return c, nil
}

// LoadStagingProvisioning enables only the private provisioning slice with production authentication.
func LoadStagingProvisioning(mode string) (bool, error) {
	value := os.Getenv("PDS_STAGING_PROVISIONING")
	if value == "" {
		return false, nil
	}
	if value != "true" || mode != "production" {
		return false, errors.New("PDS_STAGING_PROVISIONING requires explicit production mode and true opt-in")
	}
	return true, nil
}
