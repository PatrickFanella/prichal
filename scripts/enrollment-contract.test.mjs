import { test } from 'node:test';
import assert from 'node:assert/strict';
import { validEnrollment } from '../web/src/enrollment-contract.ts';
const id = '8d234722-f0ad-47b2-8ef8-10f85d0c13b9';
const base = { id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', deploymentId: id, state: 'invitation_ready', message: 'Fixture invitation', code: 'fixture', expiresAt: '2026-10-01T23:00:00Z', accountUrl: 'https://' + id + '.localhost:42131/account', emailConfirmed: false, withdrawalRequested: false, withdrawalAttempts: 0, canCancel: false };
test('Invitation contract rejects stale codes and ambiguous withdrawal authority', () => {
  assert.equal(validEnrollment(base, id), true);
  for (const bad of [
    { ...base, state: 'withdrawn', withdrawalRequested: true, withdrawalAttempts: 1 },
    { ...base, withdrawalRequested: true, withdrawalAttempts: 1 },
    { ...base, code: undefined }, { ...base, expiresAt: null }, { ...base, withdrawalAttempts: 4 },
    { ...base, withdrawalAttempts: 0.5 }, { ...base, withdrawalRequested: true },
    { ...base, accountUrl: 'https://foreign.invalid/account' }, { ...base, deploymentId: 'foreign' },
    { ...base, state: 'failed' }, { ...base, withdrawalRequested: undefined },
  ]) assert.equal(validEnrollment(bad, id), false);
  const withdrawn = { ...base, state: 'withdrawn', code: undefined, withdrawalRequested: true, withdrawalAttempts: 1 };
  assert.equal(validEnrollment(withdrawn, id), true);
  assert.equal(validEnrollment({ ...withdrawn, withdrawalRequested: false, withdrawalAttempts: 0 }, id), false);
});
test('Completed accounts require consistent native identity and never an invitation code', () => {
  const verified = { ...base, state: 'verified', code: undefined, expiresAt: null, did: 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa', handle: 'fixture.test', emailConfirmed: true };
  assert.equal(validEnrollment(verified, id), true);
  for (const bad of [{ ...verified, code: 'fixture' }, { ...verified, did: 'did:plc:AAAAAAAAAAAAAAAAAAAAAAAA' }, { ...verified, emailConfirmed: false }, { ...verified, expiresAt: base.expiresAt }]) assert.equal(validEnrollment(bad, id), false);
});

test('Queued cancellation requires explicit dispatch eligibility and consistent terminal metadata', () => {
  const pending = { ...base, state: 'pending', code: undefined, expiresAt: null, canCancel: true };
  assert.equal(validEnrollment(pending, id), true);
  assert.equal(validEnrollment({ ...pending, canCancel: false }, id), true);
  const cancelled = { ...pending, state: 'cancelled', canCancel: false };
  assert.equal(validEnrollment(cancelled, id), true);
  for (const bad of [{ ...cancelled, canCancel: true }, { ...cancelled, expiresAt: base.expiresAt }, { ...cancelled, withdrawalRequested: true, withdrawalAttempts: 1 }, { ...base, canCancel: true }, { ...pending, canCancel: undefined }]) assert.equal(validEnrollment(bad, id), false);
});
