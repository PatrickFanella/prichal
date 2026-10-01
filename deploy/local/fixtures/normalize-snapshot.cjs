// Operates only on a new staging copy. Never mount a source/runtime volume here.
const { DatabaseSync, backup } = require('node:sqlite');
const { readdirSync, lstatSync, renameSync, existsSync, unlinkSync } = require('node:fs');
const { join } = require('node:path');
async function walk(directory) {
  for (const entry of readdirSync(directory).sort()) {
    if (entry.endsWith('.sqlite-wal') || entry.endsWith('.sqlite-shm')) continue;
    const path = join(directory, entry); const info = lstatSync(path);
    if (info.isDirectory()) { await walk(path); continue; }
    if (!info.isFile()) throw new Error('Snapshot links or special files rejected');
    if (!path.endsWith('.sqlite')) continue;
    const target = `${path}.normalized`;
    if (existsSync(target)) throw new Error('Incomplete staging normalization exists');
    const source = new DatabaseSync(path, { readOnly: true });
    try { await backup(source, target); } finally { source.close(); }
    const verified = new DatabaseSync(target);
    try { verified.exec('PRAGMA journal_mode=DELETE'); if (verified.prepare('PRAGMA integrity_check').get().integrity_check !== 'ok') throw new Error('SQLite backup integrity failure'); } finally { verified.close(); }
    renameSync(target, path);
    // These are obsolete companions in the staging copy, not runtime/source journals.
    for (const suffix of ['-wal', '-shm']) if (existsSync(path + suffix)) unlinkSync(path + suffix);
  }
}
(async () => { await walk('/snapshot/data'); await walk('/snapshot/protocol'); console.log('Staging SQLite backup normalization passed'); })().catch(() => {
  console.error('Staging SQLite backup normalization failed'); process.exitCode = 1;
});
