// Retained-account API preservation only. Empty-PDS withdrawal requires a separate designated fixture.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const entries = [
  { owner: 'alice', id: 'bca4cb84-cfb2-4e58-afc6-26c5edad9ade' },
  { owner: 'bob', id: '8d234722-f0ad-47b2-8ef8-10f85d0c13b9' },
];
const digest = value => createHash('sha256').update(value).digest('hex');
function docker(args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000, maxBuffer: 2 * 1024 * 1024 });
  assert.equal(result.status, 0, 'Disposable local inventory unavailable'); return result.stdout.trim();
}
const nativeScript = readFileSync('scripts/native-held-export.cjs', 'utf8');
function inventory() {
  const work = docker(['exec', 'pds-ass-local-db-1', 'psql', '-U', 'pds_local', '-d', 'pds_local', '-At', '-c',
    `SELECT json_build_object('deployments',(SELECT json_agg(row_to_json(d) ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT json_agg(row_to_json(e) ORDER BY id) FROM enrollment_operations e),'events',(SELECT count(*) FROM enrollment_events))`]);
  const custody = entries.map(({ id }) => ({ native: JSON.parse(docker(['exec', 'pds-ass-tenant-' + id, 'node', '-e', nativeScript])),
    containers: JSON.parse(docker(['inspect', 'pds-ass-tenant-' + id, 'pds-ass-edge-' + id, 'pds-ass-enrollment-' + id])).map(v => ({ id: v.Id, running: v.State.Running, startedAt: v.State.StartedAt })),
    files: ['.json', '-tls.pem', '-service.json', '-account.json', '-enrollment/lease.json'].map(suffix => digest(readFileSync('.local/node-agent/state/' + id + suffix))) }));
  return { work: digest(work), custody: digest(JSON.stringify(custody)) };
}
const readiness = await fetch('http://localhost:5173/api/readiness'); assert.equal(readiness.status, 200);
assert.equal((await readiness.json()).schemaVersion, 29);
const before = inventory();
const clients = [];
for (const entry of entries) {
  const client = await localOwnerClient(entry.owner);
  const latest = await client.api('/api/deployments/' + entry.id + '/enrollment');
  assert.equal(latest.status, 200); assert.equal(latest.value.state, 'verified');
  assert.equal(latest.value.withdrawalRequested, false); assert.equal(latest.value.withdrawalAttempts, 0);
  assert.equal(latest.value.code, undefined);
  const path = '/api/enrollments/' + latest.value.id + '/withdraw';
  const denied = await client.call(path, { method: 'POST', body: '{}', headers: { Origin: 'http://localhost:5173', 'Content-Type': 'application/json', 'X-CSRF-Token': 'invalid' } });
  assert.equal(denied.status, 403);
  const malformed = await client.api(path, { action: 'delete' }); assert.equal(malformed.status, 400);
  for (let attempt = 0; attempt < 2; attempt++) {
    const result = await client.api(path, {});
    assert.equal(result.status, 200); assert.equal(result.value.state, 'verified');
    assert.equal(result.value.withdrawalRequested, false); assert.equal(result.value.withdrawalAttempts, 0);
    assert.equal(result.value.code, undefined); assert.equal(result.value.id, latest.value.id);
  }
  clients.push({ client, path });
}
assert.equal((await clients[0].client.api(clients[1].path, {})).status, 404);
assert.equal((await clients[1].client.api(clients[0].path, {})).status, 404);
assert.deepEqual(inventory(), before, 'Retained enrollment/work or native custody changed');
const run = randomUUID(), directory = '.local/enrollment-withdrawal-intent/' + run;
mkdirSync(directory, { recursive: true, mode: 0o700 });
const report = { run, schemaVersion: 29, existingAccountsPreserved: 2, repeatedRequests: 4, scopeAndCSRFEnforced: true, unsupportedAuthorityDenied: true,
  sourceWorkAndNativeUnchanged: true, emptyPDSWithdrawalQualified: false, completedAt: new Date().toISOString() };
writeFileSync(directory + '/report.json', JSON.stringify(report, null, 2) + '\n', { mode: 0o600, flag: 'wx' }); console.log(JSON.stringify(report));
