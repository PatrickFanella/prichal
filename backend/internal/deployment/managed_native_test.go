package deployment

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type managedRemote struct {
	ids                                     map[string]bool
	privateCalls, managedCalls              int
	failAfterCreate, healthy, wrongEndpoint bool
	onEnsure                                func()
}

func (a *managedRemote) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 2, CPUs: 8, MemoryBytes: 8 << 30}, nil
}
func (a *managedRemote) Ensure(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	a.privateCalls++
	return nodeagent.Instance{}, errors.New("private fallback is forbidden")
}
func (a *managedRemote) EnsureManaged(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	a.managedCalls++
	a.ids[id] = true
	if a.onEnsure != nil {
		a.onEnsure()
	}
	if a.failAfterCreate {
		a.failAfterCreate = false
		return nodeagent.Instance{}, errors.New("uncertain managed native completion")
	}
	host, _ := managednamespace.Endpoint(id)
	endpoint := "https://" + host
	if a.wrongEndpoint {
		endpoint = nodeagent.LocalEndpoint(id)
	}
	return nodeagent.Instance{ID: id, Endpoint: endpoint, State: "running", Healthy: a.healthy}, nil
}

func nativeIntentFixture(t *testing.T) (*Store, string, string, Operation, *managedRemote) {
	t.Helper()
	s, owner, org := managedIntentFixture(t)
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now())`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	op, err := s.Submit(context.Background(), owner, org, Request{Kind: "agent", Name: "public", Region: "local", Key: "public"})
	if err != nil {
		t.Fatal(err)
	}
	return s, owner, org, op, &managedRemote{ids: map[string]bool{}}
}

func TestManagedNativeReplayStopsBeforePublicReadiness(t *testing.T) {
	s, owner, _, op, a := nativeIntentFixture(t)
	ctx := context.Background()
	p := AgentProvisioner{Pool: s.Pool, Client: a}
	if err := p.Step(ctx, op.ID); !errors.Is(err, ErrManagedEndpointPending) || a.managedCalls != 0 {
		t.Fatal("native opt-in was bypassed", err)
	}
	p.ManagedRuntime = true
	if err := p.Step(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	a.failAfterCreate = true
	if err := p.Step(ctx, op.ID); err == nil {
		t.Fatal("uncertain host completion was accepted")
	}
	p = AgentProvisioner{Pool: s.Pool, Client: a, ManagedRuntime: true}
	if err := p.Step(ctx, op.ID); err != nil {
		t.Fatal("reconstructed worker failed", err)
	}
	if err := p.Step(ctx, op.ID); !errors.Is(err, ErrHealthPending) {
		t.Fatal("unhealthy native process accepted", err)
	}
	a.healthy = true
	worker := Worker{Pool: s.Pool, Provisioner: Router{Pool: s.Pool, Agent: a, AgentOnly: true, ManagedRuntime: true}}
	job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: Args{op.ID}}
	var snooze *river.JobSnoozeError
	if err := worker.Work(ctx, job); !errors.As(err, &snooze) {
		t.Fatal("native health became public readiness or exhausted retry", err)
	}
	retained, err := s.Get(ctx, owner, op.ID)
	if err != nil || retained.State != "checking" || retained.Endpoint != "" || retained.ManagedHostname != op.ManagedHostname || len(retained.Events) != 3 || len(a.ids) != 1 || a.privateCalls != 0 {
		t.Fatal("managed replay changed identity or claimed serving", retained, err)
	}
	var placements int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE deployment_id=$1`, op.ID).Scan(&placements); err != nil || placements != 1 {
		t.Fatal("replay changed placement", err)
	}
}

func TestManagedNativeRefusesChangedReservationAndPrivateResponse(t *testing.T) {
	for _, change := range []string{"hostname", "missing-reservation", "private-response"} {
		t.Run(change, func(t *testing.T) {
			s, owner, _, op, a := nativeIntentFixture(t)
			ctx := context.Background()
			p := AgentProvisioner{Pool: s.Pool, Client: a, ManagedRuntime: true}
			switch change {
			case "hostname":
				if _, err := s.Pool.Exec(ctx, `UPDATE deployments SET managed_hostname=id::text||'.localhost' WHERE id=$1`, op.ID); err != nil {
					t.Fatal(err)
				}
			case "missing-reservation":
				if _, err := s.Pool.Exec(ctx, `DELETE FROM managed_public_endpoints WHERE deployment_id=$1`, op.ID); err != nil {
					t.Fatal(err)
				}
			case "private-response":
				if err := p.Step(ctx, op.ID); err != nil {
					t.Fatal(err)
				}
				a.wrongEndpoint = true
			}
			if err := p.Step(ctx, op.ID); err == nil {
				t.Fatal("changed identity accepted")
			}
			retained, err := s.Get(ctx, owner, op.ID)
			if err != nil || retained.Endpoint != "" || retained.State == "ready" || a.privateCalls != 0 {
				t.Fatal("refusal fell back or activated URL", err)
			}
			if change != "private-response" && a.managedCalls != 0 {
				t.Fatal("unbound intent reached host")
			}
		})
	}
}

func TestManagedNativeLateHoldRetainsUncertainHostSuccess(t *testing.T) {
	s, owner, _, op, a := nativeIntentFixture(t)
	ctx := context.Background()
	p := AgentProvisioner{Pool: s.Pool, Client: a, ManagedRuntime: true}
	if err := p.Step(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	a.onEnsure = func() {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
			t.Error(err)
		}
	}
	a.failAfterCreate = true
	worker := Worker{Pool: s.Pool, Provisioner: p}
	job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: Args{op.ID}}
	var snooze *river.JobSnoozeError
	if err := worker.Work(ctx, job); !errors.As(err, &snooze) {
		t.Fatal("late hold exhausted uncertain native request", err)
	}
	retained, err := s.Get(ctx, owner, op.ID)
	if err != nil || retained.State != "configuring" || retained.Endpoint != "" || len(retained.Events) != 2 || len(a.ids) != 1 {
		t.Fatal("late hold checkpointed native success", err)
	}
	a.onEnsure = nil
	if _, err := s.Pool.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now()`); err != nil {
		t.Fatal(err)
	}
	if err := p.Step(ctx, op.ID); err != nil {
		t.Fatal("reconciled native replay failed", err)
	}
	if len(a.ids) != 1 || a.privateCalls != 0 {
		t.Fatal("recovery duplicated native identity")
	}
}

func TestManagedNativeUnhealthyTimeoutRetainsEndpointReservation(t *testing.T) {
	s, owner, _, op, a := nativeIntentFixture(t)
	ctx := context.Background()
	p := AgentProvisioner{Pool: s.Pool, Client: a, ManagedRuntime: true}
	for range 2 {
		if err := p.Step(ctx, op.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE deployments SET updated_at=now()-interval '3 minutes' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Pool: s.Pool, Provisioner: p}
	job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: Args{op.ID}}
	if err := worker.Work(ctx, job); err == nil {
		t.Fatal("native health timeout was accepted")
	}
	retained, err := s.Get(ctx, owner, op.ID)
	if err != nil || retained.State != "failed" || retained.Endpoint != "" || retained.ManagedHostname != op.ManagedHostname || len(a.ids) != 1 {
		t.Fatal("failure lost retained identity or advertised serving", err)
	}
	var reservations int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM managed_public_endpoints WHERE deployment_id=$1`, op.ID).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatal("native failure removed public reservation", err)
	}
}
