#!/usr/bin/env bash
# Only ephemeral smoke databases are created and dropped by the integration test.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${PDS_DATABASE_URL:?Set PDS_DATABASE_URL or use scripts/dev.sh smoke}"
node --test scripts/*.test.mjs
(cd web && pnpm install --frozen-lockfile && pnpm build)
(cd backend && go vet ./... && PDS_TEST_ADMIN_URL="$PDS_DATABASE_URL" go test -race -count=1 ./...)
