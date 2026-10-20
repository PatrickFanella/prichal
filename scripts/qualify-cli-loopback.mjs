// Real native CLI processes, synthetic loopback HTTP only. No hosting service,
// browser approval, Docker access, durable worker or release publication.
import assert from 'node:assert/strict';
import {createHash, randomBytes, randomUUID} from 'node:crypto';
import {chmodSync, lstatSync, mkdirSync, readFileSync, readdirSync, symlinkSync, writeFileSync} from 'node:fs';
import {createServer} from 'node:http';
import {resolve, join} from 'node:path';
import {spawn} from 'node:child_process';

assert.equal(process.env.PDS_QUALIFY_CLI_LOOPBACK, 'true', 'Explicit synthetic CLI qualification required');
assert.ok(process.platform === 'linux' && process.arch === 'x64', 'Native Linux AMD64 required');
const binary = resolve('.local/cli-build/pds-ass-linux-amd64');
const digest = path => createHash('sha256').update(readFileSync(path)).digest('hex');
assert.ok(lstatSync(binary).isFile() && !lstatSync(binary).isSymbolicLink());
const checksums = readFileSync('.local/cli-build/SHA256SUMS', 'utf8');
const entry = checksums.split('\n').find(line => line.endsWith('  pds-ass-linux-amd64'));
assert.ok(entry && /^[a-f0-9]{64}  pds-ass-linux-amd64$/.test(entry));
const binaryHash = digest(binary); assert.equal(binaryHash, entry.slice(0, 64));
const id = randomUUID(), root = resolve('.local/cli-loopback', id), config = join(root, 'config');
mkdirSync(root, {recursive: true, mode: 0o700});
const token = randomBytes(32).toString('base64url'), deviceCode = randomBytes(32).toString('base64url');
const org = randomUUID(), deployment = randomUUID(), key = randomUUID(), credential = randomUUID();
const checks = [], cases = [], children = new Set(), exits = [];
const report = {id, result: 'failed', stage: 'setup', checks, cases, binarySha256: binaryHash,
  nativePlatform: 'linux-amd64', qualifierSha256: digest(new URL(import.meta.url)), syntheticLoopbackOnly: true, realAuthorizationQualified: false,
  durableServerJobsQualified: false, nativePDSQualified: false, releasePublished: false};
const check = (name, value) => { assert.ok(!interrupted, 'Qualification harness interrupted'); assert.ok(value, name); checks.push(name); };
const receipt = () => writeFileSync(join(root, 'report.json'), JSON.stringify(report, null, 2), {mode: 0o600});
let api, trap, apiOrigin, trapHits = 0, protectedRequests = 0, postCount = 0;
let redirected = false, revoked = false, serverError;
let savedPayload, operation, pollObserved, interrupted = false, assertionsPassed = false;
for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { interrupted = true; for (const child of children) child.kill('SIGKILL'); });
const pollReady = new Promise(done => { pollObserved = done; });
const server = handler => createServer((request, response) => {
  void handler(request, response).catch(error => { serverError = error; response.statusCode = 500; response.end(); });
});
const listen = instance => new Promise((done, reject) => {
  instance.once('error', reject); instance.listen(0, '127.0.0.1', () => done('http://127.0.0.1:' + instance.address().port));
});
async function body(request) {
  let value = ''; for await (const part of request) { value += part; assert.ok(value.length <= 4096); }
  return value ? JSON.parse(value) : null;
}
function reply(response, value, status = 200) {
  response.writeHead(status, {'Content-Type': 'application/json'}); response.end(JSON.stringify(value));
}
function start(name, args) {
  assert.ok(!interrupted, 'Qualification harness interrupted');
  const child = spawn(binary, args.includes('--config-dir') ? args : [...args, '--config-dir', config], {stdio: ['ignore', 'pipe', 'pipe']});
  children.add(child); let out = '', err = '';
  child.stdout.on('data', data => { out += data; if (out.length > 1 << 20) child.kill('SIGKILL'); });
  child.stderr.on('data', data => { err += data; if (err.length > 1 << 20) child.kill('SIGKILL'); });
  const timer = setTimeout(() => child.kill('SIGKILL'), 20000);
  const done = new Promise((resolveChild, reject) => {
    child.once('error', error => { clearTimeout(timer); children.delete(child); reject(error); });
    child.once('close', (code, signal) => {
      clearTimeout(timer); children.delete(child);
      try {
        assert.ok(!out.includes(token) && !err.includes(token) && !out.includes(deviceCode) && !err.includes(deviceCode), 'CLI output exposed synthetic secret');
        cases.push({name, exit: code, signal, stdoutBytes: Buffer.byteLength(out), stderrBytes: Buffer.byteLength(err)});
        resolveChild({code, signal, out, err});
      } catch (error) { reject(error); }
    });
  });
  done.catch(() => {}); exits.push(done);
  return {child, done};
}
async function run(name, args, exit) {
  const result = await start(name, args).done;
  check(name + ' exit contract', result.code === exit && result.signal === null);
  assert.ok(!serverError, 'Loopback fixture rejected a request'); return result;
}
async function close(instance) {
  if (!instance?.listening) return;
  instance.closeAllConnections(); await new Promise(done => instance.close(done));
}

