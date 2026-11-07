import { test } from 'node:test';
import assert from 'node:assert/strict';
import { deploymentEndpoint } from '../web/src/deployment-contract.ts';
const id = 'e4067ea1-f000-4321-852f-0c67a179c0ed';
const local = `${id}.localhost`;
const managed = 'pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv';
test('Local deployment endpoints keep their private loopback shape', () => {
  assert.equal(deploymentEndpoint('', local, id), true);
  assert.equal(deploymentEndpoint(`https://${local}:44374`, local, id), true);
  for (const bad of [`https://${local}`, `https://${local}:35999`, `https://${local}:46000`, `http://${local}:44374`, `https://${local}:44374/xrpc`, `https://user@${local}:44374`, `https://other.localhost:44374`]) assert.equal(deploymentEndpoint(bad, local, id), false, bad);
});
test('A ready managed deployment lists its generated public endpoint', () => {
  assert.equal(deploymentEndpoint(`https://${managed}`, managed, id), true);
  assert.equal(deploymentEndpoint(`https://${managed}/`, managed, id), true);
  const other = 'pds-aaaaaaaaaaaa4aaa8aaaaaaaaaaaaaaa.subcult.tv';
  for (const [value, hostname] of [[`https://${managed}:8443`, managed], [`http://${managed}`, managed], [`https://${managed}/xrpc`, managed], [`https://${managed}?x=1`, managed], [`https://${other}`, other], [`https://${managed}`, other], ['https://example.com', 'example.com'], ['not a url', managed]]) assert.equal(deploymentEndpoint(value, hostname, id), false, value);
  assert.equal(deploymentEndpoint(`https://${managed}`, managed, 7), false);
});
