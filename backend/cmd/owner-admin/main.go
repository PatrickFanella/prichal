// Command owner-admin changes only disposable local management owner admission.
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

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" {
		fmt.Println("Usage: owner-admin {status|set} [--state open|closed --reason maintenance|safety_review|capacity_review|local_qualification]")
		return nil
	}
	if args[0] != "status" && args[0] != "set" {
		return errors.New("unknown command; use owner-admin --help")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	state := flags.String("state", "", "open or closed; required for set")
	reason := flags.String("reason", "", "maintenance, safety_review, capacity_review or local_qualification; required for set")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (args[0] == "set" && (*state != "open" && *state != "closed" || *reason == "")) || (args[0] == "status" && (*state != "" || *reason != "")) {
		return errors.New("invalid flags; use owner-admin --help")
	}
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("owner administration requires explicit local mode")
	}
	config, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || !net.ParseIP(config.ConnConfig.Host).IsLoopback() || config.ConnConfig.Database != "pds_local" {
		return errors.New("owner administration requires a loopback pds_local database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return errors.New("local management database unavailable")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "pds_local" {
		return errors.New("owner administration must target pds_local")
	}
	if err = migrate.Apply(ctx, pool); err != nil {
		return errors.New("local management migration unavailable")
	}
	store := auth.OwnerAdmissionStore{Pool: pool}
	var result auth.OwnerAdmission
	if args[0] == "status" {
		result, err = store.Status(ctx)
	} else {
		actor, actorErr := user.Current()
		if actorErr != nil {
			return errors.New("operator identity unavailable")
		}
		result, err = store.Set(ctx, actor.Username, *reason, *state == "open")
	}
	if err != nil {
		return errors.New("management owner admission unavailable; inspect the current policy and recovery hold")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
