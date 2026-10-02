package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	// Public production placement is intentionally unavailable until node registration and policy exist.
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("node qualification requires explicit local mode")
	}
	address := os.Getenv("PDS_AGENT_LISTEN")
	if address == "" {
		address = "127.0.0.1:9091"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("local agent must listen on a loopback IP")
	}
	caPath := os.Getenv("PDS_AGENT_CA_FILE")
	certPath := os.Getenv("PDS_AGENT_CERT_FILE")
	keyPath := os.Getenv("PDS_AGENT_KEY_FILE")
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return errors.New("agent CA file is required")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid agent CA")
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return errors.New("agent certificate and private key are required")
	}
	runtime, err := nodeagent.NewDocker(os.Getenv("PDS_AGENT_DOCKER_SOCKET"), os.Getenv("PDS_AGENT_STATE_DIRECTORY"))
	if err != nil {
		return err
	}
	if value := os.Getenv("PDS_AGENT_MAXIMUM_SLOTS"); value != "" {
		ceiling, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			return errors.New("invalid agent capacity ceiling")
		}
		if err = runtime.SetCapacityCeiling(ceiling); err != nil {
			return err
		}
	}
	if value := os.Getenv("PDS_AGENT_MEMBER_CEILING"); value != "" {
		ceiling, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			return errors.New("invalid agent member ceiling")
		}
		if err = runtime.SetMemberCeiling(ceiling); err != nil {
			return err
		}
	}
	if os.Getenv("PDS_AGENT_LOCAL_PROTOCOL_FIXTURES") == "true" {
		runtime.EnableLocalIdentityFixtures()
	}
	server := &http.Server{Addr: address, Handler: nodeagent.Authenticate(nodeagent.Handler(runtime)), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{certificate}}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(os.Stderr, "agent TLS: ", 0)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		log.Printf("local node agent listening on %s with mutual TLS", address)
		done <- server.ListenAndServeTLS("", "")
	}()
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
