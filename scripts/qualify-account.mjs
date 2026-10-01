// Creates one retained, isolated account on the fixed local qualification PDS.
import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
const pds = 'http://localhost:2583';
const directory = 'http://localhost:2582';
const health = await fetch(`${directory}/_health`, { signal: AbortSignal.timeout(5000) });
assert.equal((await health.json()).localFixture, true, 'Isolated identity fixture required');
const env = Object.fromEntries(readFileSync('.local/pds.env', 'utf8').trim().split('\n').map(line => {
  const offset = line.indexOf('='); return [line.slice(0, offset), line.slice(offset + 1)];
}));
const admin = `Basic ${Buffer.from(`admin:${env.PDS_ADMIN_PASSWORD}`).toString('base64')}`;
async function xrpc(method, body, authorization, expected = 200) {
  const response = await fetch(`${pds}/xrpc/${method}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { ...(authorization ? { Authorization: authorization } : {}), ...(body == null ? {} : { 'Content-Type': 'application/json' }) },
    ...(body == null ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(15000),
  });
  // Never include upstream bodies, tokens or credentials in assertion output.
  assert.equal(response.status, expected, `Local ${method.split('?')[0]} status`);
  if (expected !== 200 || response.headers.get('content-length') === '0') return undefined;
  const text = await response.text(); return text ? JSON.parse(text) : undefined;
}
const tag = randomBytes(6).toString('hex');
const description = await xrpc('com.atproto.server.describeServer');
assert.equal(description.inviteCodeRequired, true);
assert.deepEqual(description.availableUserDomains, ['.test'], 'Disposable handle domain required');
const account = { handle: `qual-${tag}.test`, email: `qualification-${tag}@fixture.invalid`, password: randomBytes(24).toString('hex') };
mkdirSync('.local/account-fixtures', { recursive: true, mode: 0o700 });
const statePath = `.local/account-fixtures/${tag}.json`;
// Save before creation so a failed journey retains recoverable fixture credentials.
writeFileSync(statePath, JSON.stringify(account), { flag: 'wx', mode: 0o600 });
const invite = await xrpc('com.atproto.server.createInviteCode', { useCount: 1 }, admin);
const session = await xrpc('com.atproto.server.createAccount', { ...account, inviteCode: invite.code });
assert.match(session.did, /^did:plc:[a-z2-7]{24}$/);
writeFileSync(statePath, JSON.stringify({ ...account, did: session.did }), { mode: 0o600 });
const authorization = `Bearer ${session.accessJwt}`;
const login = await xrpc('com.atproto.server.createSession', { identifier: account.handle, password: account.password });
assert.equal(login.did, session.did);
await xrpc('com.atproto.server.createAccount', { ...account, handle: `reuse-${tag}.test`, email: `reuse-${tag}@fixture.invalid`, inviteCode: invite.code }, undefined, 400);
const resolved = await xrpc(`com.atproto.identity.resolveHandle?handle=${account.handle}`);
assert.equal(resolved.did, session.did);
const didDoc = await (await fetch(`${directory}/${session.did}`)).json();
assert.ok(didDoc.alsoKnownAs.includes(`at://${account.handle}`));
assert.equal(didDoc.service.find(service => service.id.endsWith('#atproto_pds')).serviceEndpoint, pds);
const invalidOp = await fetch(`${directory}/${session.did}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type: 'plc_operation', sig: 'invalid' }) });
assert.equal(invalidOp.status, 400);
const audit = await (await fetch(`${directory}/${session.did}/log/audit`)).json();
const badSignature = { ...audit.at(-1).operation, prev: audit.at(-1).cid, sig: randomBytes(64).toString('base64url') };
const invalidSignature = await fetch(`${directory}/${session.did}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(badSignature) });
assert.equal(invalidSignature.status, 400, 'Forged PLC update rejected');
assert.equal((await (await fetch(`${directory}/${session.did}/log`)).json()).length, 1, 'Rejected PLC update changed history');
await xrpc('com.atproto.server.requestEmailConfirmation', null, authorization);
const messages = await (await fetch(`${directory}/_fixture/mail?recipient=${encodeURIComponent(account.email)}`)).json();
assert.equal(messages.length, 1, 'Verification mail captured locally');
// Upstream sends quoted-printable multipart mail. Decode soft wraps and escaped bytes.
const mail = messages[0].message.replace(/=\r\n/g, '').replace(/=([0-9A-F]{2})/gi, (_, hex) => String.fromCharCode(Number.parseInt(hex, 16)));
const token = /verify-email\?code=([A-Za-z0-9-]+)/.exec(mail)?.[1];
assert.ok(token, 'Verification code absent from captured mail');
await xrpc('com.atproto.server.confirmEmail', { email: account.email, token }, authorization);
await xrpc('com.atproto.server.confirmEmail', { email: account.email, token }, authorization, 400);
assert.equal((await xrpc('com.atproto.server.getSession', undefined, authorization)).emailConfirmed, true);
const media = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j5ioAAAAASUVORK5CYII=', 'base64');
const upload = await fetch(`${pds}/xrpc/com.atproto.repo.uploadBlob`, { method: 'POST', headers: { Authorization: authorization, 'Content-Type': 'image/png' }, body: media });
assert.equal(upload.status, 200, 'Media upload');
const { blob } = await upload.json();
const frames = [];
const stream = new WebSocket('ws://localhost:2583/xrpc/com.atproto.sync.subscribeRepos');
stream.binaryType = 'arraybuffer';
await new Promise((resolve, reject) => {
  const timer = setTimeout(() => { stream.close(); reject(new Error('Local subscription timed out')); }, 5000);
  stream.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
  stream.addEventListener('error', () => { clearTimeout(timer); reject(new Error('Local subscription failed')); }, { once: true });
});
stream.addEventListener('message', event => {
  if (frames.length < 20 && event.data.byteLength < 1024 * 1024) frames.push(Buffer.from(event.data).toString('base64'));
});
const record = { $type: 'app.bsky.feed.post', text: `Local qualification ${tag}`, createdAt: new Date().toISOString(), embed: { $type: 'app.bsky.embed.images', images: [{ alt: 'Local one pixel fixture', image: blob }] } };
const written = await xrpc('com.atproto.repo.createRecord', { repo: session.did, collection: 'app.bsky.feed.post', record }, authorization);
const rkey = written.uri.split('/').at(-1);
await new Promise(resolve => setTimeout(resolve, 1500));
stream.close();
assert.ok(frames.length, 'Subscription events absent after record write');
const found = await xrpc(`com.atproto.repo.getRecord?repo=${session.did}&collection=app.bsky.feed.post&rkey=${rkey}`);
assert.equal(found.cid, written.cid); assert.deepEqual(found.value, record);
const stored = await fetch(`${pds}/xrpc/com.atproto.sync.getBlob?did=${session.did}&cid=${blob.ref.$link}`);
assert.equal(stored.status, 200); assert.deepEqual(Buffer.from(await stored.arrayBuffer()), media);
const exported = await fetch(`${pds}/xrpc/com.atproto.sync.getRepo?did=${session.did}`);
assert.equal(exported.status, 200);
const car = Buffer.from(await exported.arrayBuffer());
// Verify commit signature and MST/content integrity using the pinned upstream implementation.
const identity = await (await fetch(`${directory}/${session.did}/data`)).json();
const result = spawnSync('docker', ['exec', '-i', 'pds-ass-runtime-pds-1', 'node', '/qualification/verify-repo.cjs'], { input: JSON.stringify({ car: car.toString('base64'), did: session.did, key: identity.verificationMethods.atproto, rkey, frames }), encoding: 'utf8', timeout: 15000 });
assert.equal(result.status, 0, 'Pinned upstream repository verification');
console.log(result.stdout.trim());
writeFileSync(statePath, JSON.stringify({ ...account, did: session.did, uri: written.uri, cid: written.cid, blobCid: blob.ref.$link }), { mode: 0o600 });
const restart = spawnSync('docker', ['compose', '-f', 'deploy/local/compose.yaml', 'restart', 'pds', 'protocol-fixtures'], { encoding: 'utf8', timeout: 60000 });
assert.equal(restart.status, 0, 'Local account services restart');
const ready = spawnSync(process.execPath, ['scripts/check-pds.mjs'], { encoding: 'utf8', timeout: 45000 });
assert.equal(ready.status, 0, 'PDS readiness after restart');
const recovered = await xrpc('com.atproto.server.createSession', { identifier: account.handle, password: account.password });
assert.equal(recovered.did, session.did); assert.equal(recovered.emailConfirmed, true);
assert.equal((await xrpc(`com.atproto.repo.getRecord?repo=${session.did}&collection=app.bsky.feed.post&rkey=${rkey}`)).cid, written.cid);
const retainedBlob = await fetch(`${pds}/xrpc/com.atproto.sync.getBlob?did=${session.did}&cid=${blob.ref.$link}`);
assert.equal(retainedBlob.status, 200); assert.deepEqual(Buffer.from(await retainedBlob.arrayBuffer()), media);
assert.equal((await (await fetch(`${directory}/${session.did}`)).json()).id, session.did);
console.log('Account, confirmed email, PLC identity, record and uploaded media persisted across restart.');
console.log('Local account login, invite reuse rejection, DID/handle resolution, captured email confirmation, record and media retrieval passed.');
console.log(`Retained fixture credentials: ${statePath}; downstream federation remains unqualified.`);
