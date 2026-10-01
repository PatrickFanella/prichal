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

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
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
		fmt.Println("Usage: service-admin {request|status} --help")
		return nil
	}
	command := os.Args[1]
	if command != "request" && command != "status" {
		return errors.New("unknown command; use service-admin --help")
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	var deployment, key, state, reason, operation *string
	if command == "request" {
		deployment = flags.String("deployment", "", "managed instance UUID")
		key = flags.String("key", "", "stable request UUID; reuse it for retries")
		state = flags.String("state", "", "running or suspended")
		reason = flags.String("reason", "", "maintenance, safety_review or resource_pressure")
	} else {
		operation = flags.String("operation", "", "operation UUID")
	}
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: service-admin %s [flags]\n", command)
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "Examples:")
		if command == "request" {
			fmt.Fprintln(flags.Output(), "  service-admin request --deployment INSTANCE_UUID --key REQUEST_UUID --state suspended --reason maintenance")
		} else {
			fmt.Fprintln(flags.Output(), "  service-admin status --operation OPERATION_UUID")
		}
	}
	if err := flags.Parse(os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (command == "request" && (*deployment == "" || *key == "" || *state == "" || *reason == "")) || (command == "status" && *operation == "") {
		flags.Usage()
		return errors.New("required flags are missing or unexpected arguments were provided")
	}
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("service administration requires explicit local mode")
	}
	config, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || !net.ParseIP(config.ConnConfig.Host).IsLoopback() {
		return errors.New("service administration requires a loopback database")
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
		return errors.New("service administration must target pds_local")
	}
	if err = migrate.Apply(ctx, pool); err != nil {
		return err
	}
	store, err := lifecycle.New(pool)
	if err != nil {
		return err
	}
	var result lifecycle.Operation
	if command == "status" {
		result, err = store.Get(ctx, *operation)
	} else {
		actor, actorErr := user.Current()
		if actorErr != nil {
			return errors.New("operator identity unavailable")
		}
		result, err = store.Submit(ctx, actor.Username, *deployment, lifecycle.Request{Key: *key, State: *state, Reason: *reason})
	}
	if err != nil {
		return errors.New("service intent unavailable or conflicts with retained state; inspect the operation and retry with the same key")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
