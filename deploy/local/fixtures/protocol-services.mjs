// Local qualification only. Trust validation comes from the pinned PDS's upstream PLC library.
import http from 'node:http';
import net from 'node:net';
import { createRequire } from 'node:module';
import { DatabaseSync } from 'node:sqlite';
if (process.env.PDS_FIXTURE_MODE !== 'local') throw new Error('Protocol fixtures require explicit local mode');
const rootRequire = createRequire('/app/package.json');
const require = createRequire(rootRequire.resolve('@atproto/pds'));
const plc = require('@did-plc/lib');
const { cidForCbor } = require('@atproto/common');
const db = new DatabaseSync('/fixture-data/protocol.sqlite');
db.exec(`PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS operations(did TEXT NOT NULL,cid TEXT PRIMARY KEY,operation TEXT NOT NULL,nullified INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS mail(id INTEGER PRIMARY KEY,recipient TEXT NOT NULL,message TEXT NOT NULL,created_at TEXT NOT NULL);`);
const locks = new Map();
async function serialized(did, action) {
  const previous = locks.get(did) ?? Promise.resolve();
  const current = previous.catch(() => {}).then(action);
  locks.set(did, current);
  try { return await current; } finally { if (locks.get(did) === current) locks.delete(did); }
}
function reply(response, status, body) { response.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); response.end(JSON.stringify(body)); }
function fixtureEndpoint(value) { try { const endpoint = new URL(value); return ['http:', 'https:'].includes(endpoint.protocol) && (endpoint.hostname === 'localhost' || endpoint.hostname.endsWith('.localhost')); } catch { return false; } }
async function body(request) {
  let bytes = 0; const chunks = [];
  for await (const chunk of request) { bytes += chunk.length; if (bytes > 65536) throw new Error('Request too large'); chunks.push(chunk); }
  return JSON.parse(Buffer.concat(chunks).toString());
}
const server = http.createServer(async (request, response) => {
  try {
    const url = new URL(request.url, 'http://fixture');
    if (request.method === 'GET' && url.pathname === '/_health') return reply(response, 200, { localFixture: true });
    if (request.method === 'GET' && url.pathname === '/_fixture/mail') {
      const recipient = url.searchParams.get('recipient');
      if (!recipient?.endsWith('.invalid')) return reply(response, 400, { error: 'Fixture recipient required' });
      return reply(response, 200, db.prepare('SELECT id,recipient,message,created_at AS createdAt FROM mail WHERE recipient=? ORDER BY id').all(recipient));
    }
    const match = /^\/(did:plc:[a-z2-7]{24})(?:\/(data|log|log\/audit|log\/last))?$/.exec(decodeURIComponent(url.pathname));
    if (!match) return reply(response, 404, { error: 'Unknown fixture route' });
    const [, did, suffix] = match;
    if (request.method === 'POST' && !suffix) {
      const proposed = await body(request);
      await serialized(did, async () => {
        if (db.prepare('SELECT count(*) AS count FROM operations').get().count >= 1000) throw new Error('Fixture inventory full');
        const data = plc.opToData(did, proposed);
        if (data && Object.values(data.services).some(service => !fixtureEndpoint(service.endpoint))) throw new Error('Nonlocal identity endpoint rejected');
        const active = db.prepare('SELECT cid,operation,created_at FROM operations WHERE did=? AND nullified=0 ORDER BY rowid').all(did);
        const history = await Promise.all(active.map(async entry => ({ cid: await cidForCbor(JSON.parse(entry.operation)), operation: JSON.parse(entry.operation), createdAt: new Date(entry.created_at) })));
        const result = await plc.assureValidNextOp(did, history, proposed);
        const cid = (await cidForCbor(proposed)).toString();
        db.exec('BEGIN IMMEDIATE');
        try {
          for (const nullified of result.nullified) db.prepare('UPDATE operations SET nullified=1 WHERE cid=?').run(nullified.toString());
          db.prepare('INSERT INTO operations(did,cid,operation,created_at) VALUES(?,?,?,?)').run(did, cid, JSON.stringify(proposed), new Date().toISOString());
          db.exec('COMMIT');
        } catch (error) { db.exec('ROLLBACK'); throw error; }
      });
      return reply(response, 200, { accepted: true });
    }
    if (request.method !== 'GET') return reply(response, 405, { error: 'Unsupported method' });
    const entries = db.prepare('SELECT cid,operation,nullified,created_at FROM operations WHERE did=? ORDER BY rowid').all(did);
    if (!entries.length) return reply(response, 404, { error: 'Unknown fixture DID' });
    const active = entries.filter(entry => !entry.nullified);
    const operations = active.map(entry => JSON.parse(entry.operation));
    if (suffix === 'log') return reply(response, 200, operations);
    if (suffix === 'log/audit') return reply(response, 200, entries.map(entry => ({ did, cid: entry.cid, operation: JSON.parse(entry.operation), nullified: !!entry.nullified, createdAt: entry.created_at })));
    if (suffix === 'log/last') return reply(response, 200, operations.at(-1));
    const data = await plc.validateOperationLog(did, operations);
    if (!data) return reply(response, 404, { error: 'Fixture DID is tombstoned' });
    return reply(response, 200, suffix === 'data' ? data : plc.formatDidDoc(data));
  } catch (error) {
    // Log only the exception class and stack frame, never an operation or mail body.
    console.error('Fixture rejection', error?.constructor?.name, error?.stack?.split('\n')[1]?.trim());
    if (!response.headersSent) reply(response, 400, { error: 'Fixture operation rejected' }); else response.destroy();
  }
});
server.requestTimeout = 5000; server.headersTimeout = 5000;
server.listen(2582, '0.0.0.0');
const mailServer = net.createServer(socket => {
  socket.setTimeout(10000, () => socket.destroy()); socket.write('220 local-fixture ESMTP\r\n');
  let buffer = ''; let recipient = ''; let dataMode = false; let lines = [];
  socket.on('data', chunk => {
    buffer += chunk.toString(); if (buffer.length > 65536) return socket.destroy();
    while (buffer.includes('\r\n')) {
      const end = buffer.indexOf('\r\n'); const line = buffer.slice(0, end); buffer = buffer.slice(end + 2);
      if (dataMode) {
        if (line === '.') {
          if (db.prepare('SELECT count(*) AS count FROM mail').get().count >= 1000) { socket.write('452 Fixture mail capture full\r\n'); }
          else { db.prepare('INSERT INTO mail(recipient,message,created_at) VALUES(?,?,?)').run(recipient, lines.join('\r\n'), new Date().toISOString()); socket.write('250 Captured locally\r\n'); }
          lines = []; dataMode = false;
        } else { lines.push(line.startsWith('..') ? line.slice(1) : line); if (lines.reduce((total, value) => total + value.length, 0) > 65536) socket.destroy(); }
      } else if (/^(EHLO|HELO) /i.test(line)) socket.write('250-local-fixture\r\n250 SIZE 65536\r\n');
      else if (/^MAIL FROM:/i.test(line)) { recipient = ''; socket.write('250 Sender accepted\r\n'); }
      else if (/^RCPT TO:/i.test(line)) { const match = /^RCPT TO:<([^>]+\.invalid)>$/i.exec(line); if (!match) socket.write('550 Only local fixture recipients accepted\r\n'); else { recipient = match[1]; socket.write('250 Recipient accepted\r\n'); } }
      else if (/^DATA$/i.test(line) && recipient) { dataMode = true; lines = []; socket.write('354 End with a dot\r\n'); }
      else if (/^RSET$/i.test(line)) { recipient = ''; lines = []; dataMode = false; socket.write('250 Reset\r\n'); }
      else if (/^NOOP$/i.test(line)) socket.write('250 OK\r\n');
      else if (/^QUIT$/i.test(line)) socket.end('221 Closing\r\n');
      else socket.write('502 Unsupported fixture command\r\n');
    }
  });
  socket.on('error', () => {});
});
mailServer.listen(2525, '0.0.0.0');
console.log('Local PLC validation and mail capture ready; public identity and external mail are disabled.');
