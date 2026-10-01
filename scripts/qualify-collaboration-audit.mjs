// Read-only owner history qualification against fixed disposable local tenants.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/collaboration-audit/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const instances={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return Object.values(instances).map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
const org='a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c';
function counts(){return command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT json_build_object('audit',(SELECT count(*) FROM organization_access_events),'jobs',(SELECT count(*) FROM river_job),'lifecycle',(SELECT count(*) FROM service_operations))`]);}
const alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob'),before=custody(),auditBefore=counts(),path='/api/team/organizations/'+org+'/audit';
const response=await bob.api(path);assert.equal(response.status,200);assert.equal(response.value.items.length,8);assert.equal(response.value.hasMore,false);assert.equal(response.value.nextBefore,null);
for(let i=1;i<response.value.items.length;i++)assert.ok(BigInt(response.value.items[i-1].id)>BigInt(response.value.items[i].id),'History must descend by numeric event ID');
assert.deepEqual((await bob.api(path)).value,response.value);assert.equal((await alice.api(path)).status,404);
for(const query of ['before=','before=!','before=x&before=y','unknown=x','before=%zz'])assert.equal((await bob.api(path+'?'+query)).status,400);
const cross=Buffer.from('fdf69346-856d-4130-83a6-390f321bede9:1').toString('base64url');assert.equal((await bob.api(path+'?before='+cross)).status,400);
const denied=await fetch('http://localhost:5173'+path,{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
for(const item of response.value.items){assert.deepEqual(Object.keys(item).sort(),['action','actorId','createdAt','id','role','targetId']);assert.ok(!/PRIVATE|password|ciphertext|did:plc:|@/.test(JSON.stringify(item)));}
assert.equal(counts(),auditBefore);assert.deepEqual(custody(),before);
const report={completedAt:new Date().toISOString(),events:8,pages:1,stableReadReplay:true,formerOwnerDenied:true,malformedAndCrossOrganizationCursorsDenied:true,cliScopeDenied:true,auditAndJobCountsUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
