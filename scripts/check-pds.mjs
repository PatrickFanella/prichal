import assert from 'node:assert/strict';
import { request } from 'node:http';
const origin = 'http://localhost:2583';
for (let attempt = 0; ; attempt++) {
  try {
    const response = await fetch(`${origin}/xrpc/_health`, { signal: AbortSignal.timeout(2000) });
    assert.equal(response.status, 200);
    const health = await response.json();
    assert.equal(typeof health.version, 'string');
    console.log(`Official PDS health passed (version ${health.version}).`);
    break;
  } catch (error) {
    if (attempt >= 29) throw error;
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
}
const description = await fetch(`${origin}/xrpc/com.atproto.server.describeServer`, { signal: AbortSignal.timeout(3000) });
assert.equal(description.status, 200);
const server = await description.json();
assert.equal(server.inviteCodeRequired, true);
assert.deepEqual(server.availableUserDomains, ['.test']);
const unauthorized = await fetch(`${origin}/xrpc/com.atproto.admin.getInviteCodes`, { signal: AbortSignal.timeout(3000) });
assert.equal(unauthorized.status, 401, 'PDS admin endpoint accepted an unauthenticated request');
const unknownHostStatus = await new Promise((resolve, reject) => {
  const req = request(origin, { headers: { Host: 'unregistered.invalid' }, timeout: 3000 }, response => {
    response.resume(); resolve(response.statusCode);
  });
  req.on('error', reject);
  req.on('timeout', () => req.destroy(new Error('Host allowlist check timed out')));
  req.end();
});
assert.equal(unknownHostStatus, 421);
await new Promise((resolve, reject) => {
  const socket = new WebSocket('ws://localhost:2583/xrpc/com.atproto.sync.subscribeRepos');
  const timeout = setTimeout(() => { socket.close(); reject(new Error('PDS WebSocket handshake timed out')); }, 5000);
  socket.addEventListener('open', () => { clearTimeout(timeout); socket.close(); resolve(); }, { once: true });
  socket.addEventListener('error', () => { clearTimeout(timeout); reject(new Error('PDS WebSocket failed')); }, { once: true });
});
console.log('Invite gate, admin authentication, isolated handle domain, host allowlist and WebSocket ingress passed.');
