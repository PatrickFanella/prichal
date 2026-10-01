// Operator-only local recovery files. API and job processes must not load recovery keys.
import { createCipheriv, createDecipheriv, randomBytes, randomUUID } from 'node:crypto';
import { createReadStream, lstatSync, readFileSync, openSync, readSync, writeSync, fsyncSync, closeSync, linkSync, unlinkSync } from 'node:fs';
import { dirname, join } from 'node:path';
const magic = Buffer.from('PDSBK001');
const maximumBytes = 512 * 1024 * 1024;
function key(path) {
  const info = lstatSync(path);
  if (!info.isFile() || (info.mode & 0o077) || info.size !== 32) throw new Error('Recovery key must be a private 32-byte regular file');
  return readFileSync(path);
}
function source(path) {
  const info = lstatSync(path);
  if (!info.isFile() || info.size > maximumBytes) throw new Error('Local recovery input must be a bounded regular file');
  return info.size;
}
function metadataValid(value) {
  return value?.version === 1 && /^[a-f0-9-]{36}$/.test(value.id) &&
    ['pds-local', 'management-local', 'export-local'].includes(value.scope) &&
    typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt)) &&
    Object.keys(value).sort().join(',') === 'createdAt,id,scope,version';
}
async function publish(output, action) {
  const directory = dirname(output);
  const info = lstatSync(directory);
  if (!info.isDirectory() || (info.mode & 0o077)) throw new Error('Recovery output directory must be private');
  const temporary = join(directory, `.recovery-${randomUUID()}.partial`);
  const fd = openSync(temporary, 'wx', 0o600);
  try {
    const result = await action(fd);
    fsyncSync(fd);
    // No replacement, including an existing symlink or previous verified backup.
    linkSync(temporary, output);
    const parent = openSync(directory, 'r');
    try { fsyncSync(parent); } finally { closeSync(parent); }
    return result;
  } finally { closeSync(fd); unlinkSync(temporary); }
}
function write(fd, bytes) {
  let offset = 0;
  while (offset < bytes.length) { const count = writeSync(fd, bytes, offset, bytes.length - offset); if (!count) throw new Error('Incomplete recovery file write'); offset += count; }
}
export async function sealRecovery(input, output, keyPath, metadata) {
  const inputSize = source(input);
  if (!metadataValid(metadata)) throw new Error('Invalid local recovery metadata');
  const nonce = randomBytes(12);
  const description = Buffer.from(JSON.stringify(metadata));
  const length = Buffer.alloc(4); length.writeUInt32BE(description.length);
  const header = Buffer.concat([magic, nonce, length, description]);
  if (inputSize + header.length + 16 > maximumBytes) throw new Error('Local recovery envelope exceeds its size limit');
  const cipher = createCipheriv('aes-256-gcm', key(keyPath), nonce, { authTagLength: 16 });
  cipher.setAAD(header);
  await publish(output, async fd => {
    write(fd, header);
    for await (const chunk of createReadStream(input)) write(fd, cipher.update(chunk));
    write(fd, cipher.final()); write(fd, cipher.getAuthTag());
  });
}
export async function openRecovery(input, output, keyPath) {
  const size = source(input);
  if (size < 40) throw new Error('Invalid local recovery envelope');
  const inputFD = openSync(input, 'r');
  let header, metadata, tag;
  try {
    const prefix = Buffer.alloc(24);
    if (readSync(inputFD, prefix, 0, 24, 0) !== 24 || !prefix.subarray(0, 8).equals(magic)) throw new Error('Invalid local recovery format');
    const length = prefix.readUInt32BE(20);
    if (length > 4096 || size < 24 + length + 16) throw new Error('Invalid local recovery metadata size');
    const description = Buffer.alloc(length);
    if (readSync(inputFD, description, 0, length, 24) !== length) throw new Error('Incomplete local recovery metadata');
    metadata = JSON.parse(description.toString());
    if (!metadataValid(metadata)) throw new Error('Invalid local recovery metadata');
    header = Buffer.concat([prefix, description]);
    tag = Buffer.alloc(16);
    if (readSync(inputFD, tag, 0, 16, size - 16) !== 16) throw new Error('Incomplete local recovery authentication tag');
  } finally { closeSync(inputFD); }
  const decipher = createDecipheriv('aes-256-gcm', key(keyPath), header.subarray(8, 20), { authTagLength: 16 });
  decipher.setAAD(header); decipher.setAuthTag(tag);
  return publish(output, async fd => {
    if (size > header.length + 16) {
      for await (const chunk of createReadStream(input, { start: header.length, end: size - 17 })) write(fd, decipher.update(chunk));
    }
    // Authentication must succeed before plaintext is published at the requested path.
    write(fd, decipher.final());
    return metadata;
  });
}
