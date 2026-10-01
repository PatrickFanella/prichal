// Read-only shared server status qualification against fixed disposable local tenants.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/shared-instance-pages/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const instances={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return Object.values(instances).map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
function counts(){return command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT json_build_object('audit',(SELECT count(*) FROM organization_access_events),'jobs',(SELECT count(*) FROM river_job),'lifecycle',(SELECT count(*) FROM service_operations))`]);}
function organization(instance){const id=command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT organization_id FROM deployments WHERE id='${instance}'`]);assert.match(id,/^[a-f0-9-]{36}$/);return id;}
const before=custody(),auditBefore=counts(),results=[];
for(const actor of ['alice','bob']){
 const client=await localOwnerClient(actor),org=organization(instances[actor]),other=organization(instances[actor==='alice'?'bob':'alice']),path='/api/team/organizations/'+org+'/instances/page';
 const response=await client.api(path);assert.equal(response.status,200);assert.ok(Array.isArray(response.value.items)&&response.value.items.length<=50);assert.equal(response.value.hasMore,false);assert.equal(response.value.nextAfter,null);
 const legacy=await client.api('/api/team/organizations/'+org);assert.equal(legacy.status,200);assert.deepEqual(response.value.items,legacy.value.instances);
 assert.deepEqual((await client.api(path)).value,response.value);assert.equal((await client.api('/api/team/organizations/'+other+'/instances/page')).status,404);
 for(const item of response.value.items){assert.deepEqual(Object.keys(item).sort(),['id','kind','name','state']);assert.ok(!/PRIVATE|password|ciphertext|did:plc:|endpoint|@/.test(JSON.stringify(item)));}
 for(const query of ['after=','after=!','after=x&after=y','unknown=x','after=%zz'])assert.equal((await client.api(path+'?'+query)).status,400);
 const wrong=Buffer.from(JSON.stringify({actorId:actor==='alice'?'897591fd-fe4c-4778-8da6-52bd4b67eb93':'ab452853-262b-4c70-9ccd-40eb5bcbb5b5',organizationId:org,kind:'instances',createdAt:'2026-01-01T00:00:00.123456Z',id:instances[actor]})).toString('base64url');assert.equal((await client.api(path+'?after='+wrong)).status,400);
 results.push({actor,instances:response.value.items.length,pages:1});
}
const bob=await localOwnerClient('bob');const empty=await bob.api('/api/team/organizations/a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c/instances/page');assert.equal(empty.status,200);assert.deepEqual(empty.value.items,[]);
const denied=await fetch('http://localhost:5173/api/team/organizations/'+organization(instances.alice)+'/instances/page',{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
assert.equal(counts(),auditBefore);assert.deepEqual(custody(),before);
const report={completedAt:new Date().toISOString(),results,emptyOrganization:true,stableReadReplay:true,legacyProjectionCompatible:true,foreignOrganizationDenied:true,malformedAndCrossIdentityCursorsDenied:true,cliScopeDenied:true,auditAndJobCountsUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
