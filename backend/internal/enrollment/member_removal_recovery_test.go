package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type removalRecoveryObserver struct {
	id, node string
	window   nodeagent.MemberExit
	reads    int
}

func (o *removalRecoveryObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{InstanceIDs: []string{o.id}}, nil
}
func (o *removalRecoveryObserver) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: true}, nil
}
func (o *removalRecoveryObserver) MemberExitStatus(_ context.Context, peer nodeagent.Peer, id, ticket string) (nodeagent.MemberExit, error) {
	o.reads++
	if peer.NodeID != o.node || id != o.id || ticket != o.window.Ticket {
		return nodeagent.MemberExit{}, errors.New("foreign observation")
	}
	return o.window, nil
}

// Opt-in logical PostgreSQL recovery, with simulated host observations only.
// No PDS, source management database, native credentials or host runtime is used.
func TestMemberRemovalManagementSnapshotRecovery(t *testing.T) {
	if os.Getenv("PDS_QUALIFY_MEMBER_MANAGEMENT_RECOVERY") != "true" {
		t.Skip("explicit disposable management snapshot qualification required")
	}
	const container = "prichal-storage-qualification-db-1"
	for _, confirmed := range []bool{false, true} {
		name := "requested"
		if confirmed {
			name = "migration_window"
		}
		t.Run(name, func(t *testing.T) {
			f := newStoreFixture(t)
			f.ownerAccount(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cfg := f.pool.Config().ConnConfig
			if cfg.Host != "127.0.0.1" || cfg.Port != 35433 || cfg.User != "pds_local" || !strings.HasPrefix(cfg.Database, "pds_smoke_") {
				t.Fatal("fixed disposable local database required")
			}
			docker := func(input []byte, args ...string) []byte {
				t.Helper()
				deadline, done := context.WithTimeout(ctx, 30*time.Second)
				defer done()
				cmd := exec.CommandContext(deadline, "docker", args...)
				cmd.Stdin = bytes.NewReader(input)
				out, err := cmd.Output()
				if err != nil {
					t.Fatal("fixed local snapshot operation failed")
				}
				return out
			}
			if !strings.HasPrefix(strings.TrimSpace(string(docker(nil, "context", "inspect", "--format", "{{.Endpoints.docker.Host}}"))), "unix:///") {
				t.Fatal("local Docker socket required")
			}
			var host struct {
				Config struct {
					Image  string
					Labels map[string]string
				}
				State struct{ Running bool }
			}
			if json.Unmarshal(docker(nil, "inspect", "--format", "{{json .}}", container), &host) != nil || !host.State.Running || host.Config.Labels["com.docker.compose.project"] != "prichal-storage-qualification" || host.Config.Labels["com.docker.compose.service"] != "db" || host.Config.Image != "postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73" {
				t.Fatal("fixed qualification PostgreSQL required")
			}
			member := acceptedRemovalMember(t, f, 970, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
			key := memberKey(971)
			removal, err := f.store.RemoveMember(ctx, f.owner, member.ID, key)
			if err != nil {
				t.Fatal(err)
			}
			window := removalWindow(member.DID, time.Now().UTC().Truncate(time.Millisecond))
			window.InstanceID = f.deployment
			window.Ticket = member.ID
			agent := &removalAgentFixture{result: window}
			if confirmed {
				requireRemovalSnooze(t, (&MemberRemovalWorker{Pool: f.pool, Agent: agent}).Work(ctx, removalJob(removal.ID, 1)))
			}
			ledger := func(pool *pgxpool.Pool) string {
				t.Helper()
				var value string
				err := pool.QueryRow(ctx, `SELECT json_build_object('members',(SELECT json_agg(m ORDER BY m.id) FROM member_invitations m),'memberEvents',(SELECT json_agg(e ORDER BY e.id) FROM member_invitation_events e),'removals',(SELECT json_agg(r ORDER BY r.id) FROM member_removals r),'removalEvents',(SELECT json_agg(e ORDER BY e.id) FROM member_removal_events e),'jobs',(SELECT json_agg(j ORDER BY j.id) FROM river_job j),'sequence',(SELECT json_build_object('lastValue',last_value,'isCalled',is_called) FROM member_removal_events_id_seq))::text`).Scan(&value)
				if err != nil {
					t.Fatal(err)
				}
				return value
			}
			before := ledger(f.pool)
			snapshot := docker(nil, "exec", container, "pg_dump", "-U", "pds_local", "-Fc", cfg.Database)
			if len(snapshot) == 0 || len(snapshot) > 8<<20 {
				t.Fatal("disposable snapshot outside bound")
			}
			digest := sha256.Sum256(snapshot)
			restored := testdb.NewRecovery(t)
			docker(snapshot, "exec", "-i", container, "pg_restore", "-U", "pds_local", "--exit-on-error", "--no-owner", "-d", restored.Config().ConnConfig.Database)
			copy, err := New(restored, f.store.Cipher)
			if err != nil {
				t.Fatal(err)
			}
			if ledger(restored) != before {
				t.Fatal("restored member intent, actor audit, sequence or jobs differ")
			}
			if err := recovery.Hold(ctx, restored, digest[:]); err != nil {
				t.Fatal(err)
			}
			calls := agent.calls
			worker := &MemberRemovalWorker{Pool: restored, Agent: agent}
			requireRemovalSnooze(t, worker.Work(ctx, removalJob(removal.ID, 1)))
			if _, err := copy.RemoveMember(ctx, f.owner, member.ID, key); !errors.Is(err, recovery.ErrHeld) {
				t.Fatal("held restore admitted removal replay", err)
			}
			if agent.calls != calls || ledger(restored) != before {
				t.Fatal("held work reached host or changed retained rows")
			}
			var node string
			if err := restored.QueryRow(ctx, `SELECT node_id::text FROM instance_placements WHERE deployment_id=$1`, f.deployment).Scan(&node); err != nil {
				t.Fatal(err)
			}
			observer := &removalRecoveryObserver{id: f.deployment, node: node, window: window}
			if confirmed {
				observer.window.DeactivateAfter--
				if err := recovery.Reconcile(ctx, restored, observer); err == nil {
					t.Fatal("changed host deadline released recovered jobs")
				}
				if err := recovery.Check(ctx, restored); !errors.Is(err, recovery.ErrHeld) {
					t.Fatal("mismatch lost recovery hold")
				}
				observer.window = window
				if err := recovery.Reconcile(ctx, restored, observer); err != nil {
					t.Fatal(err)
				}
				replayed, err := copy.RemoveMember(ctx, f.owner, member.ID, key)
				if err != nil || replayed.ID != removal.ID || replayed.DID != member.DID || replayed.DeactivateAfter == nil || !replayed.DeactivateAfter.Equal(time.UnixMilli(window.DeactivateAfter)) {
					t.Fatal("restored replay changed identity or window", err)
				}
				requireRemovalSnooze(t, worker.Work(ctx, removalJob(removal.ID, 1)))
			} else {
				if err := recovery.Reconcile(ctx, restored, observer); err == nil || observer.reads != 0 {
					t.Fatal("uncertain removal released hold or dispatched observation")
				}
				if err := recovery.Check(ctx, restored); !errors.Is(err, recovery.ErrHeld) {
					t.Fatal("uncertain removal lost hold")
				}
			}
			if ledger(restored) != before || ledger(f.pool) != before || agent.calls != calls {
				t.Fatal("snapshot/reconciliation/replay changed source, queued work or native calls")
			}
			repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(repository, ".local", "member-removal-recovery")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(root)
			if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
				t.Fatal("private regular qualification directory required")
			}
			directory, err := os.MkdirTemp(root, name+"-")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "snapshot.dump"), snapshot, 0600); err != nil {
				t.Fatal(err)
			}
			ledgerDigest := sha256.Sum256([]byte(before))
			report := map[string]any{"completedAt": time.Now().UTC(), "state": name, "snapshotBytes": len(snapshot), "snapshotSha256": hex.EncodeToString(digest[:]), "sourceAndRestoredLedgerSha256": hex.EncodeToString(ledgerDigest[:]), "sourceAndRestoredRowsRetained": true, "restoredJobAndSequenceRetained": true, "holdFencedWorkerAndReplay": true, "confirmedWindowReconciled": confirmed, "uncertainStateRemainsHeld": !confirmed, "nativeCallsDuringRecovery": 0, "hostObservationsSimulated": true, "productionRecoveryQualified": false}
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "report.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			t.Log("private management recovery receipt:", directory)
		})
	}
}
