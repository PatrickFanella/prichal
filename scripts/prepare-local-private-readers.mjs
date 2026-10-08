// Retain local edge/guard containers and policy before upgrading their file-reader identity.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync, lstatSync, realpathSync, renameSync, existsSync } from 'node:fs';
import path from 'node:path';
const [id] = process.argv.slice(2);
assert.match(id ?? '', /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024 });
  assert.equal(result.status, 0, 'Local private-reader Docker operation');
  return result.stdout;
}
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const tenant = JSON.parse(docker('inspect', `pds-ass-tenant-${id}`))[0];
assert.equal(tenant.Config.Labels['tv.subcult.pds-ass.instance'], id);
assert.ok(tenant.Config.Image.startsWith('ghcr.io/bluesky-social/pds:0.4@sha256:'));
assert.ok(tenant.Config.Env.includes('PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582'), 'Only isolated local identities can use this upgrade');
assert.ok(tenant.Config.Env.some(value => value.startsWith(`PDS_HOSTNAME=${id}.localhost:`)), 'Local endpoint required');
const state = realpathSync('.local/node-agent/state');
const config = path.join(state, `${id}-edge`, 'Caddyfile');
const configInfo = lstatSync(config);
assert.ok(configInfo.isFile() && !(configInfo.mode & 0o077), 'Private regular policy required');
assert.ok(!existsSync(config + '.before-private-readers'), 'Previous policy already retained');
const names = [`pds-ass-edge-${id}`, `pds-ass-enrollment-${id}`];
const containers = names.map(name => JSON.parse(docker('inspect', name))[0]);
for (const [index, container] of containers.entries()) {
  assert.equal(container.Config.Labels['tv.subcult.pds-ass.instance'], id);
  assert.equal(container.Config.Labels['tv.subcult.pds-ass.role'], index === 0 ? 'ingress' : 'enrollment-guard');
  assert.ok(container.Mounts.every(mount => mount.Type === 'bind' && !mount.RW && mount.Source.startsWith(state + path.sep)), 'Unexpected private mount');
  assert.equal(container.HostConfig.Privileged, false);
  const prior = spawnSync('docker', ['inspect', `${names[index]}-before-private-readers`], { encoding: 'utf8', timeout: 30000 });
  assert.equal(prior.status, 1, 'Previous container already retained or inspection unavailable');
  assert.match(prior.stderr, /No such (object|container)/i, 'Previous-container absence must be confirmed');
}
const root = `.local/local-private-reader-upgrades/${id}`;
mkdirSync(root, { recursive: true, mode: 0o700 });
assert.ok(lstatSync(root).isDirectory() && !(lstatSync(root).mode & 0o077));
writeFileSync(root + '/containers.json', JSON.stringify(containers), { mode: 0o600, flag: 'wx' });
for (const name of names) {
  docker('stop', name);
  docker('rename', name, `${name}-before-private-readers`);
}
renameSync(config, config + '.before-private-readers');
console.log('Previous local edge, guard and Caddyfile retained. Restart the agent and reconcile this same instance.');
