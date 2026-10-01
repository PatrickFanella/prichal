// Runs in the pinned PDS image; input contains only exported public protocol data.
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { createRequire } = require('node:module');
const root = createRequire('/app/package.json');
const upstream = createRequire(root.resolve('@atproto/pds'));
const { readCar, verifyRepoCar } = upstream('@atproto/repo');
const { cborDecodeMulti } = upstream('@atproto/common');
async function main() {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  const car = Buffer.from(input.car, 'base64');
  await readCar(car); // Checks every block's CID against its bytes by default.
  const repo = await verifyRepoCar(car, input.did, input.key);
  assert.ok(repo.creates.some(write => write.collection === 'app.bsky.feed.post' && write.rkey === input.rkey));
  const events = input.frames.map(frame => cborDecodeMulti(Buffer.from(frame, 'base64')));
  assert.ok(events.some(([header, body]) => header.t === '#commit' && body.repo === input.did &&
    body.ops.some(op => op.path === `app.bsky.feed.post/${input.rkey}`)));
  const wrong = await upstream('@atproto/crypto').Secp256k1Keypair.create();
  await assert.rejects(verifyRepoCar(car, input.did, wrong.did()));
  const tampered = Buffer.from(car);
  tampered[tampered.length - 1] ^= 1;
  await assert.rejects(readCar(tampered));
  console.log('Repository signature, record integrity and subscription commit verified; wrong key and tampered export rejected');
}
main().catch(() => { console.error('Repository or subscription verification failed'); process.exitCode = 1; });
