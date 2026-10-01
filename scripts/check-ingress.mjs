import http from 'node:http';
import { randomBytes } from 'node:crypto';
const [endpoint] = process.argv.slice(2);
const target = new URL(endpoint ?? '');
if (target.protocol !== 'http:' || !/^[a-f0-9-]{36}\.localhost$/.test(target.hostname) || !/^\d{5}$/.test(target.port)) throw new Error('Supply a local managed PDS endpoint');
function request(host) {
  return new Promise((resolve, reject) => {
    const req = http.get({ hostname: '127.0.0.1', port: target.port, path: '/xrpc/_health', headers: { Host: host }, timeout: 5000 }, response => {
      let body = ''; response.on('data', chunk => { body += chunk; }); response.on('end', () => resolve({ status: response.statusCode, body }));
    }); req.on('error', reject); req.on('timeout', () => req.destroy(new Error('Health timeout')));
  });
}
const health = await request(target.host);
if (health.status !== 200 || JSON.parse(health.body).version !== '0.4.5036') throw new Error('Managed ingress health failed');
if ((await request('unregistered.test')).status !== 421) throw new Error('Unknown host was accepted');
await new Promise((resolve, reject) => {
  const req = http.request({ hostname: '127.0.0.1', port: target.port, path: '/xrpc/com.atproto.sync.subscribeRepos', headers: { Host: target.host, Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': randomBytes(16).toString('base64') }, timeout: 5000 });
  req.on('upgrade', (response, socket) => { socket.destroy(); if (response.statusCode === 101) resolve(); else reject(new Error('WebSocket upgrade failed')); });
  req.on('response', response => { response.resume(); reject(new Error('WebSocket upgrade rejected')); });
  req.on('error', reject); req.on('timeout', () => req.destroy(new Error('WebSocket timeout'))); req.end();
});
console.log('Managed local health, unknown-host rejection and WebSocket upgrade passed.');
