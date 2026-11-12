#!/usr/bin/env bash
set -euo pipefail
image=${1:?image is required}
revision=${2:?revision is required}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]]
fixture=$(docker create --rm --network none --read-only \
  --tmpfs /config --tmpfs /data --memory 128m --cpus 0.25 \
  --cap-drop ALL --cap-add NET_BIND_SERVICE --security-opt no-new-privileges:true \
  --entrypoint /bin/sh "$image" -c \
  'exec timeout -s TERM 60 caddy run --config /etc/caddy/Caddyfile --adapter caddyfile')
trap 'docker rm -f "$fixture" >/dev/null 2>&1 || true' EXIT
docker start "$fixture" >/dev/null
for _attempt in {1..30}; do
  if docker exec "$fixture" wget -q -T 3 -O /dev/null \
    --header 'Host: prichal.subcult.tv' http://127.0.0.1/; then
    break
  fi
  sleep 0.2
done
node --input-type=module - "$fixture" "$image" "$revision" <<'NODE'
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
const [fixture, image, revision] = process.argv.slice(2);
const inspect = JSON.parse(execFileSync("docker", ["image", "inspect", image]))[0];
assert.equal(inspect.Config.Labels["org.opencontainers.image.revision"], revision);
assert.equal(inspect.Config.Labels["org.opencontainers.image.source"], "https://git.subcult.tv/subculture-collective/prichal");
const admitted = JSON.parse(readFileSync("deploy/production/web-release.json"));
assert.equal(inspect.Config.Labels["tv.subcult.prichal.management.revision"], admitted.management_revision);
const read = path => execFileSync("docker", ["exec", fixture, "cat", path]);
const fetch = (host, path) => execFileSync("docker", ["exec", fixture, "wget", "-q", "-T", "3", "-O", "-", "--header", "Host: " + host, "http://127.0.0.1" + path]);
const site = "prichal.subcult.tv";
for (const name of ["index.html", "favicon.ico", "favicon.svg", "apple-touch-icon.png", "og.png", "styles.css", "mark.svg", "robots.txt", "sitemap.xml"]) {
  assert.deepEqual(fetch(site, name === "index.html" ? "/" : "/" + name), read("/srv/site/" + name), name);
}
const html = fetch(site, "/").toString();
assert.match(html, /rel="canonical" href="https:\/\/prichal\.subcult\.tv\/"/);
assert.match(html, /property="og:site_name" content="Prichal"/);
assert.match(html, /name="twitter:card" content="summary_large_image"/);
assert.match(html, /not open for sign-up/);
assert.deepEqual(fetch("prichal-app.subcult.tv", "/"), read("/srv/app/index.html"));
const paths = execFileSync("docker", ["exec", fixture, "find", "/srv/app/assets", "-type", "f"], {encoding: "utf8"}).trim().split("\n");
assert.ok(paths.length > 0);
for (const path of paths) {
  assert.deepEqual(fetch("prichal-app.subcult.tv", path.slice("/srv/app".length)), read(path), path);
}
console.log("PASS: isolated landing metadata/assets and protected-host management shell/assets; no API or managed agent activation");
NODE
