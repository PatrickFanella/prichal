// Package observability records bounded operational findings without protocol payloads.
package observability

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Observer interface {
	Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error)
	Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error)
	Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error)
}
type Finding struct {
	Subject      string `json:"subject"`
	DeploymentID string `json:"deploymentId,omitempty"`
	Kind         string `json:"kind"`
	Active       bool   `json:"active"`
}

// Scan runs independently of the management API and never reconciles a host.
func Scan(ctx context.Context, pool *pgxpool.Pool, observer Observer) ([]Finding, error) {
	if observer == nil {
		return nil, errors.New("authenticated host observer required")
	}
	var findings []Finding
	var samples []nodeagent.Usage
	nodes, err := pool.Query(ctx, `SELECT id::text,agent_endpoint,public_key_hash FROM hosting_nodes WHERE enrolled_at IS NOT NULL ORDER BY id LIMIT 101`)
	if err != nil {
		return nil, err
	}
	var peers []nodeagent.Peer
	for nodes.Next() {
		var peer nodeagent.Peer
		if err = nodes.Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
			nodes.Close()
			return nil, err
		}
		peers = append(peers, peer)
	}
	nodes.Close()
	if err = nodes.Err(); err != nil {
		return nil, err
	}
	if len(peers) > 100 {
		return nil, errors.New("local monitoring node limit exceeded")
	}
	for _, peer := range peers {
		capacity, err := observer.Capacity(ctx, peer)
		subject := "node/" + peer.NodeID
		findings = append(findings, Finding{Subject: subject, Kind: "host_unavailable", Active: err != nil})
		if err == nil {
			findings = append(findings, Finding{Subject: subject, Kind: "capacity_exhausted", Active: len(capacity.InstanceIDs) >= capacity.MaximumSlots})
		}
		rows, err := pool.Query(ctx, `SELECT d.id::text,d.state,d.updated_at,coalesce(c.desired_state,'running'),coalesce(c.state,'applied'),coalesce(c.generation,0),coalesce(c.id::text,'') FROM deployments d LEFT JOIN LATERAL (SELECT id,desired_state,state,generation FROM service_operations WHERE deployment_id=d.id AND kind='service' ORDER BY generation DESC LIMIT 1) c ON true JOIN instance_placements p ON p.deployment_id=d.id WHERE p.node_id=$1 ORDER BY d.id LIMIT 1001`, peer.NodeID)
		if err != nil {
			return nil, err
		}
		type placed struct {
			id, state, desired, phase, ticket string
			generation                        int64
			updated                           time.Time
		}
		var instances []placed
		for rows.Next() {
			var value placed
			if err = rows.Scan(&value.id, &value.state, &value.updated, &value.desired, &value.phase, &value.generation, &value.ticket); err != nil {
				rows.Close()
				return nil, err
			}
			instances = append(instances, value)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		if len(instances) > 1000 {
			return nil, errors.New("local monitoring instance limit exceeded")
		}
		for _, instance := range instances {
			if instance.state != "ready" {
				continue
			}
			status, err := observer.Status(ctx, peer, instance.id)
			bad := err != nil || status.ID != instance.id || status.Endpoint != nodeagent.LocalEndpoint(instance.id) || status.State != "running" || !status.Healthy
			if instance.phase != "applied" {
				bad = true
			}
			if status.ServiceGeneration != instance.generation || status.ServiceTicket != instance.ticket || (instance.generation > 0 && status.ServicePhase != "applied") || (instance.generation == 0 && status.ServicePhase != "") {
				bad = true
			}
			if instance.desired == "suspended" && instance.phase == "applied" {
				bad = err != nil || status.ID != instance.id || status.Endpoint != nodeagent.LocalEndpoint(instance.id) || status.State != "suspended" || status.Healthy || status.ServiceGeneration != instance.generation || status.ServiceTicket != instance.ticket || status.ServicePhase != "applied"
			}

			var accountGeneration int64
			var accountPhase, accountDesired, accountTicket, expectedAccountDID string
			accountErr := pool.QueryRow(ctx, `SELECT generation,state,desired_state,id::text,coalesce((SELECT account_did FROM enrollment_operations WHERE deployment_id=$1 AND state='verified' ORDER BY created_at DESC LIMIT 1),'') FROM service_operations WHERE deployment_id=$1 AND kind='account' ORDER BY generation DESC LIMIT 1`, instance.id).Scan(&accountGeneration, &accountPhase, &accountDesired, &accountTicket, &expectedAccountDID)
			if accountErr != nil && !errors.Is(accountErr, pgx.ErrNoRows) {
				return nil, accountErr
			}
			if accountErr == nil {
				if accountPhase != "applied" {
					bad = true
				} else if !bad && instance.desired != "suspended" {
					inspector, ok := observer.(interface {
						AccountStatus(context.Context, nodeagent.Peer, string) (nodeagent.AccountStatus, error)
					})
					if !ok {
						bad = true
					} else {
						account, err := inspector.AccountStatus(ctx, peer, instance.id)
						bad = err != nil || account.InstanceID != instance.id || expectedAccountDID == "" || account.DID != expectedAccountDID || account.State != accountDesired || account.Generation != accountGeneration || account.Ticket != accountTicket || account.Phase != "applied"
					}
				}
			}
			findings = append(findings, Finding{Subject: "deployment/" + instance.id, DeploymentID: instance.id, Kind: "runtime_unhealthy", Active: bad})
			if bad || instance.desired == "suspended" {
				findings = append(findings, Finding{Subject: "deployment/" + instance.id, DeploymentID: instance.id, Kind: "usage_unavailable", Active: true})
				continue
			}
			usage, usageErr := observer.Usage(ctx, peer, instance.id)
			if usageErr == nil {
				usageErr = usage.Validate(instance.id)
			}
			findings = append(findings, Finding{Subject: "deployment/" + instance.id, DeploymentID: instance.id, Kind: "usage_unavailable", Active: usageErr != nil})
			if usageErr == nil {
				samples = append(samples, usage)
				findings = append(findings, Finding{Subject: "deployment/" + instance.id, DeploymentID: instance.id, Kind: "disk_pressure", Active: usage.FilesystemAvailableBytes < 256<<20}, Finding{Subject: "deployment/" + instance.id, DeploymentID: instance.id, Kind: "account_ceiling_exceeded", Active: usage.Accounts > 1})
			}

		}
	}
	rows, err := pool.Query(ctx, `SELECT id::text,state,updated_at FROM deployments ORDER BY id LIMIT 1001`)
	if err != nil {
		return nil, err
	}
	count := 0
	for rows.Next() {
		var id, state string
		var updated time.Time
		if err = rows.Scan(&id, &state, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		count++
		findings = append(findings, Finding{Subject: "deployment/" + id, DeploymentID: id, Kind: "deployment_failed", Active: state == "failed"}, Finding{Subject: "deployment/" + id, DeploymentID: id, Kind: "job_backlog", Active: (state == "queued" || state == "configuring" || state == "checking") && time.Since(updated) > 2*time.Minute})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if count > 1000 {
		return nil, errors.New("local monitoring deployment limit exceeded")
	}
	var held bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return nil, err
	}
	findings = append(findings, Finding{Subject: "management", Kind: "recovery_held", Active: held}, Finding{Subject: "management", Kind: "backup_unconfigured", Active: true})
	if err = recordScan(ctx, pool, findings, samples); err != nil {
		return nil, err
	}
	return findings, nil
}

// Record updates a complete successful scan. Failed scans never clear prior findings.
func Record(ctx context.Context, pool *pgxpool.Pool, findings []Finding) error {
	return recordScan(ctx, pool, findings, nil)
}
func recordScan(ctx context.Context, pool *pgxpool.Pool, findings []Finding, samples []nodeagent.Usage) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(73647303)`); err != nil {
		return err
	}
	for _, finding := range findings {
		var deployment any
		if finding.DeploymentID != "" {
			deployment = finding.DeploymentID
		}
		_, err = tx.Exec(ctx, `INSERT INTO service_observations(subject_key,deployment_id,kind,active,resolved_at) VALUES($1,$2,$3,$4,CASE WHEN $4 THEN NULL ELSE now() END) ON CONFLICT(subject_key,kind) DO UPDATE SET active=excluded.active,last_observed_at=now(),first_observed_at=CASE WHEN excluded.active AND NOT service_observations.active THEN now() ELSE service_observations.first_observed_at END,resolved_at=CASE WHEN excluded.active THEN NULL ELSE coalesce(service_observations.resolved_at,now()) END`, finding.Subject, deployment, finding.Kind, finding.Active)
		if err != nil {
			return err
		}
	}
	for _, sample := range samples {
		if err = sample.Validate(sample.InstanceID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO instance_usage(deployment_id,data_bytes,blob_bytes,data_files,blob_files,accounts,measured_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(deployment_id) DO UPDATE SET data_bytes=excluded.data_bytes,blob_bytes=excluded.blob_bytes,data_files=excluded.data_files,blob_files=excluded.blob_files,accounts=excluded.accounts,measured_at=excluded.measured_at,recorded_at=now() WHERE instance_usage.measured_at<=excluded.measured_at`, sample.InstanceID, sample.DataBytes, sample.BlobBytes, sample.DataFiles, sample.BlobFiles, sample.Accounts, time.UnixMilli(sample.ObservedAtMillis)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_probe_runs(singleton,completed_at) VALUES(true,now()) ON CONFLICT(singleton) DO UPDATE SET completed_at=excluded.completed_at`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
