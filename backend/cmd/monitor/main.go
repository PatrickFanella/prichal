package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("local monitor requires explicit local mode")
	}
	config, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil {
		return errors.New("local management connection required")
	}
	host := net.ParseIP(config.ConnConfig.Host)
	if config.ConnConfig.Host != "localhost" && (host == nil || !host.IsLoopback()) {
		return errors.New("local monitor requires a loopback database")
	}
	if config.ConnConfig.Database != "pds_local" {
		return errors.New("local monitor requires pds_local")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return errors.New("management monitoring unavailable")
	}
	defer pool.Close()
	if err = (migrate.Probe{Pool: pool}).Check(ctx); err != nil {
		return errors.New("monitor requires current management schema")
	}
	client, err := nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
	if err != nil {
		return err
	}
	findings, err := observability.Scan(ctx, pool, client)
	if err != nil {
		return errors.New("local monitoring scan failed; prior alerts are retained")
	}
	return json.NewEncoder(os.Stdout).Encode(findings)
}
