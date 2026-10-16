// Native sign-in qualification uses only fresh isolated PDS and captured local mail.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { generateKeyPairSync, randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const image = 'ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336';
const edgeImage = 'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d';
const id = randomUUID();
const prefix = `prichal-email-factor-${id}`;
const root = resolve('.local/pds-email-factor', id);
mkdirSync(root, { recursive: true, mode: 0o700 });
const label = `tv.subcult.prichal.email-factor-qualification=${id}`;
const network = `${prefix}-net`;
const volumes = Object.fromEntries(['data', 'blobs', 'protocol'].map(role => [role, `${prefix}-${role}`]));
const names = Object.fromEntries(['pds', 'edge', 'protocol'].map(role => [role, `${prefix}-${role}`]));
const created = [];
let failed = true;
function docker(args, input) {
  const result = spawnSync('docker', args, { input, encoding: 'utf8', timeout: 60000, maxBuffer: 2 * 1024 * 1024 });
  // Docker errors can include environment values; retain no raw diagnostic output.
  assert.equal(result.status, 0, `Disposable Docker operation failed: ${args[0]}`);
  return result.stdout.trim();
}
function inspect(name) { return JSON.parse(docker(['inspect', name]))[0]; }
function owned(name) {
  const row = inspect(name);
  assert.equal(row.Config.Labels['tv.subcult.prichal.email-factor-qualification'], id);
  assert.equal(row.HostConfig.Privileged, false);
  assert.equal(row.HostConfig.NetworkMode, network);
  assert.ok(!row.HostConfig.PortBindings || Object.keys(row.HostConfig.PortBindings).length === 0);
  return row;
}
function stop(name) { owned(name); docker(['stop', '--time', '10', name]); }
function run(role, extra, command = []) {
  const name = names[role];
  docker(['run', '-d', '--name', name, '--label', label, '--network', network,
    '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--memory', role === 'pds' ? '768m' : '192m',
    '--cpus', '1', '--pids-limit', '128', ...extra, role === 'edge' ? edgeImage : image, ...command]);
  created.push(name);
  owned(name);
}
const probe = String.raw`
const fs = require('fs');
const request = JSON.parse(fs.readFileSync(0, 'utf8'));
const timer = setTimeout(() => process.exit(1), 6000);
const req = require('http').request({ hostname: 'edge', port: 8080, path: request.path,
  method: request.method || 'GET', headers: { Host: 'localhost', ...request.headers }
}, response => {
  const chunks = []; let length = 0;
  response.on('data', chunk => { length += chunk.length; if (length > 1024 * 1024) process.exit(1); chunks.push(chunk); });
  response.on('error', () => process.exit(1));
  response.on('end', () => {
    clearTimeout(timer);
    process.stdout.write(JSON.stringify({ status: response.statusCode, body: Buffer.concat(chunks).toString('base64') }));
  });
});
req.on('error', () => process.exit(1));
req.end(request.body === undefined ? undefined : Buffer.from(request.body, 'base64'));`;
function request(path, body, authorization, contentType = 'application/json') {
  const headers = authorization ? { Authorization: authorization } : {};
  if (body != null) headers['Content-Type'] = contentType;
  const bytes = body == null ? undefined : Buffer.isBuffer(body) ? body : Buffer.from(JSON.stringify(body));
  return JSON.parse(docker(['exec', '-i', names.protocol, 'node', '-e', probe], JSON.stringify({
    path, headers, method: body === undefined ? 'GET' : 'POST', ...(bytes === undefined ? {} : { body: bytes.toString('base64') })
  })));
}
function json(response, expected = 200) {
  assert.equal(response.status, expected, 'Disposable protocol status');
  return JSON.parse(Buffer.from(response.body, 'base64'));
}
async function ready() {
  for (let attempt = 0; ; attempt++) {
    assert.ok(owned(names.edge).State.Running, 'Disposable ingress exited during startup');
    assert.ok(owned(names.protocol).State.Running, 'Disposable protocol fixture exited during startup');
    try { json(request('/xrpc/_health')); return; }
    catch (error) { if (attempt >= 29) throw error; await new Promise(resolve => setTimeout(resolve, 250)); }
  }
}
const pdsMounts = ['--mount', `type=volume,src=${volumes.data},dst=/pds`];
const privateKey = generateKeyPairSync('ec', { namedCurve: 'secp256k1' }).privateKey.export({ format: 'jwk' });
const password = randomBytes(32).toString('hex');
const adminPassword = randomBytes(32).toString('hex');
const env = {
  PDS_HOSTNAME: 'localhost', PDS_PORT: '2583', PDS_DEV_MODE: 'true',
  PDS_DATA_DIRECTORY: '/pds', PDS_BLOBSTORE_DISK_LOCATION: '/blobs', PDS_BLOBSTORE_DISK_TMP_LOCATION: '/blobs/tmp',
  PDS_BLOB_UPLOAD_LIMIT: '5242880', PDS_INVITE_REQUIRED: 'true', PDS_RATE_LIMITS_ENABLED: 'true',
  PDS_SERVICE_HANDLE_DOMAINS: '.test', PDS_DID_PLC_URL: 'http://protocol-fixtures:2582',
  PDS_EMAIL_SMTP_URL: 'smtp://protocol-fixtures:2525', PDS_EMAIL_FROM_ADDRESS: 'noreply@fixture.invalid',
  PDS_CRAWLERS: '', LOG_ENABLED: 'false', PDS_ADMIN_PASSWORD: adminPassword,
  PDS_JWT_SECRET: randomBytes(32).toString('hex'),
  PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX: Buffer.from(privateKey.d, 'base64url').toString('hex'),
};
const envPath = `${root}/pds.env`;
writeFileSync(envPath, Object.entries(env).map(([key, value]) => `${key}=${value}`).join('\n') + '\n', { flag: 'wx', mode: 0o600 });
const config = `${root}/Caddyfile`;
writeFileSync(config, '{\n auto_https off\n admin off\n}\n:8080 {\n @allowed host localhost\n handle @allowed {\n reverse_proxy pds:2583\n }\n handle {\n respond "Unknown PDS host" 421\n }\n}\n', { flag: 'wx', mode: 0o600 });
const report = { id, image, completedAt: null, publicIdentity: false, publishedPorts: false, checks: {} };
let finalized = false;
function finalize() {
  if (finalized) return;
  finalized = true;
  const stopFailures = [];
  for (const name of created.toReversed()) {
    try { stop(name); } catch { stopFailures.push(name); }
  }
  if (stopFailures.length) failed = true;
  report.completedAt = new Date().toISOString();
  report.result = failed ? 'FAIL' : 'PASS';
  report.retainedStoppedContainers = created.filter(name => !stopFailures.includes(name));
  report.retainedVolumes = volumes;
  report.stopFailures = stopFailures;
  report.limits = { oauthBrowserQualified: false, realMailDeliveryQualified: false, productionPolicyChanged: false };
  writeFileSync(`${root}/report.json`, JSON.stringify(report, null, 2), { flag: 'wx', mode: 0o600 });
  console.log(`Email-factor qualification ${report.result}: ${root}/report.json`);
}
for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => {
  failed = true;
  finalize();
  process.exit(130);
});
try {
  assert.match(docker(['context', 'inspect', '--format', '{{.Endpoints.docker.Host}}']), /^unix:\/\//, 'Local Docker socket required');
  docker(['network', 'create', '--internal', '--label', label, network]);
  assert.equal(JSON.parse(docker(['network', 'inspect', network]))[0].Internal, true);
  for (const name of Object.values(volumes)) docker(['volume', 'create', '--label', label, name]);
  run('protocol', ['--network-alias', 'protocol-fixtures', '--env', 'PDS_FIXTURE_MODE=local',
    '--mount', `type=bind,src=${resolve('deploy/local/fixtures/protocol-services.mjs')},dst=/fixtures/protocol-services.mjs,readonly`,
    '--mount', `type=volume,src=${volumes.protocol},dst=/fixture-data`], ['node', '/fixtures/protocol-services.mjs']);
  run('pds', ['--network-alias', 'pds', '--env-file', envPath, ...pdsMounts,
    '--mount', `type=volume,src=${volumes.blobs},dst=/blobs`]);
  run('edge', ['--network-alias', 'edge', '--cap-add', 'NET_BIND_SERVICE',
    '--user', `${process.getuid()}:${process.getgid()}`, '--env', 'XDG_CONFIG_HOME=/tmp/config', '--env', 'XDG_DATA_HOME=/tmp/data',
    '--mount', `type=bind,src=${config},dst=/etc/caddy/Caddyfile,readonly`]);
  await ready();
  const admin = `Basic ${Buffer.from(`admin:${adminPassword}`).toString('base64')}`;
  const invite = json(request('/xrpc/com.atproto.server.createInviteCode', { useCount: 1 }, admin));
  const account = { handle: 'email-factor.test', email: 'email-factor@fixture.invalid', password };
  let session = json(request('/xrpc/com.atproto.server.createAccount', { ...account, inviteCode: invite.code }));
  const did = session.did;
  assert.match(did, /^did:plc:[a-z2-7]{24}$/);
  let auth = `Bearer ${session.accessJwt}`;
  writeFileSync(`${root}/account.json`, JSON.stringify({ ...account, did }), { flag: 'wx', mode: 0o600 });
  const secrets = new Set([password, adminPassword, ...Object.values(env).filter(value => value.length >= 32), invite.code, session.accessJwt, session.refreshJwt]);
  function rememberSession(value) {
    for (const field of ['accessJwt', 'refreshJwt']) if (value[field]) secrets.add(value[field]);
    assert.equal(value.did, did, 'Native session belongs to the disposable identity');
    return value;
  }
  function call(method, body, authorization = auth, expected = 200) {
    const response = request(`/xrpc/${method}`, body, authorization);
    assert.equal(response.status, expected, `Disposable ${method} status`);
    const text = Buffer.from(response.body, 'base64').toString();
    return text ? JSON.parse(text) : undefined;
  }
  function capturedMail() {
    const source = "const {DatabaseSync}=require('node:sqlite');const db=new DatabaseSync('/fixture-data/protocol.sqlite',{readOnly:true});process.stdout.write(JSON.stringify(db.prepare('SELECT message FROM mail WHERE recipient=? ORDER BY id').all('email-factor@fixture.invalid').map(row=>row.message)));db.close()";
    return JSON.parse(docker(['exec', names.protocol, 'node', '--no-warnings', '-e', source]));
  }
  function decodeMail(message) {
    return message.replace(/=\r\n/g, '').replace(/=([0-9A-F]{2})/gi, (_, hex) => String.fromCharCode(parseInt(hex, 16)));
  }
  call('com.atproto.server.requestEmailConfirmation', null);
  const confirmation = capturedMail().map(decodeMail).map(message => /verify-email\?code=([A-Za-z0-9-]+)/.exec(message)?.[1]).find(Boolean);
  assert.ok(confirmation, 'Local email confirmation was captured'); secrets.add(confirmation);
  call('com.atproto.server.confirmEmail', { email: account.email, token: confirmation });
  assert.equal(call('com.atproto.server.getSession').emailConfirmed, true);
  call('com.atproto.server.updateEmail', { email: account.email, emailAuthFactor: true });
  assert.equal(call('com.atproto.server.getSession').emailAuthFactor, true);
  const beforeChallenge = capturedMail().length;
  const challenge = call('com.atproto.server.createSession', { identifier: account.handle, password }, null, 401);
  assert.equal(challenge.error, 'AuthFactorTokenRequired');
  assert.ok(!challenge.accessJwt && !challenge.refreshJwt, 'Password-only sign-in returned no session');
  const mail = capturedMail(); assert.equal(mail.length, beforeChallenge + 1, 'One sign-in email captured');
  const code = /<code\b[^>]*>\s*([A-Za-z0-9-]+)\s*<\/code>/.exec(decodeMail(mail.at(-1)))?.[1];
  assert.ok(code, 'Sign-in code was captured'); secrets.add(code);
  const invalid = call('com.atproto.server.createSession', { identifier: account.handle, password, authFactorToken: 'INVALID-CODE' }, null, 400);
  assert.ok(!invalid.accessJwt && !invalid.refreshJwt, 'Invalid code returned no session');
  session = rememberSession(call('com.atproto.server.createSession', { identifier: account.handle, password, authFactorToken: code }, null));
  assert.equal(session.emailAuthFactor, true);
  auth = `Bearer ${session.accessJwt}`;
  const replay = call('com.atproto.server.createSession', { identifier: account.handle, password, authFactorToken: code }, null, 400);
  assert.ok(!replay.accessJwt && !replay.refreshJwt, 'Consumed code returned no session');
  report.checks = { confirmedEmail: true, nativeFactorEnabled: true, passwordOnlyStatus: 401,
    passwordOnlyError: 'AuthFactorTokenRequired', capturedMailAccepted: true, invalidAndReplayedCodesRefused: true };
  console.log('Confirmed email, password-only refusal, captured code sign-in and invalid/replayed code refusal passed.');

  // Existing app-password and session behavior are separate from primary-password challenges.
  const app = call('com.atproto.server.createAppPassword', { name: 'disposable-email-factor' });
  secrets.add(app.password);
  rememberSession(call('com.atproto.server.createSession', { identifier: account.handle, password: app.password }, null));
  report.checks.appPasswordExemptionObserved = true;
  call('com.atproto.server.revokeAppPassword', { name: app.name });
  const disable = call('com.atproto.server.updateEmail', { email: account.email, emailAuthFactor: false }, auth, 400);
  assert.ok(!disable.accessJwt, 'Disabling the factor returned no session');
  assert.equal(call('com.atproto.server.getSession').emailAuthFactor, true, 'Token-free disable retained the factor');
  report.checks.tokenFreeDisableRefused = true;

  const methods = ['com.atproto.server.getSession', 'com.atproto.server.updateEmail'];
  for (const method of methods) {
    const response = request(`/xrpc/${method}`, method.endsWith('updateEmail') ? { email: account.email, emailAuthFactor: false } : undefined);
    assert.equal(response.status, 401, 'Native factor state/mutation requires account authentication');
  }
  report.checks.unauthenticatedStateAndMutationRefused = true;
  // Include codes generated by both confirmation and token-free disable requests.
  for (const message of capturedMail().map(decodeMail)) {
    for (const match of message.matchAll(/(?:code=|<code\b[^>]*>\s*)([A-Za-z0-9-]+)/gi)) secrets.add(match[1]);
  }
  for (const name of created) {
    const result = spawnSync('docker', ['logs', name], { encoding: 'utf8', timeout: 10000, maxBuffer: 2 * 1024 * 1024 });
    assert.equal(result.status, 0, 'Fixture log collection');
    const logs = result.stdout + result.stderr;
    for (const secret of secrets) {
      if (secret && secret.length >= 6) assert.ok(!logs.includes(secret), 'Native credentials or codes appeared in fixture logs');
    }
  }
  report.checks.fixtureLogSecretScanPassed = true;
  report.checks.managementJobsExportsAndAuditsNotUsed = true;
  console.log('App-password exemption recorded; token-free disable and anonymous state/mutation refused; fixture log secret scan passed.');
  failed = false;
} finally {
  finalize();
  assert.equal(report.stopFailures.length, 0, 'Some disposable containers could not be stopped; inspect the private report');
}
