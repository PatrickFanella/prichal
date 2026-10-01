// Cold-volume inventory. All runtime writers must already be stopped.
const { createHash } = require('node:crypto');
const { readdirSync, lstatSync, createReadStream } = require('node:fs');
const { join, relative } = require('node:path');
const { DatabaseSync } = require('node:sqlite');
async function inventory(root) {
  const files = []; let totalBytes = 0; let databases = 0;
  async function walk(directory) {
    for (const entry of readdirSync(directory).sort()) {
      const path = join(directory, entry); const info = lstatSync(path);
      const name = relative(root, path);
      if (name === 'host/manifest.json') continue; // Inventory cannot hash itself.
      if (info.isDirectory()) { await walk(path); continue; }
      if (!info.isFile()) throw new Error('Snapshot links and special files are unsupported');
      totalBytes += info.size;
      if (info.size > 64 * 1024 * 1024 || totalBytes > 256 * 1024 * 1024 || files.length >= 10000) throw new Error('Local snapshot inventory limit');
      if (path.endsWith('.sqlite') && !process.argv.includes('--files-only')) {
        let db;
        try { db = new DatabaseSync(path, { readOnly: true }); if (db.prepare('PRAGMA integrity_check').get().integrity_check !== 'ok') throw new Error('SQLite integrity failure'); }
        catch (error) {
          const classification = ['readonly', 'locked', 'malformed', 'no such function', 'no such collation', 'unable to open'].find(value => error.message.includes(value)) ?? 'unclassified';
          console.error('Database inventory rejection', name, error.code ?? error.constructor.name, classification); throw error;
        }
        finally { db?.close(); }
        databases++;
      }
      const digest = createHash('sha256');
      for await (const chunk of createReadStream(path)) digest.update(chunk);
      files.push({ path: name, bytes: info.size, sha256: digest.digest('hex') });
    }
  }
  for (const directory of ['data', 'blobs', 'protocol', 'host']) await walk(join(root, directory));
  return { files, totalBytes, databases };
}
inventory('/snapshot').then(value => console.log(JSON.stringify(value))).catch(() => {
  console.error('Cold-volume inventory or SQLite integrity validation failed'); process.exitCode = 1;
});
