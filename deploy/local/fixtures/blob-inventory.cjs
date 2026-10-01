// Bounded signed-record and referenced-blob inventory on the isolated local PDS.
const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const { createHash } = require('node:crypto');
const root = createRequire('/app/package.json');
const upstream = createRequire(root.resolve('@atproto/pds'));
const { verifyRepoCar, verifyRecords } = upstream('@atproto/repo');
const { sha256RawToCid } = upstream('@atproto/common');
async function main() {
  assert.equal(process.env.PDS_DID_PLC_URL, 'http://protocol-fixtures:2582');
  assert.equal(process.env.PDS_DEV_MODE, 'true');
  const pds = 'http://localhost:2583';
  async function get(url) { const response = await fetch(url, { signal: AbortSignal.timeout(15000) }); assert.equal(response.status, 200); return response; }
  const repos = []; let cursor;
  do {
    const page = await (await get(`${pds}/xrpc/com.atproto.sync.listRepos?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`)).json();
    repos.push(...page.repos); assert.ok(repos.length <= 32);
    cursor = page.repos.length ? page.cursor : undefined;
  } while (cursor);
  const result = []; let recordsTotal = 0;
  for (const repo of repos) {
    const identity = await (await get(`${process.env.PDS_DID_PLC_URL}/${repo.did}/data`)).json();
    const car = Buffer.from(await (await get(`${pds}/xrpc/com.atproto.sync.getRepo?did=${repo.did}`)).arrayBuffer());
    assert.ok(car.length <= 16 * 1024 * 1024);
    await verifyRepoCar(car, repo.did, identity.verificationMethods.atproto);
    const records = JSON.parse(JSON.stringify(await verifyRecords(car, repo.did, identity.verificationMethods.atproto)));
    recordsTotal += records.length; assert.ok(recordsTotal <= 10000);
    const references = new Map();
    function walk(value, depth = 0) {
      assert.ok(depth < 64);
      if (!value || typeof value !== 'object') return;
      if (value.$type === 'blob') {
        const cid = value.ref?.$link ?? value.ref?.['/']; // Lexicon JSON or native CAR CID serialization.
        assert.equal(typeof cid, 'string'); assert.ok(Number.isSafeInteger(value.size) && value.size >= 0 && value.size <= 5 * 1024 * 1024);
        references.set(cid, value); return;
      }
      for (const child of Object.values(value)) walk(child, depth + 1);
    }
    for (const record of records) walk(record.record);
    for (const [cid, reference] of references) {
      const response = await get(`${pds}/xrpc/com.atproto.sync.getBlob?did=${repo.did}&cid=${cid}`);
      const bytes = Buffer.from(await response.arrayBuffer()); assert.equal(bytes.length, reference.size);
      const digest = createHash('sha256').update(bytes).digest();
      assert.equal(sha256RawToCid(digest).toString(), cid);
      result.push({ did: repo.did, cid, bytes: bytes.length, mimeType: reference.mimeType, sha256: digest.toString('hex') });
    }
  }
  console.log(JSON.stringify({ repositories: repos.length, signedRecords: recordsTotal, referencedBlobs: result.sort((a, b) => `${a.did}/${a.cid}`.localeCompare(`${b.did}/${b.cid}`)) }));
}
main().catch(error => { console.error('Isolated signed repository or referenced blob inventory failed', error.constructor.name, error.stack?.split('\n').find(line => line.trim().startsWith('at '))?.trim()); process.exitCode = 1; });
