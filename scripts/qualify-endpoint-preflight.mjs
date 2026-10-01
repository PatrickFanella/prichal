// Read-only custom endpoint preflight qualification against fixed disposable local tenants.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/endpoint-preflight/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const instances={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return Object.values(instances).map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
function counts(){return command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT json_build_object('audit',(SELECT count(*) FROM organization_access_events),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT count(*) FROM enrollment_operations),'deployments',(SELECT json_agg(json_build_object('id',id,'endpoint',endpoint_url,'hostname',managed_hostname) ORDER BY id) FROM deployments),'admission',(SELECT open FROM enrollment_admission))`]);}
function organization(instance){const id=command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT organization_id FROM deployments WHERE id='${instance}'`]);assert.match(id,/^[a-f0-9-]{36}$/);return id;}
const before=custody(),auditBefore=counts(),results=[];
const request={hostname:'PDS.Creator-Hosting.test',applicationHosts:['app.creator-app.test']};
for(const actor of ['alice','bob']) {
 const client=await localOwnerClient(actor),org=organization(instances[actor]),other=organization(instances[actor==='alice'?'bob':'alice']),path='/api/organizations/'+org+'/endpoint-preflight';
 const response=await client.api(path,request);assert.equal(response.status,200);assert.equal(response.value.state,'existing_native_requires_review');assert.equal(response.value.canActivate,false);assert.equal(response.value.choiceSaved,false);assert.equal(response.value.hostname,'pds.creator-hosting.test');assert.equal(response.value.remainingGates.length,6);
 assert.deepEqual((await client.api(path,request)).value,response.value);assert.equal((await client.api('/api/organizations/'+other+'/endpoint-preflight',request)).status,404);
 for(const invalid of [{hostname:'https://creator.test',applicationHosts:[]},{hostname:'127.0.0.1',applicationHosts:[]},{hostname:'creator.test',applicationHosts:['app.creator.test']},{hostname:'creator.test',applicationHosts:Array(9).fill('app.test')},{...request,password:'PRIVATE'}])assert.equal((await client.api(path,invalid)).status,400);
 const noCSRF=await client.call(path,{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json'},body:JSON.stringify(request)});assert.equal(noCSRF.status,403);
 results.push({actor,state:response.value.state,activationAllowed:false,choiceSaved:false});
}
const bob=await localOwnerClient('bob'),empty=await bob.api('/api/organizations/a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c/endpoint-preflight',request);assert.equal(empty.status,200);assert.equal(empty.value.state,'planning_only');assert.equal(empty.value.choiceSaved,false);
const denied=await fetch('http://localhost:5173/api/organizations/'+organization(instances.alice)+'/endpoint-preflight',{method:'POST',headers:{Authorization:'Bearer '+'a'.repeat(43),'Content-Type':'application/json'},body:JSON.stringify(request),signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
assert.equal(counts(),auditBefore);assert.deepEqual(custody(),before);
const report={completedAt:new Date().toISOString(),results,emptyOrganizationPlanningOnly:true,stableReadReplay:true,foreignOrganizationDenied:true,invalidHostnameAndDomainSeparationDenied:true,missingCSRFDenied:true,cliScopeDenied:true,auditAndJobCountsUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
