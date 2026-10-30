// managed-fixture-retire records only the explicitly authorized empty staging-02 retirement.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

const instance = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
const node = "60684f10-c6b2-4833-a0b8-fb9f96a7869b"

type options struct{ action, key, native, management, withdrawal, retirement string }

func parse(args []string) (options, error) {
	var o options
	if len(args) == 0 || args[0] != "prepare" && args[0] != "release" {
		return o, errors.New("choose prepare or release")
	}
	o.action = args[0]
	f := flag.NewFlagSet("managed-fixture-retire", flag.ContinueOnError)
	f.StringVar(&o.key, "key", "", "retained retirement request UUID")
	f.StringVar(&o.native, "native-backup-sha256", "", "verified native backup digest")
	f.StringVar(&o.management, "management-backup-sha256", "", "verified management backup digest")
	f.StringVar(&o.withdrawal, "withdrawal-sha256", "", "reviewed edge withdrawal receipt digest")
	f.StringVar(&o.retirement, "native-retirement-sha256", "", "reviewed restricted native retirement receipt digest")
	confirm := f.Bool("confirm-empty-staging-02", false, "confirm only the authorized empty staging-02 fixture")
	if e := f.Parse(args[1:]); e != nil {
		return o, e
	}
	if !*confirm || f.NArg() != 0 {
		return o, errors.New("explicit empty staging-02 confirmation and no positional arguments required")
	}
	if _, e := managednamespace.Endpoint(o.key); e != nil {
		return o, e
	}
	digests := []string{o.native, o.management}
	if o.action == "release" {
		digests = []string{o.withdrawal, o.retirement}
		if o.native != "" || o.management != "" {
			return o, errors.New("release uses retained backup custody")
		}
	} else if o.withdrawal != "" || o.retirement != "" {
		return o, errors.New("prepare cannot assert withdrawal or release")
	}
	for _, s := range digests {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 || hex.EncodeToString(b) != s {
			return o, errors.New("two canonical SHA256 custody digests required")
		}
	}
	return o, nil
}
func run(args []string) error {
	o, e := parse(args)
	if e != nil {
		return e
	}
	staging, e := config.LoadStagingProvisioning(os.Getenv("PDS_MODE"))
	if e != nil {
		return e
	}
	if !staging {
		return errors.New("explicit production staging required")
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if e != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 35437 || cfg.ConnConfig.User != "pds_staging" || cfg.ConnConfig.Database != "pds_local" {
		return errors.New("dedicated staging-02 registry required")
	}
	client, e := nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = (migrate.Probe{Pool: pool}).Check(ctx); e != nil {
		return e
	}
	actor, e := user.Current()
	if e != nil {
		return e
	}
	store := managedrouting.Store{Pool: pool}
	var result managedrouting.Retirement
	if o.action == "prepare" {
		result, e = store.PrepareRetirement(ctx, actor.Username, managedrouting.RetirementRequest{DeploymentID: instance, NodeID: node, Key: o.key, NativeBackupSHA256: o.native, ManagementBackupSHA256: o.management}, client)
	} else {
		result, e = store.ReleaseRetirement(ctx, actor.Username, instance, o.key, o.withdrawal, o.retirement, client)
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
