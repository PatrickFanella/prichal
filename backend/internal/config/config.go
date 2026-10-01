package config

import (
	"errors"
	"net"
	"os"
)

type Config struct {
	DatabaseURL   string
	ListenAddress string
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
	return c, nil
}
