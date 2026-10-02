// Native member-helper qualification against an empty, identity-enabled local PDS fixture.
// Runs the agent's fixed helpers through docker exec; ingress and the admission guard are not exercised.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
const [id] = process.argv.slice(2);
assert.match(id ?? '', /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
function docker(...args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 60000, maxBuffer: 32 << 20 });
  return { status: result.status, stdout: result.stdout, stderr: result.stderr };
}
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').stdout.trim().startsWith('unix:///'), 'Local Unix Docker socket required');
const container = `pds-ass-tenant-${id}`;
const inspected = JSON.parse(docker('inspect', container).stdout)[0];
assert.equal(inspected.Config.Labels['tv.subcult.pds-ass.instance'], id);
assert.ok(inspected.Config.Env.includes('PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582'), 'Identity-enabled fixture required');
assert.ok(inspected.State.Running, 'Running fixture PDS required');
const helper = name => readFileSync(new URL(`../backend/internal/nodeagent/${name}`, import.meta.url), 'utf8');
function run(script, ...args) {
  const result = docker('exec', container, 'node', '--no-warnings', '-e', script, ...args);
  assert.equal(result.status, 0, 'Fixed helper failed');
  return JSON.parse(result.stdout);
}
const enrollment = helper('enrollment-helper.cjs'), members = helper('member-helper.cjs'), account = helper('account-helper.cjs');
// Signup goes straight to the native process here; the guard is outside this check.
const signupScript = `fetch('http://127.0.0.1:2583/xrpc/com.atproto.server.createAccount',{method:'POST',headers:{'Content-Type':'application/json'},body:process.argv[1],signal:AbortSignal.timeout(15000)}).then(async r=>process.stdout.write(JSON.stringify({status:r.status,body:r.status===200?await r.json():undefined})))`;
function signup(person, code) { return JSON.parse(docker('exec', container, 'node', '-e', signupScript, JSON.stringify({ ...person, inviteCode: code })).stdout); }
const adminCode = `fetch('http://127.0.0.1:2583/xrpc/com.atproto.server.createInviteCode',{method:'POST',headers:{Authorization:'Basic '+Buffer.from('admin:'+process.env.PDS_ADMIN_PASSWORD).toString('base64'),'Content-Type':'application/json'},body:'{"useCount":1}'}).then(r=>r.json()).then(v=>process.stdout.write(JSON.stringify(v)))`;
function person(label) {
  const tag = randomBytes(4).toString('hex');
  return { handle: `${label}-${tag}.test`, email: `${label}-${tag}@fixture.invalid`, password: randomBytes(24).toString('hex') };
}
const root = `.local/member-native/${randomUUID()}`;
mkdirSync(root, { recursive: true, mode: 0o700 });
const accounts = {};
const save = () => writeFileSync(root + '/accounts.json', JSON.stringify(accounts), { mode: 0o600 });
const checks = {};

assert.equal(run(enrollment, 'inventory').count, 0, 'Empty fixture required');
assert.equal(run(members, 'issue', '2').ownerRequired, true);
checks.ownerRequiredBeforeFirstAccount = true;

// Owner's first account through the existing first-account helper.
const firstCode = run(enrollment, 'invitation').code;
accounts.owner = person('owner'); save();
const owner = signup(accounts.owner, firstCode);
assert.equal(owner.status, 200, 'Owner signup');
accounts.owner.did = owner.body.did; save();

// Two usable codes reach a ceiling of two; an untracked code is disabled first.
const orphan = JSON.parse(docker('exec', container, 'node', '-e', adminCode).stdout).code;
const first = run(members, 'issue', '2');
assert.ok(first.code && first.accounts === 1 && first.owner === owner.body.did);
assert.equal(run(members, 'inventory', orphan).codes[orphan].disabled, true, 'Orphan code stayed usable');
checks.orphanCodeDisabled = true;
const second = run(members, 'issue', '2', first.code);
assert.ok(second.code && second.code !== first.code);
const full = run(members, 'issue', '2', first.code, second.code);
assert.ok(full.ceilingReached && !full.code, 'Ceiling with two usable codes');
checks.ceilingCountsUsableCodes = true;

// A member signs up; the code is single-use and its use is reported.
accounts.member = person('member'); save();
const member = signup(accounts.member, first.code);
assert.equal(member.status, 200, 'Member signup');
accounts.member.did = member.body.did; save();
assert.equal(signup(person('reuse'), first.code).status, 400, 'Code reuse');
const used = run(members, 'inventory', first.code).codes[first.code];
assert.ok(used.did === member.body.did && used.handle === accounts.member.handle, 'Code use reported');
checks.singleUseAndReported = true;

// A disabled code cannot create an account.
assert.equal(run(members, 'disable', second.code).codes[second.code].disabled, true);
assert.equal(signup(person('withdrawn'), second.code).status, 400, 'Disabled code signup');
checks.disabledCodeRejected = true;
const third = run(members, 'issue', '2');
assert.ok(third.code, 'One member and no usable codes leaves room for one more');
assert.ok(run(members, 'issue', '2', third.code).ceilingReached, 'Ceiling counts members');
checks.ceilingCountsMembers = true;
run(members, 'disable', third.code);

// Owner status, moderation inspection and export still resolve the owner with a member present.
const ownerStatus = run(enrollment, 'inventory');
assert.ok(ownerStatus.count === 1 && ownerStatus.did === owner.body.did, 'First-account inventory');
const moderation = run(account, 'native_status');
assert.equal(moderation.did, owner.body.did, 'Moderation inspection');
const exported = docker('exec', container, 'node', '--no-warnings', '-e', helper('export-helper.cjs'), 'native_export', owner.body.did);
assert.equal(exported.status, 0, 'Owner export');
assert.equal(JSON.parse(exported.stdout).did, owner.body.did);
checks.ownerStatusModerationExport = true;

const report = { completedAt: new Date().toISOString(), deployment: id, ownerDID: owner.body.did, memberDID: member.body.did, ...checks, ingressAndGuardExercised: false };
writeFileSync(root + '/report.json', JSON.stringify(report, null, 2), { flag: 'wx', mode: 0o600 });
console.log(JSON.stringify({ ...report, report: root + '/report.json' }));
