// Retain only the pinned pilot's closed ingress before opting into first-account leases.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { lstatSync, readFileSync, writeFileSync, realpathSync, renameSync, existsSync } from 'node:fs';
import { createHash } from 'node:crypto';
import path from 'node:path';
const [directory, policyFile] = process.argv.slice(2);
assert.ok(path.isAbsolute(directory ?? '') && path.isAbsolute(policyFile ?? ''), 'Absolute private state and policy paths required');
const state = realpathSync(directory);
assert.equal(state, directory, 'Canonical state path required');
for (const file of [state, policyFile]) assert.ok(!(lstatSync(file).mode & 0o077), 'Private host paths required');
assert.ok(lstatSync(state).isDirectory() && lstatSync(policyFile).isFile());
const policy = JSON.parse(readFileSync(policyFile, 'utf8'));
const id = policy.instanceId;
assert.match(id ?? '', /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
assert.ok(policy.hostname !== 'pds.subcult.tv' && policy.smtpUrl?.startsWith('smtps://'));
const canonical = JSON.stringify({ instanceId: id, hostname: policy.hostname, emailFrom: policy.emailFrom, smtpUrl: policy.smtpUrl });
assert.equal(readFileSync(path.join(state, 'pilot-policy.sha256'), 'utf8'), createHash('sha256').update(canonical).digest('hex') + '\n');
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024 });
  assert.equal(result.status, 0, 'Pilot ingress Docker operation failed');
  return result.stdout;
}
function absent(name) {
  const result = spawnSync('docker', ['inspect', name], { encoding: 'utf8', timeout: 30000 });
  assert.equal(result.status, 1, 'Retained container already exists or inspection failed');
  assert.match(result.stderr, /No such (object|container)/i);
}
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const tenant = JSON.parse(docker('inspect', `pds-ass-tenant-${id}`))[0];
assert.equal(tenant.Config.Labels['tv.subcult.pds-ass.instance'], id);
assert.equal(tenant.Config.Image, 'ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336');
for (const value of ['PDS_DEV_MODE=false', 'PDS_DID_PLC_URL=https://plc.directory', `PDS_HOSTNAME=${policy.hostname}`, `PDS_SERVICE_DID=did:web:${policy.hostname}`]) {
  assert.equal(tenant.Config.Env.filter(entry => entry.split('=')[0] === value.split('=')[0]).join(), value);
}
assert.equal(Object.keys(tenant.HostConfig.PortBindings ?? {}).length, 0, 'Native PDS must have no published port');
const name = `pds-ass-edge-${id}`;
const edge = JSON.parse(docker('inspect', name))[0];
assert.equal(edge.Config.Labels['tv.subcult.pds-ass.instance'], id);
assert.equal(edge.Config.Labels['tv.subcult.pds-ass.role'], 'ingress');
assert.equal(edge.Config.Image, 'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d');
assert.equal(edge.Config.User, `${process.getuid()}:${process.getgid()}`);
assert.equal(edge.HostConfig.Privileged, false);
assert.deepEqual(edge.HostConfig.PortBindings, { '8443/tcp': [{ HostIp: '127.0.0.1', HostPort: String(36000 + parseInt(id.slice(0, 4), 16) % 10000) }] });
const config = path.join(state, `${id}-edge`, 'Caddyfile');
const tls = path.join(state, `${id}-tls.pem`);
assert.equal(edge.Mounts.length, 2);
for (const [destination, source] of [['/etc/caddy/Caddyfile', config], ['/etc/pds-tls.pem', tls]]) {
  assert.ok(edge.Mounts.some(mount => mount.Destination === destination && mount.Source === source && mount.Type === 'bind' && !mount.RW));
  assert.ok(lstatSync(source).isFile() && !(lstatSync(source).mode & 0o077));
}
const closed = readFileSync(config, 'utf8');
assert.ok(closed.includes('respond @registration "Pilot account creation is closed" 403') && !closed.includes('forward_auth'));
assert.ok(closed.includes(`@allowed host ${id}.localhost ${policy.hostname}`));
assert.ok(!existsSync(config + '.before-pilot-enrollment'));
absent(name + '-before-pilot-enrollment');
absent(`pds-ass-enrollment-${id}`);
// Keep a private inspection receipt without copying the PDS environment or administrator key.
writeFileSync(path.join(state, 'pilot-enrollment-edge-before.json'), JSON.stringify(edge), { mode: 0o600, flag: 'wx' });
docker('stop', name);
docker('rename', name, name + '-before-pilot-enrollment');
renameSync(config, config + '.before-pilot-enrollment');
console.log('Closed pilot ingress retained. Enable the host opt-in and reconcile the same UUID; the PDS and its volumes are unchanged.');
