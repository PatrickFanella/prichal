#!/usr/bin/env bash
set -euo pipefail

rules_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy/homelab" && pwd)
rule_image=prom/prometheus@sha256:3c42b892cf723fa54d2f262c37a0e1f80aa8c8ddb1da7b9b0df9455a35a7f893
rule_container=$(docker create --network none --memory 256m --cpus 1 --pids-limit 128 \
  --cap-drop ALL --security-opt no-new-privileges --entrypoint /bin/promtool \
  "$rule_image" test rules /tmp/public-pds-certificate-alerts.test.yml /tmp/staging-02-certificate-alerts.test.yml)
cleanup() {
  rule_result=$?
  docker rm -f "$rule_container" >/dev/null 2>&1 || true
  exit "$rule_result"
}
trap cleanup EXIT
docker cp "$rules_root/public-pds-certificate-alerts.yml" "$rule_container:/tmp/"
docker cp "$rules_root/public-pds-certificate-alerts.test.yml" "$rule_container:/tmp/"
docker cp "$rules_root/staging-02-certificate-alerts.yml" "$rule_container:/tmp/"
docker cp "$rules_root/staging-02-certificate-alerts.test.yml" "$rule_container:/tmp/"
docker start -a "$rule_container"
rule_exit_code=$(docker wait "$rule_container")
[[ "$rule_exit_code" =~ ^[0-9]+$ ]] || exit 1
exit "$rule_exit_code"
