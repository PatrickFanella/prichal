// Operator-only qualification broker. Public API code must never import this module.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
export function publishFixtureLease(lease) {
  const source = `const fs=require('fs');const file='/fixture-data/registration-lease.json';const value=fs.readFileSync(0);if(value.length>1024)process.exit(1);const temporary=file+'.'+require('crypto').randomUUID()+'.tmp';const fd=fs.openSync(temporary,'wx',0o600);fs.writeFileSync(fd,value);fs.fsyncSync(fd);fs.closeSync(fd);fs.renameSync(temporary,file);const directory=fs.openSync('/fixture-data','r');fs.fsyncSync(directory);fs.closeSync(directory);`;
  const result = spawnSync('docker', ['exec', '-i', 'pds-ass-runtime-protocol-fixtures-1', 'node', '-e', source], { input: JSON.stringify(lease), encoding: 'utf8', timeout: 5000 });
  assert.equal(result.status, 0, 'Local registration policy publication');
}
export async function prepareFixtureEnrollment(admin) {
  const health = await fetch('http://localhost:2582/_health', { signal: AbortSignal.timeout(5000) });
  assert.equal((await health.json()).localFixture, true, 'Isolated identity fixture required');
  // Fail closed before revoking stale codes, checking limits or creating a new invitation.
  publishFixtureLease({ tenant: 'qualification', issuedAt: 0, expiresAt: 1 });
  async function call(method, body) {
    const response = await fetch(`http://localhost:2583/xrpc/${method}`, {
      method: body === undefined ? 'GET' : 'POST', headers: { Authorization: admin, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(5000),
    });
    assert.equal(response.status, 200, 'Local enrollment administrative operation');
    const text = await response.text(); return text ? JSON.parse(text) : undefined;
  }
  const codes = [];
  let cursor;
  do {
    const page = await call(`com.atproto.admin.getInviteCodes?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`);
    codes.push(...page.codes.filter(code => !code.disabled && code.available > code.uses.length).map(code => code.code));
    cursor = page.cursor;
    assert.ok(codes.length <= 1000, 'Fixture invitation inventory exceeded');
  } while (cursor);
  if (codes.length) await call('com.atproto.admin.disableInviteCodes', { codes });
  let accountCount = 0;
  cursor = undefined;
  do {
    const repos = await call(`com.atproto.sync.listRepos?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`);
    accountCount += repos.repos.length;
    assert.ok(accountCount < 32, 'Local fixture account ceiling reached');
    cursor = repos.repos.length ? repos.cursor : undefined;
  } while (cursor);
  const invitation = await call('com.atproto.server.createInviteCode', { useCount: 1 });
  const issuedAt = Date.now();
  const lease = { tenant: 'qualification', issuedAt, expiresAt: issuedAt + 15 * 60 * 1000 };
  publishFixtureLease(lease);
  return { code: invitation.code, lease };
}
