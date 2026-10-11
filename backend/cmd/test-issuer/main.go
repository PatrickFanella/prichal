package main

import (
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	settings := auth.Settings{Mode: os.Getenv("PDS_MODE"), Issuer: os.Getenv("PDS_OIDC_ISSUER"), Origin: os.Getenv("PDS_PUBLIC_ORIGIN"), ClientID: os.Getenv("PDS_OIDC_CLIENT_ID")}
	if settings.Mode != "local" {
		return errors.New("test issuer requires explicit PDS_MODE=local")
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	address := os.Getenv("PDS_TEST_ISSUER_LISTEN")
	if address == "" {
		address = "127.0.0.1:5556"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || settings.Issuer != "http://localhost:"+port {
		return errors.New("test issuer requires a matching loopback listener and localhost issuer")
	}
	issuer, err := testissuer.New(settings.Issuer, settings.ClientID, settings.Origin+"/api/auth/callback")
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: issuer.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("isolated test issuer listening on %s", address)
	return server.ListenAndServe()
}
