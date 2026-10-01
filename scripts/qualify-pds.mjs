// Qualifies only the fixed local fixture. No public identity or user account is created.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';

function command(program, args) {
  const result = spawnSync(program, args, { encoding: 'utf8', timeout: 60000 });
  if (result.status !== 0) throw new Error(`${program} qualification command failed: ${result.stderr}`);
  return result.stdout.trim();
}
function compose(...args) { return command('docker', ['compose', '-f', 'deploy/local/compose.yaml', ...args]); }
function checks() { console.log(command(process.execPath, ['scripts/check-pds.mjs'])); }
function inPds(source, ...args) { return command('docker', ['exec', 'pds-ass-runtime-pds-1', 'node', '-e', source, ...args]); }
const env = Object.fromEntries(readFileSync('.local/pds.env', 'utf8').trim().split('\n').map(line => {
  const separator = line.indexOf('='); return [line.slice(0, separator), line.slice(separator + 1)];
}));
assert.equal(typeof env.PDS_ADMIN_PASSWORD, 'string');
const auth = `Basic ${Buffer.from(`admin:${env.PDS_ADMIN_PASSWORD}`).toString('base64')}`;
async function admin(path, body) {
  const response = await fetch(`http://localhost:2583/xrpc/${path}`, {
    headers: { Authorization: auth, 'Content-Type': 'application/json' },
    method: body ? 'POST' : 'GET',
    ...(body ? { body: JSON.stringify(body) } : {}),
    signal: AbortSignal.timeout(5000),
  });
  assert.equal(response.status, 200, 'PDS admin protocol check failed');
  return response.json();
}
checks();
const invite = await admin('com.atproto.server.createInviteCode', { useCount: 1 });
assert.equal(typeof invite.code, 'string');
const marker = `.qualification-${randomUUID()}`;
const content = randomUUID();
inPds("require('fs').writeFileSync('/blobs/'+process.argv[1],process.argv[2],{flag:'wx'})", marker, content);
const integrity = `const {DatabaseSync}=require('node:sqlite');for(const file of ['account.sqlite','did_cache.sqlite','sequencer.sqlite']){const db=new DatabaseSync('/pds/'+file,{readOnly:true});if(db.prepare('PRAGMA integrity_check').get().integrity_check!=='ok')process.exit(1);db.close()}console.log('SQLite integrity passed')`;
try {
  console.log(inPds(integrity));
  compose('restart', 'pds');
  checks();
  const codes = await admin('com.atproto.admin.getInviteCodes?sort=recent&limit=100');
  assert.ok(codes.codes.some(code => code.code === invite.code && code.available === 1), 'One-use invitation did not persist');
  const persisted = inPds("process.stdout.write(require('fs').readFileSync('/blobs/'+process.argv[1],'utf8'))", marker);
  assert.equal(persisted, content, 'Blob volume fixture did not persist');
  console.log(inPds(integrity));
  compose('stop', 'pds');
  try {
    const unavailable = await fetch('http://localhost:2583/xrpc/_health', { signal: AbortSignal.timeout(5000) });
    assert.equal(unavailable.status, 502, 'Ingress reported a stopped PDS as healthy');
  } finally { compose('start', 'pds'); }
  checks();
  console.log('Invitation and blob-volume fixture persisted; SQLite integrity and stopped-runtime failure checks passed.');
} finally {
  inPds("require('fs').unlinkSync('/blobs/'+process.argv[1])", marker);
}
