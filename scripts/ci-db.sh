#!/usr/bin/env bash
# A Docker-backed Gitea runner starts one tmpfs-backed database for this job.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .local
case "${1:-}" in
  start)
    : "${GITHUB_ENV:?This helper is for a CI job}"
    test ! -e .local/ci-db.id
    docker run --detach --rm --cidfile .local/ci-db.id \
      --network bridge --label pds-ass.disposable-ci=true \
      --tmpfs /var/lib/postgresql/data:rw,size=256m \
      --memory 512m --cpus 1 \
      -e POSTGRES_USER=pds_ci -e POSTGRES_DB=pds_ci -e POSTGRES_PASSWORD=disposable-ci-only \
      postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73
    container_id=$(cat .local/ci-db.id)
    for attempt in {1..30}; do
      if docker exec "$container_id" pg_isready -U pds_ci -d pds_ci >/dev/null 2>&1; then break; fi
      if [[ "$attempt" == 30 ]]; then printf 'CI PostgreSQL did not become ready\n' >&2; exit 1; fi
      sleep 1
    done
    database_host=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container_id")
    test -n "$database_host"
    printf 'PDS_DATABASE_URL=postgres://pds_ci:disposable-ci-only@%s:5432/pds_ci?sslmode=disable\n' "$database_host" >> "$GITHUB_ENV"
    ;;
  stop)
    if [[ -f .local/ci-db.id ]]; then
      container_id=$(cat .local/ci-db.id)
      [[ "$container_id" =~ ^[a-f0-9]{64}$ ]]
      if docker inspect "$container_id" >/dev/null 2>&1; then
        label=$(docker inspect --format '{{index .Config.Labels "pds-ass.disposable-ci"}}' "$container_id")
        [[ "$label" == true ]]
        docker stop "$container_id"
      fi
    fi
    ;;
  *) printf 'Usage: scripts/ci-db.sh {start|stop}\n' >&2; exit 2 ;;
esac
