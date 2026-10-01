package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/user"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/billing"
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
	flags := flag.NewFlagSet("billing-fixture-admin", flag.ContinueOnError)
	org := flags.String("organization", "", "disposable organization UUID")
	customer := flags.String("customer", "", "fixture customer reference")
	subscription := flags.String("subscription", "", "fixture subscription reference")
	key := flags.String("key", "", "stable mapping UUID")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *org == "" || *customer == "" || *subscription == "" || *key == "" {
		flags.Usage()
		return errors.New("Supply organization, customer, subscription and key flags.")
	}
	if os.Getenv("PDS_MODE") != "local" || os.Getenv("PDS_BILLING_MODE") != "fixture" {
		return errors.New("Billing binding requires explicit local fixture mode.")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || !net.ParseIP(cfg.ConnConfig.Host).IsLoopback() || cfg.ConnConfig.Database != "pds_local" {
		return errors.New("Billing binding requires loopback pds_local.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("Local management database unavailable.")
	}
	defer pool.Close()
	if err = migrate.Apply(ctx, pool); err != nil {
		return err
	}
	store, err := billing.New(pool)
	if err != nil {
		return err
	}
	actor, err := user.Current()
	if err != nil {
		return errors.New("Local operator identity unavailable.")
	}
	if err = store.Bind(ctx, billing.Binding{OrganizationID: *org, CustomerID: *customer, SubscriptionID: *subscription, Key: *key, Actor: actor.Username}); err != nil {
		return errors.New("Fixture mapping was invalid or conflicts with retained ownership.")
	}
	fmt.Println(`{"mapped":true,"source":"local_fixture","paidEntitlement":false}`)
	return nil
}
