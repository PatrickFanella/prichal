package deployment

import (
	"context"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type recoveryForbiddenProvisioner struct{ calls int }

func (p *recoveryForbiddenProvisioner) Step(context.Context, string) error { p.calls++; return nil }
func TestHeldWorkerDoesNotTouchHostOrAdvanceDeployment(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	provisioner := &recoveryForbiddenProvisioner{}
	worker := &Worker{Pool: pool, Provisioner: provisioner}
	job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: Args{DeploymentID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}}
	if err := worker.Work(ctx, job); err == nil {
		t.Fatal("held job completed")
	}
	if provisioner.calls != 0 {
		t.Fatal("held job executed host operation")
	}
	if _, err := pool.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now()`); err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	if provisioner.calls != 3 {
		t.Fatal("reconciled job did not resume")
	}
}
