package main

import (
	"context"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
)

type managementProbe struct{ Pool *pgxpool.Pool }

func (p managementProbe) Check(ctx context.Context) error {
	if err := (migrate.Probe{Pool: p.Pool}).Check(ctx); err != nil {
		return err
	}
	return recovery.Check(ctx, p.Pool)
}
func (p managementProbe) Version() int { return migrate.Version }
