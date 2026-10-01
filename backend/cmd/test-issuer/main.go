package main

import (
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
	"log"
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
	// The fixture command deliberately has a fixed loopback address and issuer.
	if settings.Issuer != "http://localhost:5556" {
		return errors.New("test issuer command requires http://localhost:5556")
	}
	issuer, err := testissuer.New(settings.Issuer, settings.ClientID, settings.Origin+"/api/auth/callback")
	if err != nil {
		return err
	}
	server := &http.Server{Addr: "127.0.0.1:5556", Handler: issuer.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	log.Print("isolated test issuer listening on 127.0.0.1:5556")
	return server.ListenAndServe()
}
