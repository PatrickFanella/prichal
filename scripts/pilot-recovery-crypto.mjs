// Host operator only: one fixed disposable pilot; no production management activation.
import assert from 'node:assert/strict';
import { isAbsolute } from 'node:path';
import { sealRecovery, openRecovery } from './recovery-crypto.mjs';
const [action, input, output, key] = process.argv.slice(2);
assert.equal(process.argv.length, 6, 'Action and three private absolute paths required');
assert.ok([input, output, key].every(value => isAbsolute(value ?? '')));
const expected = { id: 'e8e82a65-7aa8-4e37-9306-0de701bf1b19', scope: 'pds-pilot' };
if (action === 'seal') {
  await sealRecovery(input, output, key, { version: 1, ...expected, createdAt: new Date().toISOString() });
} else if (action === 'open') {
  await openRecovery(input, output, key, expected);
} else {
  throw new Error('Only seal or open is supported');
}
console.log('Private pilot recovery operation completed. No credential values are logged.');
