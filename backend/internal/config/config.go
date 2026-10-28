package config

import (
	"errors"
	"net"
	"os"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
)

type Config struct {
	StagingProvisioning   bool
	StagingManagedIntents bool
	DatabaseURL           string
	ListenAddress         string
	Auth                  auth.Settings
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
	c.StagingManagedIntents, err = LoadStagingManagedIntents(os.Getenv("PDS_MODE"), c.StagingProvisioning)
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

// LoadStagingManagedIntents permits preparation only; runtime/public activation remains closed.
func LoadStagingManagedIntents(mode string, staging bool) (bool, error) {
	value := os.Getenv("PDS_STAGING_MANAGED_INTENTS")
	if value == "" {
		return false, nil
	}
	if value != "true" || mode != "production" || !staging {
		return false, errors.New("PDS_STAGING_MANAGED_INTENTS requires explicit production staging provisioning and true opt-in")
	}
	return true, nil
}

// LoadStagingManagedRuntime enables native identity only; it never enables public readiness.
func LoadStagingManagedRuntime(mode string, staging bool) (bool, error) {
	value := os.Getenv("PDS_STAGING_MANAGED_RUNTIME")
	if value == "" {
		return false, nil
	}
	if value != "true" || mode != "production" || !staging {
		return false, errors.New("PDS_STAGING_MANAGED_RUNTIME requires explicit production staging provisioning and true opt-in")
	}
	return true, nil
}
