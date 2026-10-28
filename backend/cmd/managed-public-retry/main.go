// managed-public-retry resumes only the designated staging-02 public verification cycle.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

const instance = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
const node = "60684f10-c6b2-4833-a0b8-fb9f96a7869b"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	staging, err := config.LoadStagingProvisioning(os.Getenv("PDS_MODE"))
	if err != nil {
		return err
	}
	if !staging {
		return errors.New("public retry requires explicit production staging")
	}
	flags := flag.NewFlagSet("managed-public-retry", flag.ContinueOnError)
	key := flags.String("key", "", "stable retry UUID")
	confirmed := flags.Bool("confirm-staging-02", false, "retry only the retained disposable staging-02 endpoint")
	if err = flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if !*confirmed || flags.NArg() != 0 {
		return errors.New("--key UUID --confirm-staging-02 required")
	}
	if _, err = managednamespace.Endpoint(*key); err != nil {
		return err
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 35437 || cfg.ConnConfig.Database != "pds_local" || cfg.ConnConfig.User != "pds_staging" {
		return errors.New("dedicated staging-02 management registry required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = migrate.Apply(ctx, pool); err != nil {
		return err
	}
	if err = deployment.Migrate(ctx, pool); err != nil {
		return err
	}
	store, err := deployment.New(pool)
	if err != nil {
		return err
	}
	store.Staging = true
	actor, err := user.Current()
	if err != nil {
		return errors.New("operator identity unavailable")
	}
	result, err := store.RetryManagedPublic(ctx, actor.Username, instance, node, *key)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
