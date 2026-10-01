// Read-only owner history qualification against fixed disposable local tenants.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/owner-journey/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const instances={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return Object.values(instances).map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
const eventsSQL=`SELECT json_build_object('deployment',(SELECT count(*) FROM deployment_events),'enrollment',(SELECT count(*) FROM enrollment_events),'domain',(SELECT count(*) FROM handle_events),'lifecycle',(SELECT count(*) FROM service_operation_events),'export',(SELECT count(*) FROM export_events),'exit',(SELECT count(*) FROM hosting_exit_events))`;
function counts(){return command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',eventsSQL]);}
const before=custody(),auditBefore=counts(),results=[],kinds=new Set();
for(const owner of ['alice','bob']){
 const client=await localOwnerClient(owner),deployment=instances[owner],other=instances[owner==='alice'?'bob':'alice'];
 const path=`/api/deployments/${deployment}/journey`;const latest=await client.api(path);assert.equal(latest.status,200);
 const replay=await client.api(path);assert.equal(replay.status,200);assert.deepEqual(replay.value,latest.value,'Read replay changed recorded history');
 const foreign=await client.api(`/api/deployments/${other}/journey`);assert.equal(foreign.status,404);
 assert.equal((await client.api(path+'?before=!')).status,400);assert.equal((await client.api(path+'?unknown=x')).status,400);
 const seen=new Set();let page=latest.value,pages=0;
 for(;;){pages++;assert.ok(Array.isArray(page.items)&&page.items.length<=50);for(const event of page.items){assert.ok(!seen.has(event.key),'Pagination repeated an event');seen.add(event.key);kinds.add(event.category);assert.ok(event.message&&event.nextStep);assert.ok(!/did:plc:|idempotency|oidc-owner:|accountHandle|ciphertext|password|token/i.test(JSON.stringify(event)),'Private field escaped history');assert.ok(!JSON.stringify(event).includes('PRIVATE'),'Private marker escaped history');assert.deepEqual(Object.keys(event).sort(),['category','createdAt','key','message','nextStep','state']);}
  if(!page.hasMore){assert.equal(page.nextBefore,null);break;}assert.ok(page.nextBefore&&page.nextBefore.length<=256);assert.equal((await client.api(`/api/deployments/${other}/journey?before=${encodeURIComponent(page.nextBefore)}`)).status,400);
  const next=await client.api(path+'?before='+encodeURIComponent(page.nextBefore));assert.equal(next.status,200);page=next.value;assert.ok(pages<100);
 }
 results.push({owner,events:seen.size,pages});
}
const denied=await fetch(`http://localhost:5173/api/deployments/${instances.alice}/journey`,{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
assert.equal(counts(),auditBefore,'History reading added audit work');assert.deepEqual(custody(),before,'History reading changed native custody');
const report={completedAt:new Date().toISOString(),results,categories:[...kinds].sort(),ownerIsolation:true,stableReadReplay:true,boundedPagination:true,crossInstanceCursorDenied:true,cliScopeDenied:true,auditCountsUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
