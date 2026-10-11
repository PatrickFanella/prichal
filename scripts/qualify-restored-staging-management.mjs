// Read-only qualification inside the isolated restore network namespace.
import assert from 'node:assert/strict';
import { localOwnerClient } from './local-owner-client.mjs';
const origin='http://127.0.0.1:5177',issuer='http://localhost:5558';
const deployment='53a1f2d3-181b-4cf2-b242-2d35ca55c784';
const alice=await localOwnerClient('alice',{origin,issuer});
const nodes=await alice.api('/api/operator/nodes');assert.equal(nodes.status,200);
assert.equal(nodes.value.items.length,1);
const node=nodes.value.items[0];assert.equal(node.id,'1ef77d45-c6d5-4bdb-b77b-ba1103b45a66');
assert.equal(node.reserved,1);assert.equal(node.available,0);assert.equal(node.fresh,false);
const restored=await alice.api('/api/deployments/'+deployment);
assert.equal(restored.status,200);assert.equal(restored.value.id,deployment);assert.equal(restored.value.state,'ready');
const bob=await localOwnerClient('bob',{origin,issuer});
assert.equal((await bob.api('/api/operator/nodes')).status,404);
assert.equal((await bob.api('/api/deployments/'+deployment)).status,404);
assert.equal((await fetch(origin+'/api/operator/nodes',{headers:{Authorization:'Bearer restore-fixture'},signal:AbortSignal.timeout(5000)})).status,403);
console.log(JSON.stringify({restoredOwnerSignIn:true,restoredDeployment:deployment,placementReserved:node.reserved,staleMeasurementRefused:true,ownerIsolation:true,operatorAuthorization:true}));
