import { test } from 'node:test';
import assert from 'node:assert/strict';
import { claimChoice, claimIntent, claimReceipt } from '../web/src/endpoint-claim-contract.ts';
const target = { id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', hostname: 'pds.creator.net' };
test('Claim review rejects malformed or foreign retained scope and activation authority', () => {
  assert.equal(claimIntent(target), true);
  for (const bad of [{ ...target, id: 'foreign' }, { ...target, hostname: 'https://pds.creator.net' }, { ...target, hostname: 'pds..creator.net' }, { ...target, hostname: '-pds.creator.net' }, []]) assert.equal(claimIntent(bad), false);
  const choice = { ...target, organizationId: 'org', state: 'awaiting_ownership', expiresAt: '2026-10-08T22:00:00Z', canActivate: false };
  assert.equal(claimChoice(choice, 'org', target), true);
  for (const bad of [{ ...choice, organizationId: 'foreign' }, { ...choice, id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' }, { ...choice, hostname: 'foreign.net' }, { ...choice, state: 'active' }, { ...choice, expiresAt: 'invalid' }, { ...choice, canActivate: true }]) assert.equal(claimChoice(bad, 'org', target), false);
});
test('A reservation receipt cannot silently imply activation or contradictory lifecycle authority', () => {
  const receipt = { choiceId: target.id, hostname: target.hostname, state: 'reserved', reserved: true, expiresAt: '2026-10-08T22:00:00Z', canActivate: false };
  assert.equal(claimReceipt(receipt, target), true);
  for (const state of ['released', 'expired']) assert.equal(claimReceipt({ ...receipt, state, reserved: false }, target), true);
  for (const bad of [{ ...receipt, choiceId: 'foreign' }, { ...receipt, hostname: 'foreign.net' }, { ...receipt, state: 'released' }, { ...receipt, reserved: false }, { ...receipt, state: 'active' }, { ...receipt, canActivate: true }, { ...receipt, expiresAt: 'invalid' }]) assert.equal(claimReceipt(bad, target), false);
});
