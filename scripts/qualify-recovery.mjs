// Cold recovery drill for the fixed, isolated account qualification stack only.
import assert from 'node:assert/strict';
import { randomBytes, randomUUID, createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, readdirSync, lstatSync, copyFileSync, openSync, closeSync, unlinkSync, rmdirSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { performance } from 'node:perf_hooks';
import { sealRecovery, openRecovery } from './recovery-crypto.mjs';
const root = resolve('.');
const id = randomUUID();
const work = resolve('.local/recovery-work', id);
const host = join(work, 'host');
const bundles = resolve('.local/recovery-bundles');
const keys = resolve('.local/recovery-keys');
for (const path of [work, host, bundles, keys]) {
  mkdirSync(path, { recursive: true, mode: 0o700 });
  const info = lstatSync(path); assert.ok(info.isDirectory() && !(info.mode & 0o077), 'Private recovery directory required');
}
mkdirSync('.local/account-fixtures/qualification.lock', { mode: 0o700 });
process.on('exit', () => rmdirSync('.local/account-fixtures/qualification.lock'));
function command(program, args, options = {}) {
  const result = spawnSync(program, args, { encoding: 'utf8', timeout: 60000, maxBuffer: 4 * 1024 * 1024, ...options });
  // Never echo command arguments or stderr: resolved runtime configuration contains secrets.
  assert.equal(result.status, 0, 'Local recovery subprocess failed');
  return result.stdout;
}
function docker(...args) { return command('docker', args); }
function original(...args) { return docker('compose', '-f', 'deploy/local/compose.yaml', ...args); }
assert.ok(docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'), 'Local Docker socket required');
const description = await (await fetch('http://localhost:2583/xrpc/com.atproto.server.describeServer')).json();
assert.equal(description.inviteCodeRequired, true); assert.deepEqual(description.availableUserDomains, ['.test']);
assert.equal((await (await fetch('http://localhost:2582/_health')).json()).localFixture, true);
const config = JSON.parse(original('config', '--format', 'json'));
assert.deepEqual(Object.keys(config.services).sort(), ['ingress', 'pds', 'protocol-fixtures']);
const image = config.services.pds.image;
assert.match(image, /^ghcr\.io\/bluesky-social\/pds:0\.4@sha256:[a-f0-9]{64}$/);
assert.equal(config.services['protocol-fixtures'].image, image);
const sourceContainers = JSON.parse(docker('inspect', 'pds-ass-runtime-pds-1', 'pds-ass-runtime-protocol-fixtures-1', 'pds-ass-runtime-ingress-1'));
for (const container of sourceContainers) assert.equal(container.Config.Labels['com.docker.compose.project'], 'pds-ass-runtime');
assert.equal(sourceContainers[0].Config.Image, image);
assert.equal(sourceContainers[1].Config.Image, image);
assert.match(config.services.ingress.image, /^caddy:2\.10\.2-alpine@sha256:[a-f0-9]{64}$/);
assert.equal(sourceContainers[2].Config.Image, config.services.ingress.image);
const environment = Object.fromEntries(sourceContainers[0].Config.Env.map(entry => { const i = entry.indexOf('='); return [entry.slice(0, i), entry.slice(i + 1)]; }));
for (const name of ['PDS_JWT_SECRET', 'PDS_ADMIN_PASSWORD', 'PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX']) assert.ok(environment[name] === config.services.pds.environment[name], 'Protected configuration differs from source runtime');
assert.equal(environment.PDS_DID_PLC_URL, 'http://protocol-fixtures:2582');
assert.equal(environment.PDS_DEV_MODE, 'true');
const fixturePaths = readdirSync('.local/account-fixtures').filter(name => name.endsWith('.json')).map(name => join('.local/account-fixtures', name));
fixturePaths.sort((a, b) => lstatSync(b).mtimeMs - lstatSync(a).mtimeMs);
let account;
for (const path of fixturePaths) {
  const info = lstatSync(path); assert.ok(info.isFile() && !(info.mode & 0o077), 'Private account fixture required');
  const value = JSON.parse(readFileSync(path, 'utf8'));
  if (value.did && value.uri && value.blobCid) { account = value; break; }
}
assert.ok(account && /^qual-[a-f0-9]+\.test$/.test(account.handle), 'Retained qualification account required');
async function xrpc(method, body, authorization) {
  const response = await fetch(`http://localhost:2583/xrpc/${method}`, { method: body === undefined ? 'GET' : 'POST',
    headers: { ...(authorization ? { Authorization: authorization } : {}), ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(15000) });
  assert.equal(response.status, 200, 'Local recovery protocol status'); return response.json();
}
const login = await xrpc('com.atproto.server.createSession', { identifier: account.handle, password: account.password });
assert.equal(login.did, account.did); assert.equal(login.emailConfirmed, true);
const sourceMarker = { $type: 'app.bsky.feed.post', text: `Local recovery source ${id}`, createdAt: new Date().toISOString() };
const sourceRecord = await xrpc('com.atproto.repo.createRecord', { repo: account.did, collection: 'app.bsky.feed.post', record: sourceMarker }, `Bearer ${login.accessJwt}`);
const protocolInventory = JSON.parse(command('docker', ['exec', 'pds-ass-runtime-pds-1', 'node', '-e', readFileSync('deploy/local/fixtures/blob-inventory.cjs', 'utf8')]));
const recordKey = sourceRecord.uri.split('/').at(-1);
const originalBlob = Buffer.from(await (await fetch(`http://localhost:2583/xrpc/com.atproto.sync.getBlob?did=${account.did}&cid=${account.blobCid}`)).arrayBuffer());
const originalIdentity = await (await fetch(`http://localhost:2582/${account.did}/data`)).json();
writeFileSync(join(host, 'compose.json'), JSON.stringify(config), { mode: 0o600 });
copyFileSync('.local/pds.env', join(host, 'pds.env'));
for (const [source, name] of [['deploy/local/Caddyfile', 'Caddyfile'], ['deploy/local/fixtures/protocol-services.mjs', 'protocol-services.mjs'], ['deploy/local/fixtures/verify-repo.cjs', 'verify-repo.cjs'], ['deploy/local/fixtures/recover-identity.cjs', 'recover-identity.cjs'], ['deploy/local/fixtures/blob-inventory.cjs', 'blob-inventory.cjs']]) copyFileSync(source, join(host, name));
const keyPath = join(keys, `${id}.key`); writeFileSync(keyPath, randomBytes(32), { flag: 'wx', mode: 0o600 });
const bundle = join(bundles, `${id}.pdsbackup`);
const freshProject = `pds-ass-recovery-${id}`;
const mounts = { data: 'pds-ass-runtime_pds-data', blobs: 'pds-ass-runtime_pds-blobs', protocol: 'pds-ass-runtime_protocol-data' };
function requireSourceFenced() {
  for (const container of JSON.parse(docker('inspect', ...sourceContainers.map(value => value.Id)))) assert.equal(container.State.Running, false, 'Source writer fencing failed');
  for (const volume of Object.values(mounts)) assert.equal(docker('ps', '--filter', `volume=${volume}`, '--format', '{{.ID}}').trim(), '', 'Another source volume user is active');
}
function snapshotMounts(volumes, hostPath, readOnly = true) {
  return [...Object.entries(volumes).flatMap(([directory, volume]) => ['--mount', `type=volume,src=${volume},dst=/snapshot/${directory}${readOnly ? ',readonly' : ''}`]),
    '--mount', `type=bind,src=${hostPath},dst=/snapshot/host${readOnly ? ',readonly' : ''}`];
}
function inventory(volumes, hostPath, filesOnly = false) {
  return JSON.parse(docker('run', '--rm', '--network', 'none', '--memory', '192m', '--cpus', '0.5', ...snapshotMounts(volumes, hostPath),
    '--mount', `type=bind,src=${root}/deploy/local/fixtures/inventory.cjs,dst=/inventory.cjs,readonly`, '--entrypoint', 'node', image, '/inventory.cjs', ...(filesOnly ? ['--files-only'] : [])));
}
let stopped = false; let restoredConfigPath; let freshStarted = false;
const recoveryStarted = performance.now();
try {
  stopped = true; original('stop', 'ingress', 'pds', 'protocol-fixtures');
  requireSourceFenced();
  const capturedAt = new Date().toISOString();
  inventory(mounts, host, true); // Enforce source byte/file/type bounds before staging copy.
  const staging = Object.fromEntries(Object.keys(mounts).map(name => [name, `${freshProject}_stage_${name}`]));
  for (const volume of Object.values(staging)) docker('volume', 'create', '--label', `pds-ass.recovery-stage=${id}`, volume);
  const raw = join(work, 'cold-source.tar'); const rawFD = openSync(raw, 'wx', 0o600);
  try { command('docker', ['run', '--rm', '--network', 'none', ...snapshotMounts(mounts, host), '--entrypoint', 'tar', image, '-C', '/snapshot', '-cf', '-', 'data', 'blobs', 'protocol'], { stdio: ['ignore', rawFD, 'pipe'] }); } finally { closeSync(rawFD); }
  docker('run', '--rm', '--network', 'none', ...snapshotMounts(staging, host, false), '--mount', `type=bind,src=${raw},dst=/source.tar,readonly`, '--entrypoint', 'tar', image, '-C', '/snapshot', '-xf', '/source.tar');
  docker('run', '--rm', '--network', 'none', ...snapshotMounts(staging, host, false), '--mount', `type=bind,src=${root}/deploy/local/fixtures/normalize-snapshot.cjs,dst=/normalize.cjs,readonly`, '--entrypoint', 'node', image, '/normalize.cjs');
  const before = inventory(staging, host);
  assert.ok(before.databases >= 4 && before.files.length > 0, 'Complete database inventory required');
  const manifest = { version: 1, id, scope: 'pds-local', capturedAt, image, inventory: before, protocolInventory };
  writeFileSync(join(host, 'manifest.json'), JSON.stringify(manifest), { mode: 0o600 });
  const plain = join(work, 'snapshot.tar'); const fd = openSync(plain, 'wx', 0o600);
  try { command('docker', ['run', '--rm', '--network', 'none', ...snapshotMounts(staging, host), '--entrypoint', 'tar', image, '-C', '/snapshot', '-cf', '-', 'data', 'blobs', 'protocol', 'host'], { stdio: ['ignore', fd, 'pipe'] }); } finally { closeSync(fd); }
  await sealRecovery(plain, bundle, keyPath, { version: 1, id, scope: 'pds-local', createdAt: capturedAt });
  const restoredArchive = join(work, 'authenticated-restore.tar');
  const metadata = await openRecovery(bundle, restoredArchive, keyPath); assert.equal(metadata.id, id);
  assert.equal(createHash('sha256').update(readFileSync(plain)).digest('hex'), createHash('sha256').update(readFileSync(restoredArchive)).digest('hex'), 'Authenticated archive differs from snapshot');
  unlinkSync(plain); // Delete only the generated staging archive after authenticated verification.
  unlinkSync(raw);
  const restoreHost = join(work, 'restored-host'); mkdirSync(restoreHost, { mode: 0o700 });
  const freshVolumes = Object.fromEntries(Object.keys(mounts).map(name => [name, `${freshProject}_${name}`]));
  for (const volume of Object.values(freshVolumes)) docker('volume', 'create', '--label', `pds-ass.recovery-fixture=${id}`, volume);
  docker('run', '--rm', '--network', 'none', ...snapshotMounts(freshVolumes, restoreHost, false), '--mount', `type=bind,src=${restoredArchive},dst=/snapshot.tar,readonly`, '--entrypoint', 'tar', image, '-C', '/snapshot', '-xf', '/snapshot.tar');
  assert.deepEqual(inventory(freshVolumes, restoreHost), before, 'Restored file/database/blob inventory differs');
  const recovered = JSON.parse(readFileSync(join(restoreHost, 'compose.json'), 'utf8'));
  recovered.name = freshProject;
  const volumeKeys = { 'pds-data': 'data', 'pds-blobs': 'blobs', 'protocol-data': 'protocol' };
  for (const [name, value] of Object.entries(recovered.volumes)) { value.name = freshVolumes[volumeKeys[name]]; value.external = true; delete value.labels; }
  for (const [name, value] of Object.entries(recovered.networks)) value.name = `${freshProject}_${name}`;
  for (const [name, service] of Object.entries(recovered.services)) {
    delete service.labels; service.restart = 'no';
    for (const mount of service.volumes) {
      if (mount.type !== 'bind') continue;
      const file = name === 'pds' ? 'verify-repo.cjs' : name === 'ingress' ? 'Caddyfile' : 'protocol-services.mjs';
      mount.source = join(restoreHost, file); mount.read_only = true;
    }
  }
  recovered.services.pds.volumes.push({ type: 'bind', source: join(restoreHost, 'recover-identity.cjs'), target: '/qualification/recover-identity.cjs', read_only: true });
  restoredConfigPath = join(work, 'restore.compose.json'); writeFileSync(restoredConfigPath, JSON.stringify(recovered), { mode: 0o600 });
  requireSourceFenced();
  freshStarted = true;
  docker('compose', '-f', restoredConfigPath, '-p', freshProject, 'up', '-d');
  command(process.execPath, ['scripts/check-pds.mjs']);
  const restoredLogin = await xrpc('com.atproto.server.createSession', { identifier: account.handle, password: account.password });
  assert.deepEqual(JSON.parse(command('docker', ['exec', `${freshProject}-pds-1`, 'node', '-e', readFileSync(join(restoreHost, 'blob-inventory.cjs'), 'utf8')])), protocolInventory, 'Signed repository or referenced media inventory differs after restore');
  assert.equal(restoredLogin.did, account.did); assert.equal(restoredLogin.emailConfirmed, true);
  const restoredRecord = await xrpc(`com.atproto.repo.getRecord?repo=${account.did}&collection=app.bsky.feed.post&rkey=${recordKey}`);
  assert.equal(restoredRecord.cid, sourceRecord.cid); assert.deepEqual(restoredRecord.value, sourceMarker);
  const restoredBlobResponse = await fetch(`http://localhost:2583/xrpc/com.atproto.sync.getBlob?did=${account.did}&cid=${account.blobCid}`); assert.equal(restoredBlobResponse.status, 200);
  assert.deepEqual(Buffer.from(await restoredBlobResponse.arrayBuffer()), originalBlob);
  assert.deepEqual(await (await fetch(`http://localhost:2582/${account.did}/data`)).json(), originalIdentity);
  command('docker', ['exec', `${freshProject}-pds-1`, 'node', '/qualification/recover-identity.cjs', account.did]);
  assert.deepEqual(await (await fetch(`http://localhost:2582/${account.did}/data`)).json(), originalIdentity, 'Recovered PLC rotation changed public account data');
  const frames = []; const stream = new WebSocket('ws://localhost:2583/xrpc/com.atproto.sync.subscribeRepos'); stream.binaryType = 'arraybuffer';
  await new Promise((accept, reject) => { const timer = setTimeout(() => { stream.close(); reject(new Error('Recovery stream timeout')); }, 5000); stream.addEventListener('open', () => { clearTimeout(timer); accept(); }, { once: true }); stream.addEventListener('error', () => { clearTimeout(timer); reject(new Error('Recovery stream failure')); }, { once: true }); });
  stream.addEventListener('message', event => { if (frames.length < 20 && event.data.byteLength < 1024 * 1024) frames.push(Buffer.from(event.data).toString('base64')); });
  const newRecord = await xrpc('com.atproto.repo.createRecord', { repo: account.did, collection: 'app.bsky.feed.post', record: { $type: 'app.bsky.feed.post', text: `Local restored write ${id}`, createdAt: new Date().toISOString() } }, `Bearer ${restoredLogin.accessJwt}`);
  await new Promise(accept => setTimeout(accept, 1500)); stream.close();
  const exported = await fetch(`http://localhost:2583/xrpc/com.atproto.sync.getRepo?did=${account.did}`); assert.equal(exported.status, 200);
  const car = Buffer.from(await exported.arrayBuffer()).toString('base64');
  command('docker', ['exec', '-i', `${freshProject}-pds-1`, 'node', '/qualification/verify-repo.cjs'], { input: JSON.stringify({ car, did: account.did, key: originalIdentity.verificationMethods.atproto, rkey: newRecord.uri.split('/').at(-1), frames }) });
  const report = { id, capturedAt, completedAt: new Date().toISOString(), recoverySeconds: Number(((performance.now() - recoveryStarted) / 1000).toFixed(2)), sourceMarkerRecovered: true,
    files: before.files.length, databases: before.databases, bytes: before.totalBytes, repositories: protocolInventory.repositories, signedRecords: protocolInventory.signedRecords, referencedBlobs: protocolInventory.referencedBlobs.length,
    observedLostSourceMarkers: 0, sourceMarkerLagSeconds: Number(((Date.parse(capturedAt) - Date.parse(sourceMarker.createdAt)) / 1000).toFixed(2)), recoveredPLCSigner: true,
    bundle: bundle.slice(root.length + 1), keyReference: keyPath.slice(root.length + 1), restoredProject: freshProject,
    limits: 'Local cold recovery only; independent storage, scheduled RPO, live DNS/TLS/email/federation and production RTO remain unqualified.' };
  writeFileSync(join(work, 'report.json'), JSON.stringify(report, null, 2), { mode: 0o600 });
  console.log(`Authenticated encrypted recovery passed: ${report.files} files, ${report.databases} SQLite databases, ${report.recoverySeconds}s local recovery.`);
  console.log(`Identity, confirmed email, signed writes, media and stream verified; report: .local/recovery-work/${id}/report.json`);
} finally {
  // Stop every restored writer before returning the original stack to its endpoint.
  if (freshStarted) docker('compose', '-f', restoredConfigPath, '-p', freshProject, 'stop');
  if (stopped) { original('start'); command(process.execPath, ['scripts/check-pds.mjs']); }
}
console.log('Original stack resumed. Restored containers/volumes and encrypted bundle retained; no source data was removed.');
