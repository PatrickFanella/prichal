// Fixed disposable local admission drill. No new first-account request is accepted.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const id=randomUUID(),root='.local/enrollment-admission/'+id;
mkdirSync(root,{recursive:true,mode:0o700});
function command(file,args){const result=spawnSync(file,args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local admission command failed');return result.stdout.trim();}
function sql(query){return JSON.parse(command('docker',['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',query]));}
function admin(state){return JSON.parse(command('./scripts/dev.sh',['enrollment-admin',...(state?['set','--state',state,'--reason','local_qualification']:['status'])]));}
const instances=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade','8d234722-f0ad-47b2-8ef8-10f85d0c13b9'];
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return instances.map(instance=>({id:instance,native:JSON.parse(command('docker',['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),containers:JSON.parse(command('docker',['inspect','pds-ass-tenant-'+instance,'pds-ass-edge-'+instance,'pds-ass-enrollment-'+instance])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+instance+suffix)).digest('hex'))}));}
function counts(){return sql(`SELECT json_build_object('operations',(SELECT count(*) FROM enrollment_operations),'events',(SELECT count(*) FROM enrollment_events),'jobs',(SELECT count(*) FROM river_job),'lifecycle',(SELECT count(*) FROM service_operations),'deploymentPolicy',(SELECT open FROM deployment_admission),'deploymentAudits',(SELECT count(*) FROM deployment_admission_events),'audits',(SELECT count(*) FROM enrollment_admission_events))`);}
assert.equal(admin().open,true,'Do not alter a previously closed enrollment policy');
const original=sql(`SELECT json_build_object('id',id,'key',idempotency_key,'recover',recovery_requested) FROM enrollment_operations WHERE deployment_id='${instances[0]}' ORDER BY created_at DESC,id DESC LIMIT 1`);
const alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob'),before=custody(),beforeCounts=counts();
let restored=false;
try {
 assert.equal(admin('closed').open,false);
 assert.equal(admin('closed').open,false);
 const path='/api/deployments/'+instances[0]+'/enrollment';
 const request={idempotencyKey:original.key,recover:original.recover};
 const policy=await alice.api(path+'/admission');assert.equal(policy.status,200);assert.equal(policy.value.open,false);assert.deepEqual(Object.keys(policy.value).sort(),['open','updatedAt']);
 const retry=await alice.api(path,request);assert.equal(retry.status,200);assert.equal(retry.value.id,original.id);
 for(const recover of [false,true]){const rejected=await alice.api(path,{recover,idempotencyKey:randomUUID()});assert.equal(rejected.status,503);assert.ok(rejected.value.error.includes('Already-issued invitations'));}
 assert.equal((await alice.api(path,{...request,recover:!request.recover})).status,409);
 assert.equal((await bob.api(path,request)).status,404);assert.equal((await bob.api(path+'/admission')).status,404);
 assert.equal((await alice.api(path)).status,200);assert.equal((await alice.api('/api/enrollments/'+original.id)).status,200);
 assert.equal((await alice.api('/api/deployments/'+instances[0]+'/journey')).status,200);
 assert.equal((await alice.api('/api/deployments/'+instances[0]+'/exports')).status,200);
 const during=counts();assert.equal(during.audits,beforeCounts.audits+1);delete during.audits;
 const expected={...beforeCounts};delete expected.audits;assert.deepEqual(during,expected,'Closing created enrollment or native work');
} finally {restored=admin('open').open;assert.equal(restored,true);}
assert.deepEqual(custody(),before,'Enrollment admission drill changed native data or custody');
const after=counts();assert.equal(after.audits,beforeCounts.audits+2);delete after.audits;const expected={...beforeCounts};delete expected.audits;assert.deepEqual(after,expected);
const report={id,completedAt:new Date().toISOString(),closedNewInvitationAndRecoveryRequests:true,stableExistingRetry:true,changedPayloadAndForeignOwnerDenied:true,statusHistoryAndExportReadsAvailable:true,effectiveAudits:2,managementAndHostWorkUnchanged:true,nativeDataContainersAndCustodyUnchanged:true,enrollmentAdmissionRestoredOpen:restored,report:root+'/report.json'};
writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
