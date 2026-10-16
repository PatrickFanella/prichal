// Destructive fault injection is confined to newly allocated, labeled fixtures.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { generateKeyPairSync, randomBytes, randomUUID, createHash } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const image = 'ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336';
const edgeImage = 'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d';
const id = randomUUID();
const prefix = `prichal-storage-${id}`;
const root = resolve('.local/pds-storage', id);
mkdirSync(root, { recursive: true, mode: 0o700 });
const label = `tv.subcult.prichal.storage-qualification=${id}`;
const network = `${prefix}-net`;
const volumes = Object.fromEntries(['data', 'blobs', 'protocol'].map(role => [role, `${prefix}-${role}`]));
const names = Object.fromEntries(['pds', 'full', 'edge', 'protocol'].map(role => [role, `${prefix}-${role}`]));
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
  assert.equal(row.Config.Labels['tv.subcult.prichal.storage-qualification'], id);
  assert.equal(row.HostConfig.Privileged, false);
  assert.equal(row.HostConfig.NetworkMode, network);
  assert.ok(!row.HostConfig.PortBindings || Object.keys(row.HostConfig.PortBindings).length === 0);
  return row;
}
function stop(name) { owned(name); docker(['stop', '--time', '10', name]); }
function run(role, extra, command = []) {
  const name = names[role];
  docker(['run', '-d', '--name', name, '--label', label, '--network', network,
    '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--memory', role === 'pds' || role === 'full' ? '768m' : '192m',
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
  if (body !== undefined) headers['Content-Type'] = contentType;
  const bytes = body === undefined ? undefined : Buffer.isBuffer(body) ? body : Buffer.from(JSON.stringify(body));
  return JSON.parse(docker(['exec', '-i', names.protocol, 'node', '-e', probe], JSON.stringify({
    path, headers, ...(bytes === undefined ? {} : { method: 'POST', body: bytes.toString('base64') })
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
  report.limits = { productionDiskQuotaQualified: false, accountCeilingQualified: false, publicFederationQualified: false };
  writeFileSync(`${root}/report.json`, JSON.stringify(report, null, 2), { flag: 'wx', mode: 0o600 });
  console.log(`Storage qualification ${report.result}: ${root}/report.json`);
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
  const account = { handle: 'storage.test', email: 'storage@fixture.invalid', password };
  let session = json(request('/xrpc/com.atproto.server.createAccount', { ...account, inviteCode: invite.code }));
  const did = session.did;
  assert.match(did, /^did:plc:[a-z2-7]{24}$/);
  let auth = `Bearer ${session.accessJwt}`;
  const media = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j5ioAAAAASUVORK5CYII=', 'base64');
  const blob = json(request('/xrpc/com.atproto.repo.uploadBlob', media, auth, 'image/png')).blob;
  const record = { $type: 'app.bsky.feed.post', text: 'Disposable storage fault fixture', createdAt: new Date().toISOString(),
    embed: { $type: 'app.bsky.embed.images', images: [{ alt: 'Storage fixture', image: blob }] } };
  const written = json(request('/xrpc/com.atproto.repo.createRecord', { repo: did, collection: 'app.bsky.feed.post', record }, auth));
  const recordPath = `/xrpc/com.atproto.repo.getRecord?repo=${did}&collection=app.bsky.feed.post&rkey=${written.uri.split('/').at(-1)}`;
  const blobPath = `/xrpc/com.atproto.sync.getBlob?did=${did}&cid=${blob.ref.$link}`;
  function retained() {
    assert.equal(json(request(recordPath)).cid, written.cid);
    const stored = request(blobPath); assert.equal(stored.status, 200);
    assert.equal(createHash('sha256').update(Buffer.from(stored.body, 'base64')).digest('hex'), createHash('sha256').update(media).digest('hex'));
  }
  retained();
  owned(names.pds); docker(['restart', '--time', '10', names.pds]); await ready();
  session = json(request('/xrpc/com.atproto.server.createSession', { identifier: account.handle, password }));
  assert.equal(session.did, did); auth = `Bearer ${session.accessJwt}`; retained();
  report.checks.restartRetainsAccountRecordAndBlob = true;
  console.log('Disposable account, record and blob survived restart.');

  // The original is stopped before a second runtime mounts its fresh fixture DB.
  stop(names.pds);
  const stopped = request('/xrpc/_health'); assert.equal(stopped.status, 502);
  report.checks.stoppedRuntimeIngressStatus = stopped.status;
  docker(['network', 'disconnect', network, names.pds]);
  run('full', ['--network-alias', 'pds', '--env-file', envPath, ...pdsMounts,
    '--tmpfs', '/blobs:rw,size=8388608,mode=0700']);
  await ready();
  // Allocate at most 9 MiB on an 8 MiB tmpfs; never fill the host filesystem.
  const fill = String.raw`const fs=require('fs');const fd=fs.openSync('/blobs/.pressure','wx');let full=false;try{for(let n=0;n<9;n++)fs.writeSync(fd,Buffer.alloc(1024*1024,0x5a))}catch(e){if(e.code!=='ENOSPC')throw e;full=true}finally{fs.closeSync(fd)}if(!full)process.exit(1);console.log('ENOSPC')`;
  assert.equal(docker(['exec', names.full, 'node', '-e', fill]), 'ENOSPC');
  const rejected = request('/xrpc/com.atproto.repo.uploadBlob', Buffer.from('bounded full-disk probe'), auth, 'application/octet-stream');
  assert.ok(rejected.status >= 500, 'Full storage must refuse an upload');
  report.checks.fullBlobStorage = { bytes: 8388608, observedErrno: 'ENOSPC', uploadStatus: rejected.status,
    processHealthStatus: request('/xrpc/_health').status };
  docker(['exec', names.full, 'node', '-e', "require('fs').unlinkSync('/blobs/.pressure')"]);
  json(request('/xrpc/com.atproto.repo.uploadBlob', Buffer.from('bounded recovered-disk probe'), auth, 'application/octet-stream'));
  report.checks.uploadResumesAfterSpaceFreed = true;
  stop(names.full); docker(['network', 'disconnect', network, names.full]);
  docker(['network', 'connect', '--alias', 'pds', network, names.pds]);
  docker(['start', names.pds]); await ready(); retained();
  console.log('Bounded ENOSPC refused upload; freeing space restored writes; original media retained.');

  // Corrupt only the newly created fixture database, with every PDS writer stopped.
  stop(names.pds);
  const corrupt = String.raw`const fs=require('fs');const file='/pds/account.sqlite';fs.copyFileSync(file,file+'.before-corruption',fs.constants.COPYFILE_EXCL);const fd=fs.openSync(file,'r+');fs.writeSync(fd,Buffer.alloc(64,0x5a),0,64,0);fs.fsyncSync(fd);fs.closeSync(fd)`;
  docker(['run', '--rm', '--network', 'none', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
    '--memory', '128m', '--pids-limit', '32', ...pdsMounts, image, 'node', '-e', corrupt]);
  docker(['start', names.pds]);
  // A corrupt DB can make startup exit or a database operation fail. Process health alone is insufficient.
  await new Promise(resolve => setTimeout(resolve, 1000));
  const corruptHealth = request('/xrpc/_health');
  const corruptAccounts = request('/xrpc/com.atproto.admin.getInviteCodes?limit=1', undefined, admin);
  assert.ok(corruptAccounts.status >= 500, 'Corrupt account DB must not pass a database-backed check');
  report.checks.corruptAccountDatabase = { processHealthStatus: corruptHealth.status, databaseProbeStatus: corruptAccounts.status };
  stop(names.pds);
  const restore = "const fs=require('fs');fs.copyFileSync('/pds/account.sqlite.before-corruption','/pds/account.sqlite');";
  docker(['run', '--rm', '--network', 'none', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
    '--memory', '128m', '--pids-limit', '32', ...pdsMounts, image, 'node', '-e', restore]);
  docker(['start', names.pds]); await ready();
  assert.equal(json(request('/xrpc/com.atproto.server.createSession', { identifier: account.handle, password })).did, did);
  retained(); report.checks.databaseRestorationRetainsAccountRecordAndBlob = true;
  console.log('Corrupt SQLite refused database-backed readiness; restored fixture retained login, record and blob.');
  failed = false;
} finally {
  finalize();
  assert.equal(report.stopFailures.length, 0, 'Some disposable containers could not be stopped; inspect the private report');
}
