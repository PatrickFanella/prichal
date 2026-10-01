// Read-only incoming offer qualification against fixed disposable local tenants.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/incoming-offers/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const instances={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return Object.values(instances).map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
function counts(){return command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT json_build_object('audit',(SELECT count(*) FROM organization_access_events),'jobs',(SELECT count(*) FROM river_job),'lifecycle',(SELECT count(*) FROM service_operations))`]);}
function organization(instance){const id=command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT organization_id FROM deployments WHERE id='${instance}'`]);assert.match(id,/^[a-f0-9-]{36}$/);return id;}
const before=custody(),auditBefore=counts(),results=[];
for(const actor of ['alice','bob']) {
 const client=await localOwnerClient(actor),legacy=await client.api('/api/team/incoming');assert.equal(legacy.status,200);
 for(const kind of ['invitations','transfers']) {
  const path='/api/team/incoming/'+kind+'/page',response=await client.api(path);assert.equal(response.status,200);assert.ok(Array.isArray(response.value.items)&&response.value.items.length<=50);assert.equal(response.value.hasMore,false);assert.equal(response.value.nextAfter,null);
  const normalize=items=>items.map(item=>({...item,expiresAt:new Date(item.expiresAt).toISOString()})).toSorted((a,b)=>a.id.localeCompare(b.id));assert.deepEqual(normalize(response.value.items),normalize(legacy.value[kind]));assert.deepEqual((await client.api(path)).value,response.value);
  for(const item of response.value.items){assert.deepEqual(Object.keys(item).sort(),kind==='invitations'?['expiresAt','id','organizationId','role']:['expiresAt','id','organizationId']);assert.ok(!/PRIVATE|password|ciphertext|did:plc:|endpoint|@|subject|issuer|idempotency/.test(JSON.stringify(item)));}
  for(const query of ['after=','after=!','after=x&after=y','unknown=x','after=%zz'])assert.equal((await client.api(path+'?'+query)).status,400);
  const wrong=Buffer.from(JSON.stringify({actorId:actor==='alice'?'897591fd-fe4c-4778-8da6-52bd4b67eb93':'ab452853-262b-4c70-9ccd-40eb5bcbb5b5',kind,createdAt:'2026-01-01T00:00:00.123456Z',id:instances[actor]})).toString('base64url');assert.equal((await client.api(path+'?after='+wrong)).status,400);
  results.push({actor,kind,offers:response.value.items.length,pages:1});
 }
}
const denied=await fetch('http://localhost:5173/api/team/incoming/invitations/page',{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
assert.equal(counts(),auditBefore);assert.deepEqual(custody(),before);
const report={completedAt:new Date().toISOString(),results,stableReadReplay:true,legacyProjectionCompatible:true,malformedAndCrossIdentityCursorsDenied:true,cliScopeDenied:true,auditAndJobCountsUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
