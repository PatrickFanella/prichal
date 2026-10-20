import { test } from "node:test";
import assert from "node:assert/strict";
import { validMemberInvitation, validMemberSummary } from "../web/src/member-invitation-contract.ts";
const deployment = "8d234722-f0ad-47b2-8ef8-10f85d0c13b9";
const base = { id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", deploymentId: deployment, state: "invitation_ready", message: "Fixture", code: "fixture", expiresAt: "2026-10-21T23:00:00Z", createdAt: "2026-10-19T23:00:00Z", accountUrl: `https://${deployment}.localhost:42131/account`, withdrawalRequested: false };
test("Member invitation boundary rejects foreign identity, unsafe destinations and inconsistent codes", () => {
  assert.equal(validMemberInvitation(base, deployment), true);
  for (const bad of [
    { ...base, deploymentId: "foreign" }, { ...base, id: "foreign" }, { ...base, state: ["invitation_ready"] },
    { ...base, state: "accepted" }, { ...base, state: "expired" }, { ...base, code: undefined },
    { ...base, code: "<script>" }, { ...base, expiresAt: null }, { ...base, createdAt: "invalid" },
    { ...base, accountUrl: "https://foreign.invalid/account" }, { ...base, accountUrl: base.accountUrl + "?code=secret" },
    { ...base, accountUrl: base.accountUrl.replace("https://", "https://user:secret@") }, { ...base, withdrawalRequested: true },
  ]) assert.equal(validMemberInvitation(bad, deployment), false);
  const accepted = { ...base, state: "accepted", code: undefined, did: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", handle: "member.test" };
  assert.equal(validMemberInvitation(accepted, deployment), true);
  assert.equal(validMemberInvitation({ ...accepted, code: "stale" }, deployment), false);
  assert.equal(validMemberInvitation({ ...accepted, did: "did:plc:invalid" }, deployment), false);
  const withdrawn = { ...base, state: "withdrawn", code: undefined, withdrawalRequested: true };
  assert.equal(validMemberInvitation(withdrawn, deployment), true);
  assert.equal(validMemberInvitation({ ...withdrawn, withdrawalRequested: false }, deployment), false);
});
test("Member summary refuses duplicate IDs, malformed rows and unsupported ceilings", () => {
  const summary = { ceiling: 25, members: 0, open: 1, accounts: [], invitations: [base] };
  assert.equal(validMemberSummary(summary, deployment), true);
  for (const bad of [{ ...summary, ceiling: 26 }, { ...summary, members: -1 }, { ...summary, open: 0.5 }, { ...summary, invitations: [base, base] }, { ...summary, invitations: [{ ...base, state: "unknown" }] }, { ...summary, invitations: Array(101).fill(base) }]) assert.equal(validMemberSummary(bad, deployment), false);
});

test("Accepted roster is independent of invitation history and rejects duplicates or count mismatch", () => {
 const account={invitationId:"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",did:"did:plc:bbbbbbbbbbbbbbbbbbbbbbbb",handle:"older-member.test"};
 const summary={ceiling:25,members:1,open:1,accounts:[account],invitations:[base]};
 assert.equal(validMemberSummary(summary,deployment),true);
 for(const bad of [{...summary,accounts:undefined},{...summary,accounts:[]},{...summary,members:2},{...summary,members:2,accounts:[account,account]},{...summary,accounts:[{...account,did:"did:plc:invalid"}]},{...summary,accounts:[{...account,invitationId:"invalid"}]}])assert.equal(validMemberSummary(bad,deployment),false);
});
