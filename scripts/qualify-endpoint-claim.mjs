// Fixed disposable local-namespace claim refusal. No public domain reservation or native deployment is accepted.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const id=randomUUID(),root='.local/endpoint-claim/'+id;mkdirSync(root,{recursive:true,mode:0o700});
function command(args){const r=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(r.status,0,'Local intent inventory failed');return r.stdout.trim();}
function sql(query){return JSON.parse(command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',query]));}
const instances=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade','8d234722-f0ad-47b2-8ef8-10f85d0c13b9'],nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return instances.map(instance=>({id:instance,native:JSON.parse(command(['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+instance,'pds-ass-edge-'+instance,'pds-ass-enrollment-'+instance])).map(v=>({id:v.Id,running:v.State.Running,startedAt:v.State.StartedAt})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+instance+suffix)).digest('hex'))}));}
function unchanged(){return sql(`SELECT json_build_object('deployments',(SELECT json_agg(row_to_json(d) ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT count(*) FROM enrollment_operations),'events',(SELECT count(*) FROM enrollment_events),'orgAudits',(SELECT count(*) FROM organization_access_events),'deploymentAdmission',(SELECT open FROM deployment_admission),'enrollmentAdmission',(SELECT open FROM enrollment_admission))`);}
function choices(){return sql(`SELECT json_build_object('choices',(SELECT count(*) FROM endpoint_choices),'pending',(SELECT count(*) FROM endpoint_choices WHERE state='awaiting_ownership'),'audits',(SELECT count(*) FROM endpoint_choice_events),'claims',(SELECT count(*) FROM endpoint_claims),'claimAudits',(SELECT count(*) FROM endpoint_claim_events))`);}

const bob=await localOwnerClient('bob'),alice=await localOwnerClient('alice'),org='fdf69346-856d-4130-83a6-390f321bede9',path='/api/organizations/'+org;
const before=custody(),workBefore=unchanged(),countBefore=choices();
const body={hostname:'pds.claim-local.test',applicationHosts:['app.claim-app.test'],idempotencyKey:randomUUID(),confirmed:true},cancelKey=randomUUID();
writeFileSync(root+'/request.json',JSON.stringify({org,body,cancelKey}),{mode:0o600,flag:'wx'});
assert.equal((await bob.api(path+'/endpoint-choice')).value.choice,null,'Do not replace an existing pending choice');
let c;
try {
 const saved=await bob.api(path+'/endpoint-choices',body);assert.equal(saved.status,200);c=saved.value;
 const endpoint=path+'/endpoint-choices/'+c.id+'/claim';
 for(let i=0;i<2;i++){const r=await bob.api(endpoint,{});assert.equal(r.status,409);assert.equal(typeof r.value.error,'string');assert.equal(Object.hasOwn(r.value,'proofValue'),false);assert.equal(JSON.stringify(r.value).includes(c.proofValue),false);}
 assert.equal((await bob.api(endpoint)).status,404,'Unconfirmed namespace acquired a claim receipt');
 assert.equal((await alice.api(endpoint,{})).status,404);
 assert.equal((await bob.api(endpoint,{resolver:'PRIVATE'})).status,400);
 const noCSRF=await bob.call(endpoint,{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json'},body:'{}'});assert.equal(noCSRF.status,403);
 const cli=await fetch('http://localhost:5173'+endpoint,{method:'POST',headers:{Authorization:'Bearer '+'a'.repeat(43),'Content-Type':'application/json'},body:'{}',signal:AbortSignal.timeout(5000)});assert.equal(cli.status,403);
 assert.deepEqual((await bob.api(path+'/endpoint-choices/'+c.id)).value,c,'Claim refusal mutated saved choice');
 assert.deepEqual(unchanged(),workBefore);assert.equal(choices().audits,countBefore.audits+1,'Read created an audit');
}finally{
 if(!c){const retry=await bob.api(path+'/endpoint-choices',body);if(retry.status===200)c=retry.value;}
 if(c){const cancelled=await bob.api(path+'/endpoint-choices/'+c.id+'/cancel',{idempotencyKey:cancelKey,confirmed:true});assert.equal(cancelled.status,200);assert.equal(cancelled.value.state,'cancelled');}
}
const retained=await bob.api(path+'/endpoint-choices/'+c.id+'/claim',{});assert.equal(retained.status,409);assert.equal((await bob.api(path+'/endpoint-choices/'+c.id+'/claim')).status,404);
assert.equal((await bob.api(path+'/endpoint-choice')).value.choice,null);
const after=choices();assert.equal(after.choices,countBefore.choices+1);assert.equal(after.audits,countBefore.audits+2);assert.equal(after.pending,countBefore.pending);assert.equal(after.claims,countBefore.claims);assert.equal(after.claimAudits,countBefore.claimAudits);
assert.deepEqual(unchanged(),workBefore);assert.deepEqual(custody(),before);
const report={id,completedAt:new Date().toISOString(),choiceId:c.id,localNamespaceClaimRefused:true,cancelledChoiceClaimRefused:true,noClaimOrClaimAuditMutation:true,foreignScopeAndMissingCSRFDenied:true,customerResolverAndCLIRejected:true,publicDNSMatchingAndPositiveReservationUnqualified:true,allCreatedChoicesCancelled:true,effectiveAudits:2,managementAndHostWorkUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
