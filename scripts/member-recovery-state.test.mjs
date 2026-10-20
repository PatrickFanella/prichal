import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, copyFileSync, chmodSync, symlinkSync, unlinkSync, readdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { memberRecoveryFiles, restoreMemberRecoveryFiles } from './member-recovery-state.mjs';
const id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', ticket = 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', did = 'did:plc:bbbbbbbbbbbbbbbbbbbbbbbb';
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'member-recovery-')); t.after(() => rmSync(root, { recursive: true, force: true }));
  const state = join(root, 'state'), checkpoint = join(root, 'checkpoint'), candidate = join(root, 'candidate');
  for (const p of [state, checkpoint, candidate]) mkdirSync(p, { mode: 0o700 });
  mkdirSync(join(state, id + '-enrollment'), { mode: 0o700 });
  const write = (path, value) => writeFileSync(path, JSON.stringify(value), { mode: 0o600 });
  const members = join(state, id + '-members.json'), exits = join(state, id + '-member-exits.json'), policy = join(state, id + '-enrollment', 'members.json');
  return { root, state, checkpoint, candidate, members, exits, policy, write };
}
test('Member recovery retains private journals, invitation codes and exact exit timestamps without exposing them to the guard', t => {
  const f = fixture(t); const start = Date.now();
  f.write(f.members, { instanceId: id, entries: [{ ticket, state: 'accepted', did, handle: 'member.test' }, { ticket: 'cccccccc-cccc-cccc-cccc-cccccccccccc', state: 'open', code: 'private-fixture-code', issuedAt: start, expiresAt: start + 72 * 3600000 }] });
  f.write(f.exits, { instanceId: id, version: 1, entries: [{ instanceId: id, ticket, did, state: 'migration_window', requestedAt: start, deactivateAfter: start + 30 * 86400000 }] });
  f.write(f.policy, { instanceId: id, ceiling: 25, invitations: [{ ticket: 'cccccccc-cccc-cccc-cccc-cccccccccccc', issuedAt: start, expiresAt: start + 72 * 3600000 }] });
  writeFileSync(join(f.state, id + '.json'), 'PRIVATE ADMINISTRATOR KEY', { mode: 0o600 });
  const files = memberRecoveryFiles(f.state, id); assert.equal(files.length, 3);
  for (const [source, name] of files) copyFileSync(source, join(f.checkpoint, name));
  const restored = restoreMemberRecoveryFiles(f.checkpoint, f.candidate, id); assert.equal(restored.files, 3);
  const again = memberRecoveryFiles(f.candidate, id);
  for (let n = 0; n < files.length; n++) assert.ok(readFileSync(files[n][0]).equals(readFileSync(again[n][0])));
  assert.deepEqual(readdirSync(join(f.candidate, id + '-enrollment')), ['members.json']);
  assert.ok(!readFileSync(restored.guardPolicy, 'utf8').includes('private-fixture-code'));
  assert.throws(() => restoreMemberRecoveryFiles(f.checkpoint, f.candidate, id), /Existing member recovery target/);
});
test('Member recovery refuses foreign identities, unsafe paths, orphaned deadlines and public or symlinked metadata', t => {
  const f = fixture(t);
  assert.deepEqual(memberRecoveryFiles(f.state, id), []);
  assert.throws(() => memberRecoveryFiles(f.state, '../other'));
  f.write(f.exits, { version: 1, instanceId: id, entries: [] });
  assert.throws(() => memberRecoveryFiles(f.state, id), /requires member journal/);
  f.write(f.members, { instanceId: id, entries: [] });
  f.write(f.exits, { version: 1, instanceId: id, entries: [{ ticket, did }] });
  assert.throws(() => memberRecoveryFiles(f.state, id), /accepted identity/);
  unlinkSync(f.exits);
  f.write(f.members, { instanceId: 'foreign', entries: [] });
  assert.throws(() => memberRecoveryFiles(f.state, id), /Foreign/);
  f.write(f.members, { instanceId: id, entries: [] }); chmodSync(f.members, 0o644);
  assert.throws(() => memberRecoveryFiles(f.state, id), /Unsafe/);
  chmodSync(f.members, 0o600); copyFileSync(f.members, join(f.root, 'retained')); unlinkSync(f.members); symlinkSync(join(f.root, 'retained'), f.members);
  assert.throws(() => memberRecoveryFiles(f.state, id), /Unsafe/);
});
test('Owner-only checkpoints remain compatible and unsafe restore directories are refused', t => {
  const f = fixture(t);
  assert.deepEqual(restoreMemberRecoveryFiles(f.checkpoint, f.candidate, id), { files: 0, guardPolicy: null });
  assert.deepEqual(memberRecoveryFiles(f.candidate, id), []);
  chmodSync(join(f.candidate, id + '-enrollment'), 0o755);
  assert.throws(() => restoreMemberRecoveryFiles(f.checkpoint, f.candidate, id), /Private regular/);
  f.write(join(f.checkpoint, 'member-invitations.json'), { instanceId: id, entries: [] });
  writeFileSync(join(f.checkpoint, 'member-exits.json'), '{', { mode: 0o600 });
  assert.throws(() => restoreMemberRecoveryFiles(f.checkpoint, f.candidate, id), /Invalid member recovery JSON/);
});
