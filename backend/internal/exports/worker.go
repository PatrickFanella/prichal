package exports

import (
	"context"
	"errors"
	"os"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/riverqueue/river"
)

// Agent captures only the expected native actor through an authenticated fixed contract.
type Agent interface {
	Export(context.Context, nodeagent.Peer, string, string) (nodeagent.PortableExport, error)
	Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
}

// Worker serializes export publication and native lifecycle work for each deployment.
type Worker struct {
	river.WorkerDefaults[Args]
	Store *Store
	Agent Agent
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	if w.Store.Artifacts == nil || w.Agent == nil {
		return river.JobSnooze(time.Minute)
	}
	if err := recovery.Check(ctx, w.Store.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.capture(ctx, job.Args.ExportID)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrUnavailable) {
		return river.JobSnooze(time.Minute)
	}
	if job.Attempt >= job.MaxAttempts {
		tx, txErr := w.Store.Pool.Begin(ctx)
		if txErr != nil {
			return errors.New("export outcome could not be retained")
		}
		defer tx.Rollback(ctx)
		changed, txErr := tx.Exec(ctx, `UPDATE export_operations SET state='failed',updated_at=now() WHERE id=$1 AND state='queued'`, job.Args.ExportID)
		if txErr == nil && changed.RowsAffected() > 0 {
			_, txErr = tx.Exec(ctx, `INSERT INTO export_events(export_id,state) VALUES($1,'failed')`, job.Args.ExportID)
		}
		if txErr != nil {
			return errors.New("export outcome could not be retained")
		}
		if txErr = tx.Commit(ctx); txErr != nil {
			return errors.New("export outcome could not be retained")
		}
	}
	return errors.New("portable capture could not be verified; retained data is unchanged")
}
func (w *Worker) NextRetry(job *river.Job[Args]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}
func (w *Worker) capture(ctx context.Context, id string) error {
	tx, err := w.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment, did, state string
	if err = tx.QueryRow(ctx, `SELECT deployment_id,account_did FROM export_operations WHERE id=$1`, id).Scan(&deployment, &did); err != nil {
		return err
	}
	var ready string
	if err = tx.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1 FOR UPDATE`, deployment).Scan(&ready); err != nil {
		return err
	}
	if ready != "ready" {
		return ErrUnavailable
	}
	if err = tx.QueryRow(ctx, `SELECT state FROM export_operations WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
		return err
	}
	if state != "queued" {
		return tx.Commit(ctx)
	}
	plain, manifest, err := w.Store.Artifacts.Read(id, deployment, did)
	if errors.Is(err, os.ErrNotExist) {
		var peer nodeagent.Peer
		if err = tx.QueryRow(ctx, `SELECT n.id,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining')`, deployment).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
			return err
		}
		status, statusErr := w.Agent.Status(ctx, peer, deployment)
		if statusErr != nil {
			return errors.New("export runtime status unavailable")
		}
		if status.State == "suspended" {
			return ErrUnavailable
		}
		if status.ID != deployment || status.State != "running" || !status.Healthy {
			return errors.New("export runtime not healthy")
		}
		data, captureErr := w.Agent.Export(ctx, peer, deployment, did)
		if captureErr != nil {
			return errors.New("portable capture unavailable")
		}
		plain, manifest, err = w.Store.Artifacts.Publish(id, deployment, did, data)
	}
	if err != nil {
		return errors.New("protected export artifact unavailable")
	}
	if _, err = tx.Exec(ctx, `UPDATE export_operations SET state='ready',archive_bytes=$2,archive_sha256=$3,captured_at=$4,expires_at=$4::timestamptz+interval '24 hours',updated_at=now() WHERE id=$1`, id, len(plain), digest(plain), manifest.CapturedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO export_events(export_id,state) VALUES($1,'ready')`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
