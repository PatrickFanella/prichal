// Restore the fixed local management database into a new, held qualification copy.
import assert from 'node:assert/strict';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdirSync, lstatSync, openSync, closeSync, readFileSync, writeFileSync, unlinkSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { sealRecovery, openRecovery } from './recovery-crypto.mjs';
const id = randomUUID();
const database = `pds_recovery_${id.replaceAll('-', '')}`;
const work = resolve('.local/management-recovery', id);
const keys = resolve('.local/recovery-keys');
const bundles = resolve('.local/recovery-bundles');
for (const directory of [work, keys, bundles]) {
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const info = lstatSync(directory); assert.ok(info.isDirectory() && !(info.mode & 0o077), 'Private recovery directory required');
}
function command(program, args, options = {}) {
  const result = spawnSync(program, args, { encoding: 'utf8', timeout: 60000, maxBuffer: 1024 * 1024, ...options });
  assert.equal(result.status, 0, 'Management recovery subprocess failed');
  return result.stdout;
}
function docker(...args) { return command('docker', args); }
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'), 'Local Docker required');
const container = JSON.parse(docker('inspect', 'pds-ass-local-db-1'))[0];
assert.equal(container.Config.Labels['com.docker.compose.project'], 'pds-ass-local');
assert.equal(container.Config.Labels['com.docker.compose.service'], 'db');
const source = new URL(process.env.PDS_DATABASE_URL);
assert.equal(source.hostname, '127.0.0.1'); assert.equal(source.port, '35433'); assert.equal(source.pathname, '/pds_local');
function sql(target, query) { return docker('exec', 'pds-ass-local-db-1', 'psql', '-U', 'pds_local', '-d', target, '-At', '-v', 'ON_ERROR_STOP=1', '-c', query).trim(); }
const tables = ['owners', 'organizations', 'deployments', 'deployment_events', 'instance_placements', 'hosting_nodes'];
const inventoryQuery = `SELECT json_build_object(${tables.map(table => `'${table}',json_build_object('count',(SELECT count(*) FROM ${table}),'sha256',(SELECT encode(sha256(convert_to(coalesce(string_agg(row_to_json(snapshot)::text, E'\\n' ORDER BY row_to_json(snapshot)::text),''),'UTF8')),'hex') FROM ${table} snapshot))`).join(',')})`;

const before = JSON.parse(sql('pds_local', inventoryQuery));
const dump = join(work, 'management.dump');
let fd = openSync(dump, 'wx', 0o600);
try { command('docker', ['exec', 'pds-ass-local-db-1', 'pg_dump', '-U', 'pds_local', '-Fc', 'pds_local'], { stdio: ['ignore', fd, 'pipe'] }); } finally { closeSync(fd); }
const digest = createHash('sha256').update(readFileSync(dump)).digest('hex');
const key = join(keys, `${id}-management.key`);
writeFileSync(key, randomBytes(32), { mode: 0o600, flag: 'wx' });
const bundle = join(bundles, `${id}-management.pdsbk`);
await sealRecovery(dump, bundle, key, { version: 1, id, scope: 'management-local', createdAt: new Date().toISOString() });
const authenticated = join(work, 'authenticated.dump');
await openRecovery(bundle, authenticated, key);
assert.equal(createHash('sha256').update(readFileSync(authenticated)).digest('hex'), digest);
unlinkSync(dump);
// The database identifier is generated above, never supplied by a caller.
sql('pds_local', `CREATE DATABASE ${database}`);
fd = openSync(authenticated, 'r');
try { command('docker', ['exec', '-i', 'pds-ass-local-db-1', 'pg_restore', '-U', 'pds_local', '--exit-on-error', '--no-owner', '-d', database], { stdio: [fd, 'pipe', 'pipe'] }); } finally { closeSync(fd); }
source.pathname = `/${database}`;
const environment = { ...process.env, PDS_MODE: 'local', PDS_DATABASE_URL: source.toString(), PDS_RECOVERY_SNAPSHOT_SHA256: digest };
command('go', ['run', './cmd/management-recovery', 'hold'], { cwd: resolve('backend'), env: environment });
assert.deepEqual(JSON.parse(sql(database, inventoryQuery)), before);
assert.equal(sql(database, `SELECT state FROM management_recovery`), 'held');
const check = spawnSync('go', ['run', './cmd/management-recovery', 'check'], { cwd: resolve('backend'), env: environment, encoding: 'utf8', timeout: 60000 });
assert.notEqual(check.status, 0, 'Restored database must refuse execution before reconciliation');
const worker = spawnSync('go', ['run', './cmd/worker'], { cwd: resolve('backend'), env: environment, encoding: 'utf8', timeout: 30000 });
assert.notEqual(worker.status, 0, 'Restored worker started before reconciliation');
assert.ok(worker.stderr.includes('management recovery requires host reconciliation'), 'Worker must stop for the recovery hold');
assert.deepEqual(JSON.parse(sql(database, inventoryQuery)), before);
assert.deepEqual(JSON.parse(sql('pds_local', inventoryQuery)), before);
unlinkSync(authenticated);
const report = { id, database, snapshotSha256: digest, bundle, recoveryKeyFile: key, inventory: before, state: 'held', workerRefused: true, sourceInventoryUnchanged: true, completedAt: new Date().toISOString() };
writeFileSync(join(work, 'report.json'), JSON.stringify(report, null, 2)+'\n', { mode: 0o600, flag: 'wx' });
console.log(JSON.stringify({ database, inventory: Object.fromEntries(Object.entries(before).map(([table,value]) => [table,value.count])), state: 'held', workerRefused: true, sourceInventoryUnchanged: true }));
