package main

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/fleet"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("node administration requires explicit local mode")
	}
	if len(os.Args) < 2 {
		return errors.New("usage: node-admin {issue --endpoint URL --slots N --out FILE|capacity --node UUID --slots N --reason REASON|serve}")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	configuration, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || !net.ParseIP(configuration.ConnConfig.Host).IsLoopback() {
		return errors.New("local node administration requires a loopback database")
	}
	pool, err := pgxpool.NewWithConfig(startup, configuration)
	if err != nil {
		return errors.New("management database configuration unavailable")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(startup, `SELECT current_database()`).Scan(&database); err != nil {
		return errors.New("management database unavailable")
	}
	if database != "pds_local" {
		return errors.New("local node administration must target pds_local")
	}
	if err = migrate.Apply(startup, pool); err != nil {
		return err
	}
	store := fleet.Store{Pool: pool}
	switch os.Args[1] {
	case "capacity":
		flags := flag.NewFlagSet("capacity", flag.ContinueOnError)
		node := flags.String("node", "", "registered node UUID")
		slots := flags.Int("slots", 0, "operator capacity ceiling")
		reason := flags.String("reason", "", "fixed operator reason")
		if err = flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected capacity argument")
		}
		actor, lookupErr := user.Current()
		if lookupErr != nil {
			return errors.New("operator identity unavailable")
		}
		if err = store.SetCapacityCeiling(startup, *node, actor.Username, *reason, *slots); err != nil {
			return err
		}
		fmt.Println("Local capacity policy recorded; authenticated measurement required before placement.")
		return nil
	case "issue":
		flags := flag.NewFlagSet("issue", flag.ContinueOnError)
		endpoint := flags.String("endpoint", "https://127.0.0.1:9091", "private local agent endpoint")
		slots := flags.Int("slots", 1, "maximum reserved instances")
		out := flags.String("out", "", "private bootstrap output file")
		if err = flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *out == "" || flags.NArg() != 0 {
			return errors.New("--out FILE is required")
		}
		file, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("bootstrap output must be a new private file")
		}
		defer file.Close()
		bootstrap, err := store.Issue(startup, *endpoint, *slots)
		if err != nil {
			_ = os.Remove(*out)
			return errors.New("node bootstrap could not be issued; inspect duplicate endpoint or database state")
		}
		if err = json.NewEncoder(file).Encode(bootstrap); err != nil {
			return errors.New("bootstrap persistence failed; node remains pending and credential expires")
		}
		if err = file.Sync(); err != nil {
			return err
		}
		fmt.Printf("Node %s bootstrap written to %s; expires in 15 minutes.\n", bootstrap.NodeID, *out)
		return nil
	case "serve":
		caPEM, err := os.ReadFile(os.Getenv("PDS_ENROLLMENT_CA_FILE"))
		if err != nil {
			return errors.New("enrollment CA file required")
		}
		block, _ := pem.Decode(caPEM)
		if block == nil {
			return errors.New("invalid enrollment CA")
		}
		store.CA, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return errors.New("invalid enrollment CA")
		}
		keyPath := os.Getenv("PDS_ENROLLMENT_CA_KEY_FILE")
		info, err := os.Lstat(keyPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("enrollment CA key must be a private regular file")
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return err
		}
		block, _ = pem.Decode(keyPEM)
		if block == nil {
			return errors.New("invalid enrollment CA key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return errors.New("enrollment CA key must use PKCS8")
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return errors.New("unsupported enrollment CA key")
		}
		store.Key = signer
		certificate, err := tls.LoadX509KeyPair(os.Getenv("PDS_ENROLLMENT_CERT_FILE"), enrollmentListenerKeyFile())
		if err != nil {
			return errors.New("enrollment listener certificate required")
		}
		server := &http.Server{Addr: "127.0.0.1:9092", Handler: store.Handler(), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		done := make(chan error, 1)
		go func() {
			fmt.Println("Local enrollment listener on 127.0.0.1:9092")
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
	default:
		return errors.New("unknown node administration command")
	}
}

// The old name remains accepted for direct operator scripts during transition.
func enrollmentListenerKeyFile() string {
	if path := os.Getenv("PDS_ENROLLMENT_LISTENER_KEY_FILE"); path != "" {
		return path
	}
	return os.Getenv("PDS_ENROLLMENT_KEY_FILE")
}
