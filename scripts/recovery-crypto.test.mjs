import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, existsSync, readdirSync, chmodSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomBytes, randomUUID } from 'node:crypto';
import { sealRecovery, openRecovery } from './recovery-crypto.mjs';
test('Recovery encrypts private content and authenticates data and metadata before publication', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'pds-recovery-'));
  try {
    const input = join(directory, 'input'); const encrypted = join(directory, 'encrypted'); const key = join(directory, 'key');
    const payload = Buffer.concat([Buffer.from('private recovery fixture'), randomBytes(180000)]);
    writeFileSync(input, payload, { mode: 0o600 }); writeFileSync(key, randomBytes(32), { mode: 0o600 });
    const metadata = { version: 1, id: randomUUID(), scope: 'pds-local', createdAt: new Date().toISOString() };
    await sealRecovery(input, encrypted, key, metadata);
    assert.equal(readFileSync(encrypted).includes(Buffer.from('private recovery fixture')), false);
    const restored = join(directory, 'restored');
    assert.deepEqual(await openRecovery(encrypted, restored, key), metadata);
    assert.deepEqual(readFileSync(restored), payload);
    await assert.rejects(openRecovery(encrypted, restored, key));
    assert.deepEqual(readFileSync(restored), payload, 'Verified destination was overwritten');
    const wrongKey = join(directory, 'wrong-key'); writeFileSync(wrongKey, randomBytes(32), { mode: 0o600 });
    const rejected = join(directory, 'rejected');
    await assert.rejects(openRecovery(encrypted, rejected, wrongKey)); assert.equal(existsSync(rejected), false);
    for (const offset of [8, 35, readFileSync(encrypted).length - 17, readFileSync(encrypted).length - 1]) {
      const tampered = Buffer.from(readFileSync(encrypted)); tampered[offset] ^= 1;
      const file = join(directory, `tampered-${offset}`); writeFileSync(file, tampered, { mode: 0o600 });
      await assert.rejects(openRecovery(file, rejected, key)); assert.equal(existsSync(rejected), false);
    }
    const truncated = join(directory, 'truncated'); writeFileSync(truncated, readFileSync(encrypted).subarray(0, 25));
    await assert.rejects(openRecovery(truncated, rejected, key));
    chmodSync(key, 0o644); await assert.rejects(openRecovery(encrypted, rejected, key)); chmodSync(key, 0o600);
    const alias = join(directory, 'key-alias'); symlinkSync(key, alias); await assert.rejects(openRecovery(encrypted, rejected, alias));
    assert.equal(readdirSync(directory).some(name => name.endsWith('.partial')), false);
  } finally { rmSync(directory, { recursive: true }); }
});
test('Empty backups round trip and nonce reuse does not occur across encryptions', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'pds-empty-'));
  try {
    const input = join(directory, 'input'); const key = join(directory, 'key'); writeFileSync(input, ''); writeFileSync(key, randomBytes(32), { mode: 0o600 });
    const metadata = { version: 1, id: randomUUID(), scope: 'management-local', createdAt: new Date().toISOString() };
    const first = join(directory, 'first'); const second = join(directory, 'second');
    await sealRecovery(input, first, key, metadata); await sealRecovery(input, second, key, metadata);
    assert.notDeepEqual(readFileSync(first).subarray(8, 20), readFileSync(second).subarray(8, 20));
    const restored = join(directory, 'restored'); await openRecovery(first, restored, key); assert.equal(readFileSync(restored).length, 0);
  } finally { rmSync(directory, { recursive: true }); }
});

test('Pilot recovery rejects a different scope or instance before plaintext publication', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'pds-pilot-recovery-'));
  try {
    const input = join(directory, 'input'); const encrypted = join(directory, 'encrypted'); const key = join(directory, 'key');
    writeFileSync(input, 'pilot native snapshot', { mode: 0o600 }); writeFileSync(key, randomBytes(32), { mode: 0o600 });
    const expected = { scope: 'pds-pilot', id: randomUUID() };
    const metadata = { version: 1, ...expected, createdAt: new Date().toISOString() };
    await sealRecovery(input, encrypted, key, metadata);
    const rejected = join(directory, 'rejected');
    await assert.rejects(openRecovery(encrypted, rejected, key, { ...expected, scope: 'pds-local' }));
    await assert.rejects(openRecovery(encrypted, rejected, key, { ...expected, id: randomUUID() }));
    assert.equal(existsSync(rejected), false);
    assert.deepEqual(await openRecovery(encrypted, join(directory, 'restored'), key, expected), metadata);
  } finally { rmSync(directory, { recursive: true }); }
});
