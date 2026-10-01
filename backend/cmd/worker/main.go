package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if os.Getenv("PDS_MODE") != "local" {
		return errors.New("test worker requires explicit local mode")
	}
	url := os.Getenv("PDS_DATABASE_URL")
	if url == "" {
		return errors.New("PDS_DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(startup, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = migrate.Apply(startup, pool); err != nil {
		return err
	}
	if err = deployment.Migrate(startup, pool); err != nil {
		return err
	}
	var agent deployment.AgentClient
	if os.Getenv("PDS_WORKER_AGENT_CA_FILE") != "" {
		client, err := nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
		if err != nil {
			return err
		}
		agent = client
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &deployment.Worker{Pool: pool, Provisioner: deployment.Router{Pool: pool, Agent: agent}})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}}})
	if err != nil {
		return err
	}
	if err = client.Start(ctx); err != nil {
		return err
	}
	log.Print("local deployment worker started; host execution remains in the node agent")
	<-ctx.Done()
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	return client.Stop(shutdown)
}
