package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/app"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/enrollment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/handleproof"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(startup, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = migrate.Apply(startup, pool); err != nil {
		return err
	}
	if err = recovery.Check(startup, pool); err != nil {
		return err
	}
	if err = deployment.Migrate(startup, pool); err != nil {
		return err
	}
	deployments, err := deployment.New(pool)
	if err != nil {
		return err
	}
	owners, err := auth.New(startup, pool, cfg.Auth)
	if err != nil {
		return err
	}
	var enrollmentCipher *enrollment.Cipher
	if path := os.Getenv("PDS_ENROLLMENT_KEY_FILE"); path != "" {
		enrollmentCipher, err = enrollment.LoadCipher(path)
		if err != nil {
			return err
		}
	}
	enrollments, err := enrollment.New(pool, enrollmentCipher)
	if err != nil {
		return err
	}
	handles := &handleproof.Store{Pool: pool, Checker: handleproof.Checker{DNS: net.DefaultResolver, Directory: handleproof.NewDirectory(cfg.Auth.Mode == "local"), Local: cfg.Auth.Mode == "local"}}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: app.Handler(migrate.Probe{Pool: pool}, owners.Register, (entitlement.Store{Pool: pool}).Register(owners), deployments.Register(owners, cfg.Auth.Mode == "local"), enrollments.Register(owners, cfg.Auth.Mode == "local"), handles.Register(owners, cfg.Auth.Mode == "local")), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() {
		log.Printf("management API listening on %s", cfg.ListenAddress)
		done <- server.ListenAndServe()
	}()
	select {
	case err = <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