try {
  trap = server(async (_request, response) => { trapHits++; reply(response, {}); });
  const trapOrigin = await listen(trap);
  api = server(async (request, response) => {
    const path = request.url;
    assert.ok(!request.headers.cookie && !request.headers.origin, 'Ambient browser authority reached fixture');
    if (path === '/api/cli/device' || path === '/api/cli/token') {
      assert.equal(request.method, 'POST'); assert.ok(!request.headers.authorization);
      const value = await body(request); assert.equal(value.clientId, 'pds-ass-cli');
      if (path.endsWith('/device')) return reply(response, {deviceCode, userCode: 'ABCD-EFGH', verificationUri: apiOrigin + '/device', expiresIn: 60, interval: 5});
      assert.equal(value.deviceCode, deviceCode);
      return reply(response, {accessToken: token, credentialId: credential, expiresAt: new Date(Date.now() + 3600000).toISOString(), scope: 'deployments', tokenType: 'Bearer'});
    }
    protectedRequests++;
    assert.ok(request.headers.authorization === 'Bearer ' + token, 'Wrong scoped transport credential');
    if (revoked) return reply(response, {}, 401);
    if (path === '/api/organizations') {
      assert.equal(request.method, 'GET');
      if (redirected) { response.writeHead(307, {Location: trapOrigin + '/trap'}); response.end(); return; }
      return reply(response, [{id: org, name: 'Synthetic loopback organization', accessToken: token}]);
    }
    if (path === '/api/organizations/' + org + '/deployments') {
      assert.equal(request.method, 'POST'); const value = await body(request);
      assert.ok(value.idempotencyKey === key && value.name === 'loopback-server' && value.kind === 'test' && value.region === 'local');
      postCount++;
      if (savedPayload) assert.equal(JSON.stringify(value), savedPayload);
      else { savedPayload = JSON.stringify(value); operation = {id: deployment, organizationId: org, name: value.name, kind: value.kind, state: 'queued', accessToken: token}; }
      return reply(response, operation);
    }
    if (path === '/api/deployments/' + deployment) {
      assert.equal(request.method, 'GET'); reply(response, operation); pollObserved(); return;
    }
    throw Error('Unexpected loopback route');
  });
  apiOrigin = await listen(api);
  report.stage = 'synthetic-device-protocol'; receipt();
  const login = await run('synthetic login', ['login', '--api-url', apiOrigin, '--no-browser'], 0);
  check('synthetic login emits safe JSON', JSON.parse(login.out).signedIn === true && JSON.parse(login.out).credentialId === credential);
  const credentials = join(config, 'credentials.json');
  check('native config and credential permissions', (lstatSync(config).mode & 0o777) === 0o700 && (lstatSync(credentials).mode & 0o777) === 0o600);
  const stored = JSON.parse(readFileSync(credentials)); check('synthetic credential persisted privately', stored.token === token && stored.apiUrl === apiOrigin);
  report.stage = 'private-file-refusal'; receipt();
  const beforeUnsafe = protectedRequests; chmodSync(credentials, 0o644);
  try {
    const unsafe = await run('public credential refusal', ['organizations', 'list'], 1);
    check('public credential refused before HTTP', protectedRequests === beforeUnsafe && unsafe.out === '');
  } finally { chmodSync(credentials, 0o600); }
  const linked = join(root, 'linked-config'); mkdirSync(linked, {mode: 0o700}); symlinkSync(credentials, join(linked, 'credentials.json'));
  const symlink = await run('symlink credential refusal', ['organizations', 'list', '--config-dir', linked], 1);
  check('symlink credential refused before HTTP', protectedRequests === beforeUnsafe && symlink.out === '');
  const organizations = await run('scoped organization read', ['organizations', 'list'], 0);
  const rows = JSON.parse(organizations.out); check('organization output strips unknown secret fields', rows.length === 1 && rows[0].id === org && !('accessToken' in rows[0]));
  report.stage = 'real-sigint'; receipt();
  const deployArgs = ['deploy', '--organization', org, '--name', 'loopback-server', '--kind', 'test', '--idempotency-key', key];
  const waiting = start('interrupted deployment wait', [...deployArgs, '--wait']);
  let pollTimer; try { await Promise.race([pollReady, new Promise((_, reject) => { pollTimer = setTimeout(() => reject(Error('Native CLI never polled fixture')), 10000); })]); } finally { clearTimeout(pollTimer); }
  check('SIGINT delivered to real CLI process', waiting.child.kill('SIGINT'));
  const interrupted = await waiting.done;
  check('real SIGINT exits 130 without signal termination', interrupted.code === 130 && interrupted.signal === null);
  check('interruption prints recovery guidance', interrupted.err.includes('retry deploy with the same key'));
  check('fixture retains original queued operation', operation.state === 'queued' && operation.id === deployment && postCount === 1);
  const requestFile = join(config, 'request-' + key + '.json'), originalRequest = readFileSync(requestFile);
  check('request saved privately before interruption', (lstatSync(requestFile).mode & 0o777) === 0o600 && JSON.parse(originalRequest).request.idempotencyKey === key);
  report.stage = 'retained-key-replay'; receipt();
  const pending = await run('retained deployment replay', deployArgs, 3);
  check('replay uses one retained operation', JSON.parse(pending.out).id === deployment && postCount === 2 && readFileSync(requestFile).equals(originalRequest));
  const changed = [...deployArgs]; changed[changed.indexOf('loopback-server')] = 'changed-server';
  const conflict = await run('changed payload refusal', changed, 2);
  check('changed request refused before HTTP', postCount === 2 && conflict.out === '' && readFileSync(requestFile).equals(originalRequest));
  operation.state = 'failed'; const failed = await run('failed fixture status', ['status', '--deployment', deployment], 4);
  check('failed status retains deployment identity', JSON.parse(failed.out).id === deployment);
  operation.state = 'ready'; const ready = await run('ready fixture status', ['status', '--deployment', deployment], 0);
  check('ready output strips secret fields', JSON.parse(ready.out).id === deployment && !('accessToken' in JSON.parse(ready.out)));
  report.stage = 'redirect-and-revocation'; receipt();
  redirected = true; await run('redirect refusal', ['organizations', 'list'], 1); check('redirect target receives no request', trapHits === 0); redirected = false;
  revoked = true; const denied = await run('revoked credential refusal', ['organizations', 'list'], 1);
  check('revoked response is actionable and stdout empty', denied.out === '' && denied.err.includes('expired or was revoked'));
  await run('already revoked logout', ['logout'], 0);
  check('logout removes credential but preserves request', !readdirSync(config).includes('credentials.json') && readFileSync(requestFile).equals(originalRequest));
  check('binary and checksum file unchanged', digest(binary) === binaryHash && readFileSync('.local/cli-build/SHA256SUMS', 'utf8') === checksums);
  assert.ok(!serverError);
  Object.assign(report, {stage: 'cleanup', deploymentPosts: postCount, fixtureOperations: 1, redirectTargetRequests: trapHits, accessTokenAndDeviceCodeAbsentFromProcessOutput: true, credentialsRemoved: true});
  assertionsPassed = true; receipt();
} catch (error) {
  assertionsPassed = false;
  receipt(); writeFileSync(join(root, 'failure.txt'), String(error), {mode: 0o600});
  console.error('Native CLI loopback qualification failed; inspect the private stage receipt.'); process.exitCode = 1;
} finally {
  for (const child of children) child.kill('SIGKILL');
  await Promise.allSettled(exits);
  await close(api); await close(trap);
  report.cleanupVerified = children.size === 0 && !api?.listening && !trap?.listening;
  if (assertionsPassed && report.cleanupVerified && !interrupted) Object.assign(report, {result: 'passed', stage: 'complete', completedAt: new Date().toISOString()});
  else { report.result = 'failed'; process.exitCode = 1; }
  receipt();
  if (report.result === 'passed') console.log(JSON.stringify(report));
}
