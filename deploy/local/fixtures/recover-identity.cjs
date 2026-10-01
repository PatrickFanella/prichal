// Proves recovered PLC material in the restored, isolated directory only.
const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const root = createRequire('/app/package.json');
const upstream = createRequire(root.resolve('@atproto/pds'));
async function main() {
  const did = process.argv[2];
  assert.match(did ?? '', /^did:plc:[a-z2-7]{24}$/);
  assert.equal(process.env.PDS_DID_PLC_URL, 'http://protocol-fixtures:2582');
  assert.equal(process.env.PDS_DEV_MODE, 'true');
  const plc = upstream('@did-plc/lib');
  const { Secp256k1Keypair } = upstream('@atproto/crypto');
  const signer = await Secp256k1Keypair.import(process.env.PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX);
  const client = new plc.Client(process.env.PDS_DID_PLC_URL);
  const original = await client.getLastOp(did);
  assert.ok(original.rotationKeys.includes(signer.did()), 'Recovered signer is not authorized');
  const temporary = await Secp256k1Keypair.create();
  const rotated = await plc.createUpdateOp(original, signer, data => ({ ...data, rotationKeys: [temporary.did(), ...data.rotationKeys] }));
  await client.sendOperation(did, rotated);
  const active = await client.getLastOp(did);
  assert.equal(active.rotationKeys[0], temporary.did());
  const restored = await plc.createUpdateOp(active, temporary, data => ({ ...data, rotationKeys: original.rotationKeys }));
  await client.sendOperation(did, restored);
  assert.deepEqual((await client.getLastOp(did)).rotationKeys, original.rotationKeys);
  console.log('Recovered PLC signer and temporary key rotation/rollback verified in the isolated restored directory');
}
main().catch(() => { console.error('Isolated identity-key recovery failed'); process.exitCode = 1; });
