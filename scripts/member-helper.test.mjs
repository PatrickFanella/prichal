import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

const source = readFileSync(new URL('../backend/internal/nodeagent/member-helper.cjs', import.meta.url), 'utf8');
const owner = 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa', member = 'did:plc:bbbbbbbbbbbbbbbbbbbbbbbb';
// In-memory invite_code/invite_code_use/actor tables behind the helper's fixed queries.
function nativeStore({ accounts = [owner], codes = {} } = {}) {
  return { accounts: [...accounts], codes: Object.fromEntries(Object.entries(codes).map(([code, value]) => [code, { disabled: false, usedBy: '', ...value }])), minted: 0 };
}
async function run(args, store, { failWrite = false } = {}) {
  const calls = [];
  let stdout = '', stderr = '', exitCode;
  const usable = () => Object.entries(store.codes).filter(([, value]) => !value.disabled && !value.usedBy).map(([code]) => ({ code }));
  await runInNewContext(source, {
    process: {
      argv: ['helper', ...args], env: { PDS_DEV_MODE: 'true', PDS_DID_PLC_URL: 'http://pds-ass-protocol-fixtures:2582', PDS_ADMIN_PASSWORD: 'private-fixture-admin' },
      stdout: { write(value) { stdout += value; } }, stderr: { write(value) { stderr += value; } },
      set exitCode(value) { exitCode = value; },
    }, Buffer, AbortSignal,
    require(name) {
      assert.equal(name, 'node:sqlite');
      return { DatabaseSync: class {
        constructor(path, options) { assert.equal(path, '/pds/account.sqlite'); assert.equal(options.readOnly, true); }
        exec(sql) { assert.equal(sql, 'BEGIN'); }
        prepare(query) {
          if (query === 'SELECT count(*) AS count FROM actor') return { get: () => ({ count: store.accounts.length }) };
          if (query === 'SELECT did FROM actor ORDER BY createdAt,did LIMIT 1') return { get: () => (store.accounts[0] ? { did: store.accounts[0] } : undefined) };
          if (query.startsWith('SELECT c.code FROM invite_code c')) return { all: usable };
          if (query === 'SELECT disabled FROM invite_code WHERE code=?') return { get: code => (store.codes[code] ? { disabled: store.codes[code].disabled ? 1 : 0 } : undefined) };
          if (query.startsWith('SELECT u.usedBy AS did,a.handle AS handle FROM invite_code_use')) return { get: code => (store.codes[code]?.usedBy ? { did: store.codes[code].usedBy, handle: 'member.test' } : undefined) };
          throw new Error('unexpected query ' + query);
        }
        close() {}
      } };
    },
    async fetch(url, options) {
      const method = url.split('/xrpc/')[1], body = JSON.parse(options.body);
      calls.push({ method, body });
      assert.ok(options.headers.Authorization.startsWith('Basic '));
      if (failWrite) return { ok: false };
      if (method === 'com.atproto.admin.disableInviteCodes') { for (const code of body.codes) if (store.codes[code]) store.codes[code].disabled = true; return { ok: true, async text() { return ''; } }; }
      assert.equal(method, 'com.atproto.server.createInviteCode');
      assert.deepEqual(body, { useCount: 1 });
      const code = `minted-${++store.minted}`;
      store.codes[code] = { disabled: false, usedBy: '' };
      return { ok: true, async text() { return JSON.stringify({ code }); } };
    },
  });
  assert.ok(!stdout.includes('private-fixture-admin') && !stderr.includes('private-fixture-admin'));
  return { calls, value: stdout ? JSON.parse(stdout) : undefined, stderr, exitCode };
}

test('Issuance waits for the owner account and never touches first-account codes', async () => {
  const store = nativeStore({ accounts: [], codes: { 'first-account': {} } });
  for (const args of [['issue', '25'], ['sweep']]) {
    const result = await run(args, store);
    assert.equal(result.value.ownerRequired, true);
    assert.deepEqual(result.calls, []);
  }
  assert.equal(store.codes['first-account'].disabled, false);
});

test('Issuance disables untracked codes, counts members and usable codes, then mints one code', async () => {
  const store = nativeStore({ accounts: [owner, member], codes: { tracked: {}, orphan: {}, used: { usedBy: member } } });
  const issued = await run(['issue', '3', 'tracked'], store);
  assert.equal(issued.exitCode, undefined);
  assert.equal(issued.value.code, 'minted-1');
  assert.equal(store.codes.orphan.disabled, true);
  assert.equal(store.codes.tracked.disabled, false);
  assert.deepEqual(issued.calls.map(value => value.method), ['com.atproto.admin.disableInviteCodes', 'com.atproto.server.createInviteCode']);
  // One member plus two usable tracked codes reaches a ceiling of three.
  const full = await run(['issue', '3', 'tracked', 'minted-1'], store);
  assert.equal(full.value.ceilingReached, true);
  assert.equal(full.value.code, undefined);
  assert.equal(store.minted, 1);
});

test('Disable and inventory report code use without exposing other codes', async () => {
  const store = nativeStore({ accounts: [owner, member], codes: { open: {}, used: { usedBy: member } } });
  const inventory = await run(['inventory', 'open', 'used'], store);
  assert.deepEqual(inventory.value.codes.used, { known: true, disabled: false, did: member, handle: 'member.test' });
  assert.equal(inventory.value.codes.open.did, '');
  assert.deepEqual(inventory.calls, []);
  const disabled = await run(['disable', 'open'], store);
  assert.equal(disabled.value.codes.open.disabled, true);
  assert.equal(disabled.value.owner, owner);
});

test('Member helper fails closed on invalid input and native write failure', async () => {
  const store = nativeStore({ codes: { orphan: {} } });
  for (const args of [['issue', '0'], ['issue', '26'], ['issue', 'x'], ['inventory', 'bad code'], ['delete'], ['issue', '3', ...Array(26).fill('code')]]) {
    const result = await run(args, store);
    assert.equal(result.exitCode, 1);
    assert.equal(result.stderr, 'Native member operation failed');
  }
  const failed = await run(['issue', '3'], store, { failWrite: true });
  assert.equal(failed.exitCode, 1);
  assert.equal(store.minted, 0);
});
