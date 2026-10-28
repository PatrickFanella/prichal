package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/billing"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/enrollment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/exports"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/hostingexit"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
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
	mode := os.Getenv("PDS_MODE")
	staging, err := config.LoadStagingProvisioning(mode)
	if err != nil {
		return err
	}
	managed, err := config.LoadStagingManagedRuntime(mode, staging)
	if err != nil {
		return err
	}
	if mode != "local" && !staging {
		return errors.New("worker requires local mode or production mode with PDS_STAGING_PROVISIONING=true")
	}
	agentClient, err := loadAgentClient()
	if err != nil {
		return err
	}
	if staging && agentClient == nil {
		return errors.New("staging worker requires authenticated agent credentials")
	}
	if staging {
		if _, err := billing.LoadSettings(false); err != nil {
			return err
		}
		if _, err := billing.LoadSessionSettings(false, billing.Settings{}); err != nil {
			return err
		}
		if _, err := exports.LoadArtifacts(false); err != nil {
			return err
		}
		if os.Getenv("PDS_ENROLLMENT_KEY_FILE") != "" {
			return errors.New("staging provisioning worker does not process enrollment jobs")
		}
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
	if err = recovery.Check(startup, pool); err != nil {
		return err
	}
	if err = deployment.Migrate(startup, pool); err != nil {
		return err
	}
	if staging {
		return runStagingWorker(ctx, pool, agentClient, managed)
	}
	var exportAgent exports.Agent
	var enrollmentAgent enrollment.Agent
	var memberAgent enrollment.MemberAgent
	var removalAgent enrollment.MemberRemovalAgent
	var serviceAgent lifecycle.Agent
	var agent deployment.AgentClient
	if client := agentClient; client != nil {
		agent = client
		enrollmentAgent = client
		memberAgent = client
		removalAgent = client
		serviceAgent = client
		exportAgent = client
	}
	var enrollmentCipher *enrollment.Cipher
	if path := os.Getenv("PDS_ENROLLMENT_KEY_FILE"); path != "" {
		enrollmentCipher, err = enrollment.LoadCipher(path)
		if err != nil {
			return err
		}
	}
	billingSettings, err := billing.LoadSettings(true)
	if err != nil {
		return err
	}
	billingAuthority, err := billing.NewAuthority(billingSettings)
	if err != nil {
		return err
	}
	billingEvents, err := billing.New(pool)
	if err != nil {
		return err
	}
	sessionSettings, err := billing.LoadSessionSettings(true, billingSettings)
	if err != nil {
		return err
	}
	var checkoutIntents *billing.CheckoutIntents
	var checkoutAuthority *billing.FixtureSessions
	var checkoutCompletions *billing.CompletionInbox
	if sessionSettings.Billing.Enabled {
		checkoutAuthority, err = billing.NewFixtureSessions(sessionSettings)
		if err != nil {
			return err
		}
		checkoutIntents, err = billing.NewCheckoutIntents(true, pool, checkoutAuthority)
		if err != nil {
			return err
		}
	}
	if checkoutAuthority != nil {
		checkoutCompletions, err = billing.NewCompletionInbox(true, pool, checkoutAuthority)
		if err != nil {
			return err
		}
	}
	exportArtifacts, err := exports.LoadArtifacts(true)
	if err != nil {
		return err
	}
	portableExports, err := exports.New(pool, exportArtifacts)
	if err != nil {
		return err
	}
	hostingExits, err := hostingexit.New(pool, nil, portableExports)
	if err != nil {
		return err
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &hostingexit.Worker{Store: hostingExits})
	river.AddWorker(workers, &hostingexit.RetentionWorker{Store: hostingExits})
	river.AddWorker(workers, &exports.Worker{Store: portableExports, Agent: exportAgent})
	river.AddWorker(workers, &billing.Worker{Store: billingEvents, Authority: billingAuthority})
	checkoutWorker := &billing.CheckoutWorker{Store: checkoutIntents}
	if checkoutAuthority != nil {
		checkoutWorker.Authority = checkoutAuthority
	}
	river.AddWorker(workers, checkoutWorker)
	river.AddWorker(workers, &billing.CompletionWorker{Inbox: checkoutCompletions, Intents: checkoutIntents, Authority: checkoutAuthority})
	river.AddWorker(workers, &lifecycle.Worker{Pool: pool, Agent: serviceAgent})
	river.AddWorker(workers, &enrollment.Worker{Pool: pool, Cipher: enrollmentCipher, Agent: enrollmentAgent})
	river.AddWorker(workers, &enrollment.MemberRemovalWorker{Pool: pool, Agent: removalAgent})
	river.AddWorker(workers, &enrollment.MemberWorker{Pool: pool, Cipher: enrollmentCipher, Agent: memberAgent})
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

func loadAgentClient() (*nodeagent.Client, error) {
	ca := os.Getenv("PDS_WORKER_AGENT_CA_FILE")
	cert := os.Getenv("PDS_WORKER_AGENT_CERT_FILE")
	key := os.Getenv("PDS_WORKER_AGENT_KEY_FILE")
	if ca == "" && cert == "" && key == "" {
		return nil, nil
	}
	if ca == "" || cert == "" || key == "" {
		return nil, errors.New("PDS_WORKER_AGENT_CA_FILE, PDS_WORKER_AGENT_CERT_FILE and PDS_WORKER_AGENT_KEY_FILE must be configured together")
	}
	return nodeagent.NewClient(ca, cert, key)
}

func runStagingWorker(ctx context.Context, pool *pgxpool.Pool, agent deployment.AgentClient, managed bool) error {
	workers := river.NewWorkers()
	river.AddWorker(workers, &deployment.Worker{Pool: pool, Provisioner: deployment.Router{Pool: pool, Agent: agent, AgentOnly: true, ManagedRuntime: managed}})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{deployment.StagingQueue: {MaxWorkers: 1}, deployment.ManagedIntentQueue: {MaxWorkers: 1}}})
	if err != nil {
		return err
	}
	if err := client.Start(ctx); err != nil {
		return err
	}
	log.Print("private staging deployment worker started; host execution remains in the node agent")
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Stop(shutdown)
}
