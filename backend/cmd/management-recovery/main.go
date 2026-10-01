// management-recovery inspects only disposable local restoration databases.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"regexp"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if os.Getenv("PDS_MODE") != "local" || len(os.Args) != 2 {
		return errors.New("explicit local mode and hold, check or reconcile action required")
	}
	config, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil {
		return errors.New("local restore connection required")
	}
	host := net.ParseIP(config.ConnConfig.Host)
	if config.ConnConfig.Host != "localhost" && (host == nil || !host.IsLoopback()) {
		return errors.New("only loopback restore databases are supported")
	}
	if !regexp.MustCompile(`^pds_recovery_[a-f0-9]{32}$`).MatchString(config.ConnConfig.Database) {
		return errors.New("disposable restore database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return errors.New("restore database unavailable")
	}
	defer pool.Close()
	if err = migrate.Apply(ctx, pool); err != nil {
		return errors.New("restore schema could not be prepared")
	}
	switch os.Args[1] {
	case "hold":
		digest, err := hex.DecodeString(os.Getenv("PDS_RECOVERY_SNAPSHOT_SHA256"))
		if err != nil || len(digest) != 32 {
			return errors.New("snapshot SHA256 required")
		}
		if err = recovery.Hold(ctx, pool, digest); err != nil {
			return err
		}
		log.Print("restored management database held; do not start services before host reconciliation")
	case "check":
		return recovery.Check(ctx, pool)
	case "reconcile", "acknowledge-local-fixture", "qualify-replay":
		client, err := nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
		if err != nil {
			return err
		}
		if os.Args[1] == "qualify-replay" {
			if err = replay(ctx, pool, client, os.Getenv("PDS_RECOVERY_REPLAY_INSTANCE_ID")); err != nil {
				return err
			}
			log.Print("disposable management replay completed against existing local runtime")
			return nil
		}
		if os.Args[1] == "acknowledge-local-fixture" {
			if err = recovery.AcknowledgeRetained(ctx, pool, client, os.Getenv("PDS_RECOVERY_RETAINED_NODE_ID"), os.Getenv("PDS_RECOVERY_RETAINED_INSTANCE_ID")); err != nil {
				return err
			}
			log.Print("local fixture retained and recorded; management recovery remains held")
			return nil
		}
		if err = recovery.Reconcile(ctx, pool, client); err != nil {
			return err
		}
		log.Print("restored management database reconciled against authenticated host inventory")
	default:
		return errors.New("unsupported recovery action")
	}
	return nil
}
