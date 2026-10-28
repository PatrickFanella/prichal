// managed-route-admin prepares private endpoint inventory; it never publishes or reloads a route.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managedrouting"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"os"
	"os/user"
	"regexp"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type scopedEmpty struct {
	*nodeagent.Client
	node string
}

func (s scopedEmpty) Status(ctx context.Context, p nodeagent.Peer, id string) (nodeagent.Instance, error) {
	if p.NodeID != s.node {
		return nodeagent.Instance{}, managedrouting.ErrConflict
	}
	return s.Client.Status(ctx, p, id)
}
func (s scopedEmpty) Usage(ctx context.Context, p nodeagent.Peer, id string) (nodeagent.Usage, error) {
	if p.NodeID != s.node {
		return nodeagent.Usage{}, managedrouting.ErrConflict
	}
	return s.Client.Usage(ctx, p, id)
}
func run() error {
	mode := os.Getenv("PDS_MODE")
	staging, err := config.LoadStagingProvisioning(mode)
	if err != nil {
		return err
	}
	if mode != "local" && !staging {
		return errors.New("managed route administration requires local or explicit production staging mode")
	}
	if len(os.Args) < 2 {
		return errors.New("usage: managed-route-admin {reserve|reserve-empty|observe|export}")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	id := flags.String("deployment", "", "retained native deployment UUID")
	node := flags.String("node", "", "expected current node UUID")
	key := flags.String("key", "", "stable observation request UUID")
	out := flags.String("out", "", "new private bundle directory")
	confirmed := flags.Bool("confirm-empty-fixture", false, "confirm the designated staging-02 fixture may receive a public-name reservation")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected managed route argument")
	}
	if command != "reserve" && command != "reserve-empty" && command != "observe" && command != "export" {
		return errors.New("unknown managed route command")
	}
	switch command {
	case "reserve":
		if *id == "" || *node != "" || *key != "" || *out != "" || *confirmed {
			return errors.New("reserve requires only --deployment UUID")
		}
	case "reserve-empty":
		if *id == "" || *node == "" || *key != "" || *out != "" || !*confirmed {
			return errors.New("reserve-empty requires --deployment UUID --node UUID --confirm-empty-fixture")
		}
	case "observe":
		if *id == "" || *node == "" || *key == "" || *out != "" || *confirmed {
			return errors.New("observe requires --deployment UUID --node UUID --key UUID")
		}
	case "export":
		if *out == "" || *id != "" || *node != "" || *key != "" || *confirmed {
			return errors.New("export requires only --out NEW_DIRECTORY")
		}
	}
	for _, identity := range []string{*id, *node, *key} {
		if identity != "" {
			if _, err := managednamespace.Endpoint(identity); err != nil {
				return managedrouting.ErrInvalid
			}
		}
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || net.ParseIP(cfg.ConnConfig.Host) == nil || !net.ParseIP(cfg.ConnConfig.Host).IsLoopback() {
		return errors.New("private loopback management registry required")
	}
	if cfg.ConnConfig.Database != "pds_local" && !(command == "export" && regexp.MustCompile(`^pds_recovery_[a-f0-9]{32}$`).MatchString(cfg.ConnConfig.Database)) {
		return errors.New("designated local staging registry or held read-only recovery copy required")
	}
	if command == "reserve-empty" && (!*confirmed || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 35437 || *node == "") {
		return errors.New("empty-fixture reservation requires explicit staging-02 scope, node and confirmation")
	}
	var client *nodeagent.Client
	if command == "observe" || command == "reserve-empty" {
		client, err = nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if command == "export" {
		if err := (migrate.Probe{Pool: pool}).Check(ctx); err != nil {
			return err
		}
	} else if err := migrate.Apply(ctx, pool); err != nil {
		return err
	}
	store := managedrouting.Store{Pool: pool}
	actor, err := user.Current()
	if err != nil {
		return errors.New("operator identity unavailable")
	}
	switch command {
	case "reserve":
		value, err := store.Reserve(ctx, actor.Username, *id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(value)
	case "reserve-empty", "observe":
		if command == "reserve-empty" {
			value, err := store.ReserveEmptyFixture(ctx, actor.Username, *id, scopedEmpty{Client: client, node: *node})
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(value)
		}
		value, err := store.Observe(ctx, actor.Username, *id, *node, *key, client)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(value)
	case "export":
		if *out == "" {
			return errors.New("export requires --out NEW_DIRECTORY")
		}
		bundle, err := store.Export(ctx)
		if err != nil {
			return err
		}
		if err := managedrouting.WriteBundle(*out, bundle); err != nil {
			return err
		}
		fmt.Printf("Private inventory generation %d exported with %d public certificates; no route published.\n", bundle.Inventory.Generation, len(bundle.Inventory.Endpoints))
		return nil
	}
	return errors.New("unknown managed route command")
}
