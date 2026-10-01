package lifecycle

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// InsertExitTx creates only an exit-owned service fence in the caller's atomic transition.
// Stop requires an applied running service; resume requires that same exit's exact applied stop.
// Native account moderation is never modified. Ordinary operator Submit cannot select this reason.
func (s *Store) InsertExitTx(ctx context.Context, tx pgx.Tx, exitID, deployment, key, expectedStop string) (string, error) {
	if !uuid.MatchString(exitID) || !uuid.MatchString(deployment) || !uuid.MatchString(key) || (expectedStop != "" && !uuid.MatchString(expectedStop)) {
		return "", ErrConflict
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return "", err
	}
	if held {
		return "", recovery.ErrHeld
	}
	var state, kind string
	if err := tx.QueryRow(ctx, `SELECT state,kind FROM deployments WHERE id=$1 FOR UPDATE`, deployment).Scan(&state, &kind); err != nil {
		return "", err
	}
	if state != "ready" || kind != "agent" {
		return "", ErrConflict
	}
	var bound bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hosting_exits WHERE id=$1 AND deployment_id=$2 AND (($3='' AND state IN ('scheduled','stopping')) OR ($3<>'' AND state IN ('ended','resuming') AND stop_operation_id::text=$3)))`, exitID, deployment, expectedStop).Scan(&bound); err != nil {
		return "", err
	}
	if !bound {
		return "", ErrConflict
	}
	actor := "hosting-exit:" + exitID
	desired := "suspended"
	prior := "running"
	if expectedStop != "" {
		desired = "running"
		prior = "suspended"
	}
	var id, previousActor, previousState, previousKind, reason string
	err := tx.QueryRow(ctx, `SELECT id,actor,desired_state,kind,reason FROM service_operations WHERE deployment_id=$1 AND idempotency_key=$2`, deployment, key).Scan(&id, &previousActor, &previousState, &previousKind, &reason)
	if err == nil {
		if previousActor != actor || previousState != desired || previousKind != "service" || reason != "customer_exit" {
			return "", ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var generation int64
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(generation),0),coalesce(bool_or(state='pending'),false) FROM service_operations WHERE deployment_id=$1`, deployment).Scan(&generation, &pending); err != nil {
		return "", err
	}
	if pending || generation >= 1<<53 {
		return "", ErrConflict
	}
	var currentID, currentPhase, currentDesired, currentReason, currentActor string
	err = tx.QueryRow(ctx, `SELECT id,state,desired_state,reason,actor FROM service_operations WHERE deployment_id=$1 AND kind='service' ORDER BY generation DESC LIMIT 1`, deployment).Scan(&currentID, &currentPhase, &currentDesired, &currentReason, &currentActor)
	if expectedStop == "" {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if err == nil && (currentPhase != "applied" || currentDesired != "running") {
			return "", ErrConflict
		}
	} else if err != nil || currentID != expectedStop || currentPhase != "applied" || currentDesired != "suspended" || currentReason != "customer_exit" || currentActor != actor {
		return "", ErrConflict
	}
	if err = tx.QueryRow(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,desired_state,prior_state,reason,actor,kind) VALUES($1,$2,$3,$4,$5,'customer_exit',$6,'service') RETURNING id`, deployment, key, generation+1, desired, prior, actor).Scan(&id); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_operation_events(operation_id,state) VALUES($1,'pending')`, id); err != nil {
		return "", err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return "", err
	}
	return id, nil
}
