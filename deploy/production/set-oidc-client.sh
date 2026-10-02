#!/usr/bin/env bash
# Store the Gitea OAuth client for the hosted preview without echoing the secret, then restart the API.
set -euo pipefail
env_file=/etc/prichal/production.env
[[ -f $env_file && ! -L $env_file ]] || { echo "Missing $env_file" >&2; exit 1; }
read -r -p 'Gitea OAuth client ID: ' client_id
read -r -s -p 'Gitea OAuth client secret: ' client_secret; echo
[[ $client_id =~ ^[A-Za-z0-9-]{8,128}$ ]] || { echo 'Unexpected client ID format' >&2; exit 1; }
[[ ${#client_secret} -ge 16 && $client_secret != *$'\n'* ]] || { echo 'Unexpected client secret' >&2; exit 1; }
umask 077
cp -p "$env_file" "$env_file.before-oidc-$(date -u +%Y%m%dT%H%M%SZ)"
grep -v -E '^PRICHAL_OIDC_CLIENT_(ID|SECRET)=' "$env_file" > "$env_file.new"
printf 'PRICHAL_OIDC_CLIENT_ID=%s\nPRICHAL_OIDC_CLIENT_SECRET=%s\n' "$client_id" "$client_secret" >> "$env_file.new"
chmod 600 "$env_file.new" && mv "$env_file.new" "$env_file"
cd "$(dirname "$0")"
set -a; . "$env_file"; set +a
PRICHAL_REVISION=$(git rev-parse HEAD) docker compose up -d --no-build api
echo 'API restarted with the new client. Sign in at https://prichal-app.subcult.tv'
