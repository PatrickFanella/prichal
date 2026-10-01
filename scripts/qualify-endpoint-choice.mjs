// Fixed disposable local owner-intent drill. No DNS lookup or native deployment is accepted.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const id=randomUUID(),root='.local/endpoint-choice/'+id;mkdirSync(root,{recursive:true,mode:0o700});
function command(args){const r=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(r.status,0,'Local intent inventory failed');return r.stdout.trim();}
function sql(query){return JSON.parse(command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',query]));}
const instances=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade','8d234722-f0ad-47b2-8ef8-10f85d0c13b9'],nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return instances.map(instance=>({id:instance,native:JSON.parse(command(['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+instance,'pds-ass-edge-'+instance,'pds-ass-enrollment-'+instance])).map(v=>({id:v.Id,running:v.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+instance+suffix)).digest('hex'))}));}
function unchanged(){return sql(`SELECT json_build_object('deployments',(SELECT json_agg(row_to_json(d) ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT count(*) FROM enrollment_operations),'events',(SELECT count(*) FROM enrollment_events),'orgAudits',(SELECT count(*) FROM organization_access_events),'deploymentAdmission',(SELECT open FROM deployment_admission),'enrollmentAdmission',(SELECT open FROM enrollment_admission))`);}
function choices(){return sql(`SELECT json_build_object('choices',(SELECT count(*) FROM endpoint_choices),'pending',(SELECT count(*) FROM endpoint_choices WHERE state='awaiting_ownership'),'audits',(SELECT count(*) FROM endpoint_choice_events))`);}
const bob=await localOwnerClient('bob'),alice=await localOwnerClient('alice'),before=custody(),workBefore=unchanged(),countBefore=choices();
const organizations=['fdf69346-856d-4130-83a6-390f321bede9','a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c'];
const request={hostname:'PDS.creator-hosting.test',applicationHosts:['app.creator-app.test'],confirmed:true},proofs=[],receipts=[],intents=[];
try {
for(const org of organizations){
 const path='/api/organizations/'+org,body={...request,idempotencyKey:randomUUID()},cancelKey=randomUUID();
 writeFileSync(root+'/'+org+'-request.json',JSON.stringify({org,body,cancelKey}),{mode:0o600,flag:'wx'});
 const current=await bob.api(path+'/endpoint-choice');assert.equal(current.status,200);assert.equal(current.value.choice,null,'Do not replace an existing owner choice');
 const intent={org,path,body,cancelKey,choice:null};intents.push(intent);
 let choice;
 {
  const response=await bob.api(path+'/endpoint-choices',body);assert.equal(response.status,200);choice=response.value;intent.choice=choice;
  assert.equal(choice.state,'awaiting_ownership');assert.equal(choice.canActivate,false);assert.equal(choice.proofName,'_pdsaas.pds.creator-hosting.test');assert.match(choice.proofValue,/^pdsaas-domain-v1=[a-zA-Z0-9_-]{43}$/);proofs.push(choice.proofValue);
  assert.deepEqual((await bob.api(path+'/endpoint-choices',body)).value,choice);
  assert.deepEqual((await bob.api(path+'/endpoint-choice')).value.choice,choice);
  assert.deepEqual((await bob.api(path+'/endpoint-choices/'+choice.id)).value,choice);
  assert.equal((await bob.api(path+'/endpoint-choices',{...body,hostname:'another-hosting.test'})).status,409);
  assert.equal((await bob.api(path+'/endpoint-choices',{...body,idempotencyKey:randomUUID()})).status,409);
  assert.equal((await alice.api(path+'/endpoint-choices/'+choice.id)).status,404);
  assert.equal((await alice.api(path+'/endpoint-choice')).status,404);
  assert.equal((await bob.api(path+'/deployments',{kind:'agent',name:'custom-pending',region:'local',idempotencyKey:randomUUID()})).status,409);
  assert.equal((await bob.api(path+'/endpoint-choices/'+choice.id+'/cancel',{idempotencyKey:cancelKey,confirmed:false})).status,400);
  const noCSRF=await bob.call(path+'/endpoint-choices/'+choice.id+'/cancel',{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json'},body:JSON.stringify({idempotencyKey:cancelKey,confirmed:true})});assert.equal(noCSRF.status,403);
 }
}
assert.notEqual(proofs[0],proofs[1]);
assert.equal(choices().pending,countBefore.pending+2,'Both identical hostnames must remain independently pending');
} finally {
 for(const intent of intents){
  const {path,body,cancelKey,org}=intent;
  let choice=intent.choice;
  // A lost response is resolved only by replaying this exact retained request.
  if(!choice){const retry=await bob.api(path+'/endpoint-choices',body);if(retry.status===200)choice=retry.value;}
  if(!choice)continue;
  const cancel={idempotencyKey:cancelKey,confirmed:true},response=await bob.api(path+'/endpoint-choices/'+choice.id+'/cancel',cancel);
  assert.equal(response.status,200);assert.equal(response.value.state,'cancelled');
  assert.deepEqual((await bob.api(path+'/endpoint-choices/'+choice.id+'/cancel',cancel)).value,response.value);
  assert.equal((await bob.api(path+'/endpoint-choice')).value.choice,null);
  assert.equal((await bob.api(path+'/endpoint-choices',body)).value.state,'cancelled');
  assert.equal((await bob.api(path+'/endpoint-choices/'+choice.id+'/cancel',{idempotencyKey:randomUUID(),confirmed:true})).status,409);
  receipts.push({organizationId:org,choiceId:choice.id,state:'cancelled'});
 }
}

const nativeOrg=sql(`SELECT to_json(organization_id) FROM deployments WHERE id='${instances[1]}'`);
assert.equal((await bob.api('/api/organizations/'+nativeOrg+'/endpoint-choices',{...request,idempotencyKey:randomUUID()})).status,409);
const cli=await fetch('http://localhost:5173/api/organizations/'+organizations[0]+'/endpoint-choice',{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(cli.status,403);
assert.deepEqual(unchanged(),workBefore);assert.deepEqual(custody(),before);
const after=choices();assert.equal(after.choices,countBefore.choices+2);assert.equal(after.audits,countBefore.audits+4);assert.equal(after.pending,countBefore.pending);
const report={id,completedAt:new Date().toISOString(),receipts,stableSaveReadCancelReplay:true,changedPayloadAndForeignScopeDenied:true,unverifiedDomainsNotGloballyReserved:true,newNativeFallbackDenied:true,existingNativeChoiceDenied:true,missingCSRFDenied:true,cliScopeDenied:true,effectiveAudits:4,allCreatedChoicesCancelled:true,managementAndHostWorkUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
