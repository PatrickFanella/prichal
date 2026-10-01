import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

const source = readFileSync(new URL('../backend/internal/nodeagent/enrollment-helper.cjs', import.meta.url), 'utf8');
async function run(action, { accounts = 0, failDisable = false, failInventory = false } = {}) {
  const calls = [];
  let stdout = '', stderr = '', exitCode;
  await runInNewContext(source, {
    process: {
      argv: ['helper', action], env: { PDS_DEV_MODE: 'true', PDS_DID_PLC_URL: 'http://pds-ass-protocol-fixtures:2582', PDS_ADMIN_PASSWORD: 'private-fixture-admin' },
      stdout: { write(value) { stdout += value; } }, stderr: { write(value) { stderr += value; } },
      set exitCode(value) { exitCode = value; },
    }, Buffer, AbortSignal,
    require(name) {
      assert.equal(name, 'node:sqlite');
      return { DatabaseSync: class {
        constructor(path, options) { assert.equal(path, '/pds/account.sqlite'); assert.equal(options.readOnly, true); }
        prepare(query) { assert.equal(query, 'SELECT did FROM actor LIMIT 2'); return { all() { if (failInventory) throw new Error('private-fixture-admin'); return Array.from({ length: accounts }, () => ({ did: 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa' })); } }; }
        close() {}
      } };
    },
    async fetch(url, options) {
      const method = url.split('/xrpc/')[1];
      calls.push({ method, body: options.body ? JSON.parse(options.body) : undefined });
      if (method === 'com.atproto.admin.disableInviteCodes' && failDisable) return { ok: false };
      let value;
      if (method.startsWith('com.atproto.admin.getInviteCodes')) value = { codes: [
        { code: 'unused', disabled: false, available: 1, uses: [] },
        { code: 'disabled', disabled: true, available: 1, uses: [] },
        { code: 'consumed', disabled: false, available: 1, uses: [{}] },
      ] };
      else if (method.startsWith('com.atproto.admin.getAccountInfo')) value = { did: 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa', handle: 'fixture.test', emailConfirmedAt: 'fixture' };
      else if (method === 'com.atproto.server.createInviteCode') value = { code: 'replacement' };
      else assert.equal(method, 'com.atproto.admin.disableInviteCodes');
      return { ok: true, async text() { return value ? JSON.stringify(value) : ''; } };
    },
  });
  assert.ok(!stdout.includes('private-fixture-admin') && !stderr.includes('private-fixture-admin'));
  return { calls, stdout, stderr, exitCode };
}

test('Withdrawal disables unused native codes without issuing replacements', async () => {
  const result = await run('withdraw');
  assert.deepEqual(JSON.parse(result.stdout), { count: 0 });
  assert.equal(result.exitCode, undefined);
  assert.deepEqual(result.calls.map(value => value.method), ['com.atproto.admin.getInviteCodes?limit=100', 'com.atproto.admin.disableInviteCodes']);
  assert.deepEqual(result.calls[1].body, { codes: ['unused'] });
});
test('Withdrawal preserves an existing native account and fails closed on unavailable inventory or revocation', async () => {
  const existing = await run('withdraw', { accounts: 1 });
  assert.equal(JSON.parse(existing.stdout).count, 1);
  assert.equal(existing.calls.length, 1);
  assert.ok(existing.calls[0].method.startsWith('com.atproto.admin.getAccountInfo'));
  for (const options of [{ failInventory: true }, { failDisable: true }, { accounts: 2 }]) {
    const failed = await run('withdraw', options);
    assert.equal(failed.stdout, ''); assert.equal(failed.exitCode, 1);
    assert.equal(failed.stderr, 'Native enrollment operation failed');
    assert.ok(!failed.calls.some(value => value.method === 'com.atproto.server.createInviteCode'));
  }
});
