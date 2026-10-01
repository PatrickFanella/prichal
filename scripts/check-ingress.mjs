import https from 'node:https';
import { readFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
const [endpoint, certificatePath] = process.argv.slice(2);
const target = new URL(endpoint ?? '');
if (target.protocol !== 'https:' || !/^[a-f0-9-]{36}\.localhost$/.test(target.hostname) || !/^\d{5}$/.test(target.port) || !certificatePath) throw new Error('Supply a local managed PDS endpoint and its public fixture certificate');
const ca = readFileSync(certificatePath, 'utf8');
if (ca.includes('PRIVATE KEY')) throw new Error('Supply a public fixture certificate only');
const tlsOptions = { ca, servername: target.hostname, minVersion: 'TLSv1.3' };
function request(host, path = '/xrpc/_health') {
  return new Promise((resolve, reject) => {
    const req = https.get({ ...tlsOptions, hostname: '127.0.0.1', port: target.port, path, headers: { Host: host }, timeout: 5000 }, response => {
      let body = ''; response.on('data', chunk => { body += chunk; }); response.on('end', () => resolve({ status: response.statusCode, body }));
    }); req.on('error', reject); req.on('timeout', () => req.destroy(new Error('Health timeout')));
  });
}
const health = await request(target.host);
if (health.status !== 200 || JSON.parse(health.body).version !== '0.4.5036') throw new Error('Managed ingress health failed');
const description = await request(target.host, '/xrpc/com.atproto.server.describeServer');
if (JSON.parse(description.body).did !== `did:web:${target.hostname}%3A${target.port}`) throw new Error('PDS service identity does not match ingress');
const metadata = await request(target.host, '/.well-known/oauth-authorization-server');
if (metadata.status !== 200 || JSON.parse(metadata.body).issuer !== endpoint) throw new Error('Advertised PDS URL does not match ingress');
if ((await request('unregistered.test')).status !== 421) throw new Error('Unknown host was accepted');
await new Promise((resolve, reject) => {
  const req = https.request({ ...tlsOptions, hostname: '127.0.0.1', port: target.port, path: '/xrpc/com.atproto.sync.subscribeRepos', headers: { Host: target.host, Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': randomBytes(16).toString('base64') }, timeout: 5000 });
  req.on('upgrade', (response, socket) => { socket.destroy(); if (response.statusCode === 101) resolve(); else reject(new Error('WebSocket upgrade failed')); });
  req.on('response', response => { response.resume(); reject(new Error('WebSocket upgrade rejected')); });
  req.on('error', reject); req.on('timeout', () => req.destroy(new Error('WebSocket timeout'))); req.end();
});
console.log('Managed local TLS, advertised URL/service identity, health, unknown-host rejection and WebSocket upgrade passed.');
