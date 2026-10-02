// Member invitation journey on a retained disposable local deployment whose owner account is verified.
// Run the agent with PDS_AGENT_MEMBER_CEILING=2 so the native ceiling is reachable.
import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { localOwnerClient } from './local-owner-client.mjs';
import { localNativeClient } from './local-native-client.mjs';
const deployment = process.argv[2];
assert.match(deployment ?? '', /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
const ownerName = process.env.PDS_LOCAL_QUALIFICATION_OWNER ?? 'alice';
assert.ok(['alice', 'bob'].includes(ownerName), 'Disposable local owner required');
const owner = await localOwnerClient(ownerName), other = await localOwnerClient(ownerName === 'alice' ? 'bob' : 'alice');
assert.equal((await (await fetch('http://localhost:2582/_health')).json()).localFixture, true);
const root = `.local/member-invitations/${randomUUID()}`;
mkdirSync(root, { recursive: true, mode: 0o700 });
const firstAccount = await owner.api(`/api/deployments/${deployment}/enrollment`);
assert.equal(firstAccount.status, 200, 'Owner enrollment read');
assert.equal(firstAccount.value.state, 'verified', 'A verified owner account is required');
const ownerDID = firstAccount.value.did;
const native = localNativeClient(deployment, new URL(firstAccount.value.accountUrl).origin);
const path = `/api/deployments/${deployment}/members/invitations`;
async function wait(id, states) {
  for (let n = 0; n < 120; n++) {
    const result = await owner.api('/api/member-invitations/' + id);
    assert.equal(result.status, 200, 'Owner member invitation read');
    if (states.includes(result.value.state)) return result.value;
    assert.ok(!['failed'].includes(result.value.state), 'Member invitation failed');
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  throw new Error('Member invitation checkpoint timed out');
}
async function invite() {
  const key = randomUUID();
  const requested = await owner.api(path, { idempotencyKey: key });
  assert.equal(requested.status, 200, 'Member invitation submission');
  return { key, invitation: await wait(requested.value.id, ['invitation_ready', 'ceiling_reached', 'owner_required']) };
}
function person(label) {
  const tag = randomBytes(4).toString('hex');
  return { handle: `${label}-${tag}.test`, email: `${label}-${tag}@fixture.invalid`, password: randomBytes(24).toString('hex') };
}
const accounts = {};
const checks = {};

// Issue, replay and owner scope.
const a = await invite();
assert.equal(a.invitation.state, 'invitation_ready');
assert.ok(a.invitation.code && new Date(a.invitation.expiresAt) - Date.now() > 71 * 3600 * 1000, 'Member window');
const replay = await owner.api(path, { idempotencyKey: a.key });
assert.ok(replay.status === 200 && replay.value.code === a.invitation.code && replay.value.expiresAt === a.invitation.expiresAt, 'Replay changed the invitation');
assert.equal((await other.api('/api/member-invitations/' + a.invitation.id)).status, 404);
assert.equal((await other.api(path)).status, 404);
assert.equal((await other.api(path, { idempotencyKey: randomUUID() })).status, 404);
checks.ownerReplay = checks.foreignOwnerDenied = true;

// Ceiling of two: owner + no members + two usable codes.
const b = await invite();
assert.equal(b.invitation.state, 'invitation_ready');
const refused = await invite();
assert.equal(refused.invitation.state, 'ceiling_reached', 'Native ceiling with two usable codes');
assert.ok(!refused.invitation.code);
checks.nativeCeilingCountsUsableCodes = true;

// Member A creates an account directly on the PDS; the code is single-use.
accounts.a = person('member-a');
writeFileSync(root + '/accounts.json', JSON.stringify(accounts), { mode: 0o600 });
const created = await native('com.atproto.server.createAccount', { ...accounts.a, inviteCode: a.invitation.code });
assert.match(created.did, /^did:plc:[a-z2-7]{24}$/);
assert.notEqual(created.did, ownerDID);
accounts.a.did = created.did;
writeFileSync(root + '/accounts.json', JSON.stringify(accounts), { mode: 0o600 });
await native('com.atproto.server.createAccount', { ...person('reuse'), inviteCode: a.invitation.code }, undefined, 400);
checks.singleUseCode = true;
assert.equal((await owner.api('/api/member-invitations/' + a.invitation.id + '/refresh', {})).status, 200);
const accepted = await wait(a.invitation.id, ['accepted']);
assert.ok(!accepted.code && accepted.did === created.did && accepted.handle === accounts.a.handle, 'Accepted member identity');
const login = await native('com.atproto.server.createSession', { identifier: accounts.a.handle, password: accounts.a.password });
assert.equal(login.did, created.did, 'Member login');
checks.memberAccepted = checks.memberLogin = true;

// Withdrawal disables an unused code; admission closes without open invitations.
assert.equal((await owner.api('/api/member-invitations/' + b.invitation.id + '/withdraw', {})).status, 200);
const withdrawn = await wait(b.invitation.id, ['withdrawn', 'accepted']);
assert.equal(withdrawn.state, 'withdrawn');
await native('com.atproto.server.createAccount', { ...person('withdrawn'), inviteCode: b.invitation.code }, undefined, 403);
checks.withdrawalClosedAdmission = true;

// One member plus one usable code reaches the ceiling again.
const c = await invite();
assert.equal(c.invitation.state, 'invitation_ready');
await native('com.atproto.server.createAccount', { ...person('withdrawn'), inviteCode: b.invitation.code }, undefined, 400);
checks.withdrawnCodeRejectedNatively = true;
const full = await invite();
assert.equal(full.invitation.state, 'ceiling_reached');
checks.ceilingCountsMembers = true;
assert.equal((await owner.api('/api/member-invitations/' + c.invitation.id + '/withdraw', {})).status, 200);
assert.equal((await wait(c.invitation.id, ['withdrawn'])).state, 'withdrawn');

// The owner account is still the one management tracks.
await new Promise(resolve => setTimeout(resolve, 5100));
assert.equal((await owner.api('/api/enrollments/' + firstAccount.value.id + '/refresh', {})).status, 200);
// A failing native inventory would mark the operation failed after three attempts.
await new Promise(resolve => setTimeout(resolve, 8000));
const ownerStatus = await owner.api('/api/enrollments/' + firstAccount.value.id);
assert.ok(ownerStatus.value.state === 'verified' && ownerStatus.value.did === ownerDID, 'Owner account identity changed');
checks.ownerStatusUnchanged = true;

// The owner's portable export still captures only the owner's repository.
const exportKey = randomUUID();
const exportRequest = await owner.api(`/api/deployments/${deployment}/exports`, { idempotencyKey: exportKey });
assert.equal(exportRequest.status, 200, 'Owner export request');
let exported;
for (let n = 0; n < 80; n++) {
  exported = (await owner.api('/api/exports/' + exportRequest.value.id)).value;
  if (exported.state !== 'queued') break;
  await new Promise(resolve => setTimeout(resolve, 500));
}
assert.equal(exported.state, 'ready', 'Owner export with a member account present');
assert.equal(exported.did ?? ownerDID, ownerDID);
checks.ownerExportWithMember = true;

const summary = await owner.api(path);
assert.equal(summary.status, 200);
assert.equal(summary.value.members, 1);
assert.equal(summary.value.open, 0);
const report = { completedAt: new Date().toISOString(), owner: ownerName, deployment, ownerDID, memberDID: created.did, invitations: summary.value.invitations.map(value => ({ id: value.id, state: value.state })), ...checks, liveExpiryQualified: false, liveFederationQualified: false };
writeFileSync(root + '/report.json', JSON.stringify(report, null, 2), { flag: 'wx', mode: 0o600 });
console.log(JSON.stringify({ ...report, report: root + '/report.json' }));
