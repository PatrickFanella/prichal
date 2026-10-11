// Fixed private Kvant management qualification. Never targets production accounts.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { existsSync, lstatSync, readFileSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const origin='http://127.0.0.1:5177',issuer='http://localhost:5558';
const node='1ef77d45-c6d5-4bdb-b77b-ba1103b45a66';
const root=homedir()+'/.local/state/prichal-staging-management';
const control=homedir()+'/.local/state/prichal-staging-control';
const scopePath=root+'/qualification-scope.json',reportPath=root+'/qualification-report.json';
function command(program,args,options={}) { const r=spawnSync(program,args,{encoding:'utf8',timeout:30000,...options});assert.equal(r.status,0,'Private staging operator command failed');return r.stdout.trim(); }
function sql(query){return command('docker',['exec','prichal-staging-registry-postgres-1','psql','-U','pds_staging','-d','pds_local','-At','-v','ON_ERROR_STOP=1','-c',query]);}
const client=await localOwnerClient('alice',{origin,issuer});
const inventory=await client.api('/api/operator/nodes');assert.equal(inventory.status,200);assert.equal(inventory.value.items.length,1);assert.equal(inventory.value.items[0].id,node);
const other=await localOwnerClient('bob',{origin,issuer});assert.equal((await other.api('/api/operator/nodes')).status,404);
assert.equal((await fetch(origin+'/api/operator/nodes',{headers:{Authorization:'Bearer private-fixture'},signal:AbortSignal.timeout(5000)})).status,403);
let scope;
if(existsSync(scopePath)){
 const info=lstatSync(scopePath);assert.ok(info.isFile()&&!(info.mode&0o077));scope=JSON.parse(readFileSync(scopePath,'utf8'));
}else{
 assert.equal(inventory.value.items[0].available,1,'Start with one freshly verified empty slot');
 const org=await client.api('/api/organizations',{name:'Kvant staging'});assert.equal(org.status,201);
 scope={node,organization:org.value.id,requestKey:randomUUID(),grantKey:randomUUID(),name:'kvant-staging'};
 writeFileSync(scopePath,JSON.stringify(scope,null,2)+'\n',{flag:'wx',mode:0o600});
}
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
assert.equal(scope.node,node);for(const field of ['organization','requestKey','grantKey'])assert.ok(uuid.test(scope[field]));assert.equal(scope.name,'kvant-staging');
const env=Object.fromEntries(readFileSync(control+'/admin.env','utf8').trim().split('\n').map(line=>{const i=line.indexOf('=');return[line.slice(0,i),line.slice(i+1)];}));
command(control+'/bin/entitlement-admin',['grant','--organization',scope.organization,'--key',scope.grantKey,'--reason','qualification','--plan','local-single','--hours','24'],{env:{...process.env,PDS_MODE:'local',PDS_DATABASE_URL:env.PDS_DATABASE_URL}});
const request={kind:'agent',name:scope.name,region:'local',idempotencyKey:scope.requestKey};
const submitted=await client.api(`/api/organizations/${scope.organization}/deployments`,request);assert.equal(submitted.status,200);const id=submitted.value.id;assert.ok(uuid.test(id));
if(scope.deployment)assert.equal(scope.deployment,id);else{scope.deployment=id;writeFileSync(scopePath,JSON.stringify(scope,null,2)+'\n',{mode:0o600});}
let op=submitted.value;
for(let attempt=0;attempt<75&&op.state!=='ready';attempt++){
 assert.notEqual(op.state,'failed','Retain failed staging scope and inspect the private worker; do not create another request');
 await new Promise(resolve=>setTimeout(resolve,2000));const result=await client.api('/api/deployments/'+id);assert.equal(result.status,200);op=result.value;
}
assert.equal(op.state,'ready','Staging readiness not confirmed within bounded qualification');
const replay=await client.api(`/api/organizations/${scope.organization}/deployments`,request);assert.equal(replay.status,200);assert.equal(replay.value.id,id);
assert.equal((await other.api('/api/deployments/'+id)).status,404);
const placement=sql(`SELECT node_id::text FROM instance_placements WHERE deployment_id='${id}'`);assert.equal(placement,node);
assert.equal(sql(`SELECT count(*) FROM instance_placements WHERE deployment_id='${id}'`),'1');
const report={completedAt:new Date().toISOString(),node,organization:scope.organization,deployment:id,state:op.state,endpoint:op.endpoint,operatorInventory:true,ordinaryOwnerDenied:true,cliInventoryDenied:true,ownerIsolation:true,retainedReplay:true,placementCount:1,publicHostingQualified:false};
writeFileSync(reportPath,JSON.stringify(report,null,2)+'\n',{mode:0o600});console.log(JSON.stringify(report));
