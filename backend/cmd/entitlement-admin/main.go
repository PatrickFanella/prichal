// entitlement-admin writes explicit local pilot exceptions. It cannot accept payments.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/user"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
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
	if len(os.Args) < 2 || os.Args[1] == "--help" {
		fmt.Println("Usage: entitlement-admin {grant|revoke} --help\nLocal pilot exceptions only. No payment or runtime administration.")
		return nil
	}
	command := os.Args[1]
	if command != "grant" && command != "revoke" {
		return errors.New("Use grant or revoke.")
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	org := flags.String("organization", "", "organization UUID")
	key := flags.String("key", "", "stable request UUID")
	reason := flags.String("reason", "", "pilot or qualification")
	var plan *string
	var hours *int
	if command == "grant" {
		plan = flags.String("plan", "", "local-single (1 native intent) or local-small (3)")
		hours = flags.Int("hours", 24, "grant duration: 1..720 hours")
	}
	if err := flags.Parse(os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *org == "" || *key == "" || *reason == "" {
		return errors.New("Supply organization, key and reason flags.")
	}
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("Pilot administration requires explicit local mode.")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || !net.ParseIP(cfg.ConnConfig.Host).IsLoopback() || cfg.ConnConfig.Database != "pds_local" {
		return errors.New("Pilot administration requires loopback pds_local.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("Local management database unavailable.")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "pds_local" {
		return errors.New("Pilot administration must target pds_local.")
	}
	if err = migrate.Apply(ctx, pool); err != nil {
		return err
	}
	actor, err := user.Current()
	if err != nil {
		return errors.New("Operator identity unavailable.")
	}
	request := entitlement.Request{Key: *key, State: "revoked", Reason: *reason}
	if command == "grant" {
		request.State = "active"
		request.Plan = *plan
		request.Hours = *hours
	}
	operation, err := (entitlement.Store{Pool: pool}).Submit(ctx, actor.Username, *org, request)
	if err != nil {
		return errors.New("Pilot request was invalid or conflicted. Inspect its stable key and payload.")
	}
	return json.NewEncoder(os.Stdout).Encode(operation)
}
