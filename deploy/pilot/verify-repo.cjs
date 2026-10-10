// Run inside the pinned pilot PDS image with exported public protocol data on stdin.
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { createRequire } = require('node:module');
const root = createRequire('/app/package.json');
const upstream = createRequire(root.resolve('@atproto/pds'));
const { readCar, verifyRepoCar } = upstream('@atproto/repo');
async function main() {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  assert.equal(input.did, 'did:plc:gh4owm5uqgahod5wzoxqebhk');
  assert.match(input.rkey, /^[234567a-z]{13}$/);
  const car = Buffer.from(input.car, 'base64');
  await readCar(car);
  const repo = await verifyRepoCar(car, input.did, input.key);
  assert.ok(repo.creates.some(write => write.collection === 'app.bsky.feed.post' && write.rkey === input.rkey));
  const wrong = await upstream('@atproto/crypto').Secp256k1Keypair.create();
  await assert.rejects(verifyRepoCar(car, input.did, wrong.did()));
  const tampered = Buffer.from(car);
  tampered[tampered.length - 1] ^= 1;
  await assert.rejects(readCar(tampered));
  console.log(JSON.stringify({ signatureVerified: true, recordIntegrityVerified: true, wrongKeyRejected: true, tamperedExportRejected: true }));
}
main().catch(() => { console.error('Pilot repository verification failed'); process.exitCode = 1; });
