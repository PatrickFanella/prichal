// Fixed retained-account checks. This drill refuses an empty PDS and never cancels an invitation.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync, lstatSync } from 'node:fs';
import { request } from 'node:https';
import { spawnSync } from 'node:child_process';

const instances = ['bca4cb84-cfb2-4e58-afc6-26c5edad9ade', '8d234722-f0ad-47b2-8ef8-10f85d0c13b9'];
const nativeScript = readFileSync('scripts/native-held-export.cjs', 'utf8');
const digest = value => createHash('sha256').update(value).digest('hex');
function docker(args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 30000, maxBuffer: 2 * 1024 * 1024 });
  assert.equal(result.status, 0, 'Retained local inventory unavailable');
  return result.stdout.trim();
}
function inventory() {
  const management = docker(['exec', 'pds-ass-local-db-1', 'psql', '-U', 'pds_local', '-d', 'pds_local', '-At', '-c',
    `SELECT json_build_object('deployments',(SELECT json_agg(row_to_json(d) ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT json_agg(row_to_json(e) ORDER BY id) FROM enrollment_operations e),'events',(SELECT count(*) FROM enrollment_events))`]);
  const custody = instances.map(id => {
    const native = JSON.parse(docker(['exec', 'pds-ass-tenant-' + id, 'node', '-e', nativeScript]));
    const containers = JSON.parse(docker(['inspect', 'pds-ass-tenant-' + id, 'pds-ass-edge-' + id, 'pds-ass-enrollment-' + id]))
      .map(value => ({ id: value.Id, running: value.State.Running, startedAt: value.State.StartedAt }));
    assert.ok(containers.every(value => value.running), 'Retained local PDS must be running');
    return { native, containers, files: ['.json', '-tls.pem', '-service.json', '-account.json', '-enrollment/lease.json'].map(suffix => digest(readFileSync('.local/node-agent/state/' + id + suffix))) };
  });
  return { management: digest(management), custody: digest(JSON.stringify(custody)) };
}
const pki = '.local/node-agent/certs/';
const tls = { ca: readFileSync(pki + 'ca.crt'), cert: readFileSync(pki + 'worker.crt'), key: readFileSync(pki + 'worker.key') };
async function withdraw(id, ticket, extra = {}) {
  return new Promise((done, reject) => {
    const body = JSON.stringify({ ticket, ...extra });
    const req = request({ ...tls, hostname: '127.0.0.1', port: 9091, method: 'POST', path: '/v1/instances/' + id + '/enrollment/withdraw', agent: false,
      headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) }, timeout: 30000 }, response => {
      let text = '';
      response.on('data', value => { text += value; if (text.length > 4096) req.destroy(new Error('Bounded agent response required')); });
      response.on('end', () => done({ status: response.statusCode, body: text }));
    });
    req.on('timeout', () => req.destroy(new Error('Local withdrawal check timed out')));
    req.on('error', reject); req.end(body);
  });
}
const before = inventory();
for (const id of instances) {
  const lease = JSON.parse(readFileSync('.local/node-agent/state/' + id + '-enrollment/lease.json', 'utf8'));
  assert.equal(lease.state, 'complete', 'This drill requires a retained completed account');
  assert.equal(lease.instanceId, id);
  assert.match(lease.ticket, /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
  assert.equal((await withdraw(id, lease.ticket, { action: 'delete' })).status, 400);
  assert.equal((await withdraw(id, randomUUID())).status, 503);
  for (let attempt = 0; attempt < 2; attempt++) {
    const response = await withdraw(id, lease.ticket);
    assert.equal(response.status, 200, 'Existing account preservation was not confirmed');
    const result = JSON.parse(response.body);
    assert.equal(result.instanceId, id); assert.equal(result.ticket, lease.ticket);
    assert.equal(result.state, 'verified'); assert.equal(result.emailConfirmed, true);
    assert.equal(result.code, undefined); assert.ok(!result.expiresAt);
    assert.match(result.did, /^did:plc:[a-z2-7]{24}$/);
    const credentials = JSON.parse(readFileSync('.local/node-agent/state/' + id + '.json', 'utf8'));
    for (const secret of Object.values(credentials)) assert.ok(!response.body.includes(secret), 'Native credential escaped the agent');
  }
}
assert.deepEqual(inventory(), before, 'Retained native or management state changed');
const run = randomUUID(), directory = '.local/enrollment-withdrawal/' + run;
mkdirSync(directory, { recursive: true, mode: 0o700 }); assert.ok(!(lstatSync(directory).mode & 0o077));
const report = { run, retainedAccounts: 2, repeatedStatusChecks: 4, foreignTicketDenied: true, unsupportedAuthorityDenied: true,
  nativeAndManagementUnchanged: true, successfulEmptyPDSWithdrawalQualified: false, completedAt: new Date().toISOString() };
writeFileSync(directory + '/report.json', JSON.stringify(report, null, 2) + '\n', { mode: 0o600, flag: 'wx' });
console.log(JSON.stringify(report));
