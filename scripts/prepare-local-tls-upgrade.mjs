// Preserves rollback containers and private inspection snapshots for one empty local fixture.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync, renameSync, existsSync, lstatSync } from 'node:fs';
const [id] = process.argv.slice(2);
assert.match(id ?? '', /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000 });
  assert.equal(result.status, 0, 'Local TLS upgrade Docker operation');
  return result.stdout;
}
const tenant = `pds-ass-tenant-${id}`;
const edge = `pds-ass-edge-${id}`;
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'), 'Local Docker socket required');
const snapshots = JSON.parse(docker('inspect', tenant, edge));
assert.equal(snapshots.length, 2);
for (const snapshot of snapshots) {
  assert.equal(snapshot.Config.Labels['tv.subcult.pds-ass.instance'], id, 'Foreign runtime');
  for (const bindings of Object.values(snapshot.HostConfig.PortBindings ?? {})) {
    for (const binding of bindings ?? []) assert.equal(binding.HostIp, '127.0.0.1', 'Nonlocal runtime');
  }
}
const runtime = snapshots.find(snapshot => snapshot.Name === `/${tenant}`);
assert.ok(runtime.Config.Env.includes('PDS_DEV_MODE=true') && runtime.Config.Env.includes('PDS_DID_PLC_URL=http://127.0.0.1:9') && runtime.Config.Env.includes('PDS_SERVICE_HANDLE_DOMAINS=.test'), 'Isolated identity-disabled fixture required');
const count = docker('exec', tenant, 'node', '-e', "const{DatabaseSync}=require('node:sqlite');const db=new DatabaseSync('/pds/account.sqlite',{readOnly:true});process.stdout.write(String(db.prepare('SELECT count(*) AS count FROM actor').get().count))");
assert.equal(count, '0', 'Populated identity cannot use this endpoint correction');
const directory = `.local/local-tls-upgrades/${id}`;
mkdirSync(directory, { recursive: true, mode: 0o700 });
assert.ok(lstatSync(directory).isDirectory() && (lstatSync(directory).mode & 0o077) === 0, 'Private rollback directory required');
writeFileSync(`${directory}/containers.json`, JSON.stringify(snapshots), { flag: 'wx', mode: 0o600 });
// Stop ingress first, then its only writer. No container, volume or credential is deleted.
docker('stop', edge, tenant);
docker('rename', edge, `${edge}-before-local-tls`);
docker('rename', tenant, `${tenant}-before-local-tls`);
const configuration = `.local/node-agent/state/${id}-edge/Caddyfile`;
assert.ok(existsSync(configuration));
renameSync(configuration, `${configuration}.before-local-tls`);
console.log('Empty local fixture stopped and archived. Storage and credentials retained; agent reconciliation can create TLS runtimes.');
