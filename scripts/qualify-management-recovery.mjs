// Restore the fixed local management database into a new, held qualification copy.
import assert from 'node:assert/strict';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdirSync, lstatSync, openSync, closeSync, readFileSync, writeFileSync, unlinkSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { sealRecovery, openRecovery } from './recovery-crypto.mjs';
import {managementInventorySQL,managementSequenceSQL} from './management-inventory.mjs';
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
assert.equal(process.env.PDS_LOCAL_MANAGEMENT_QUIESCED,'true','Stop only this project management API, worker and monitor before stable inventory qualification');
const allTables=sql('pds_local',"SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename").split('\n');allTables.forEach(table=>assert.match(table,/^[a-z][a-z0-9_]{0,62}$/));assert.ok(allTables.includes('service_operations')&&allTables.includes('enrollment_operations')&&allTables.includes('instance_usage'));
const tables=allTables.filter(table=>!['management_recovery','recovery_retained_namespaces'].includes(table));
const durableTables=tables.filter(table=>!['management_sessions','login_attempts'].includes(table));

const inventoryQuery=managementInventorySQL(tables),durableInventoryQuery=managementInventorySQL(durableTables);

const sequences=JSON.parse(sql('pds_local',managementSequenceSQL));
const schemaVersion=Number(sql('pds_local','SELECT version FROM schema_version'));const currentVersion=Number(/const Version = ([0-9]+)/.exec(readFileSync('backend/internal/migrate/migrate.go','utf8'))?.[1]);assert.equal(schemaVersion,currentVersion,'Current local management schema required');

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
assert.deepEqual(JSON.parse(sql(database,inventoryQuery)),before,'Complete restored snapshot differs');
assert.deepEqual(JSON.parse(sql(database,managementSequenceSQL)),sequences,'Restored sequence positions differ');
source.pathname = `/${database}`;
const environment = { ...process.env, PDS_MODE: 'local', PDS_DATABASE_URL: source.toString(), PDS_RECOVERY_SNAPSHOT_SHA256: digest };
command('go', ['run', './cmd/management-recovery', 'hold'], { cwd: resolve('backend'), env: environment });
assert.deepEqual(JSON.parse(sql(database,durableInventoryQuery)),Object.fromEntries(durableTables.map(table=>[table,before[table]])));
assert.equal(sql(database,'SELECT (SELECT count(*) FROM management_sessions)+(SELECT count(*) FROM login_attempts)'),'0');
assert.equal(sql(database, `SELECT state FROM management_recovery`), 'held');
const check = spawnSync('go', ['run', './cmd/management-recovery', 'check'], { cwd: resolve('backend'), env: environment, encoding: 'utf8', timeout: 60000 });
assert.notEqual(check.status, 0, 'Restored database must refuse execution before reconciliation');
const worker = spawnSync('go', ['run', './cmd/worker'], { cwd: resolve('backend'), env: environment, encoding: 'utf8', timeout: 30000 });
assert.notEqual(worker.status, 0, 'Restored worker started before reconciliation');
assert.ok(worker.stderr.includes('management recovery requires host reconciliation'), 'Worker must stop for the recovery hold');
assert.deepEqual(JSON.parse(sql(database,durableInventoryQuery)),Object.fromEntries(durableTables.map(table=>[table,before[table]])));
assert.equal(sql(database,'SELECT (SELECT count(*) FROM management_sessions)+(SELECT count(*) FROM login_attempts)'),'0');
assert.deepEqual(JSON.parse(sql('pds_local', inventoryQuery)), before);
assert.deepEqual(JSON.parse(sql('pds_local',managementSequenceSQL)),sequences,'Source sequence positions changed');
unlinkSync(authenticated);
const report = { id, database, snapshotSha256: digest, bundle, recoveryKeyFile: key, inventory: before, schemaVersion, sequences, tables:tables.length, sequencesVerified:true, copiedAuthenticationRevoked:true, state: 'held', workerRefused: true, sourceInventoryUnchanged: true, completedAt: new Date().toISOString() };
writeFileSync(join(work, 'report.json'), JSON.stringify(report, null, 2)+'\n', { mode: 0o600, flag: 'wx' });
console.log(JSON.stringify({ database, schemaVersion, tables:tables.length,sequencesVerified:true,copiedAuthenticationRevoked:true, inventory: Object.fromEntries(Object.entries(before).map(([table,value]) => [table,value.count])), state: 'held', workerRefused: true, sourceInventoryUnchanged: true }));
