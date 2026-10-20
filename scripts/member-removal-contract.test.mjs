import { test } from "node:test";
import assert from "node:assert/strict";
import { validRemoval } from "../web/src/member-removal-contract.ts";
const target = { deploymentId: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", invitationId: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", did: "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb" };
const now = new Date().toISOString();
const receipt = { ...target, id: "cccccccc-cccc-cccc-cccc-cccccccccccc", idempotencyKey: "dddddddd-dddd-dddd-dddd-dddddddddddd", requestedBy: target.deploymentId, state: "requested", message: "Recorded", createdAt: now, hostRequestedAt: null, deactivateAfter: null, completedAt: null, events: [{ state: "requested", message: "Recorded", createdAt: now, actorId: target.deploymentId }] };
test("Removal absence is explicit and bound to the accepted target", () => {
  assert.equal(validRemoval({ ...target, state: "not_requested" }, target), true);
  for (const bad of [{ ...target, state: "not_requested", code: "secret" }, { ...target, state: "not_requested", did: "did:plc:cccccccccccccccccccccccc" }, { state: "not_requested" }, null, { error: "Not found" }]) assert.equal(validRemoval(bad, target), false);
});
test("Removal receipts reject foreign scope, malformed audit and premature or changed windows", () => {
  assert.equal(validRemoval(receipt, target), true);
  const window = { ...receipt, state: "migration_window", hostRequestedAt: now, deactivateAfter: new Date(Date.parse(now) + 30 * 86400000).toISOString() };
  assert.equal(validRemoval(window, target), true);
  assert.equal(validRemoval({ ...window, state: "review" }, target), true);
  for (const bad of [{ ...receipt, did: "foreign" }, { ...receipt, idempotencyKey: "foreign" }, { ...receipt, requestedBy: "foreign" }, { ...receipt, password: "secret" }, { ...receipt, events: [] }, { ...receipt, events: [{ ...receipt.events[0], actorId: "invented" }] }, { ...window, deactivateAfter: now }, { ...window, state: "deactivated", completedAt: now }, { ...window, state: "requested" }, { ...receipt, completedAt: now }]) assert.equal(validRemoval(bad, target), false);
  const old = Date.now() - 31 * 86400000;
  const done = { ...window, state: "deactivated", createdAt: new Date(old).toISOString(), hostRequestedAt: new Date(old).toISOString(), deactivateAfter: new Date(old + 30 * 86400000).toISOString(), completedAt: now };
  assert.equal(validRemoval(done, target), true);
  assert.equal(validRemoval({ ...done, completedAt: done.hostRequestedAt }, target), false);
});
