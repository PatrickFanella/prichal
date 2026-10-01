// Close/reopen only disposable local management-owner admission. No native signup or deployment.
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {localOwnerClient} from './local-owner-client.mjs';
const id=randomUUID(),root='.local/owner-admission/'+id;mkdirSync(root,{recursive:true,mode:0o700});
function command(program,args){const r=spawnSync(program,args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(r.status,0,'Local owner-admission command failed');return r.stdout.trim();}
function sql(query){return JSON.parse(command('docker',['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-v','ON_ERROR_STOP=1','-c',query]));}
function inventory(){return sql(`SELECT json_build_object('owners',(SELECT json_agg(o ORDER BY id) FROM owners o),'organizations',(SELECT json_agg(o ORDER BY id) FROM organizations o),'deployments',(SELECT json_agg(d ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'enrollment',(SELECT json_agg(e ORDER BY id) FROM enrollment_operations e),'exports',(SELECT json_agg(e ORDER BY id) FROM export_operations e),'choices',(SELECT count(*) FROM endpoint_choices),'deploymentAdmission',(SELECT open FROM deployment_admission),'enrollmentAdmission',(SELECT open FROM enrollment_admission))`);}
const instances=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade','8d234722-f0ad-47b2-8ef8-10f85d0c13b9'],nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return instances.map(instance=>({instance,native:JSON.parse(command('docker',['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+instance+suffix)).digest('hex'))}));}
function admin(state){return JSON.parse(command('./scripts/dev.sh',['owner-admin',...(state?['set','--state',state,'--reason','local_qualification']:['status'])]));}
const before=inventory(),nativeBefore=custody(),policy=admin(),auditBefore=sql('SELECT count(*) FROM owner_admission_events');assert.equal(policy.open,true,'Start from open local owner admission; never override an existing pause');assert.equal(sql('SELECT version FROM schema_version'),27);
let restored=false;
try {
 assert.equal(admin('closed').open,false);const first=admin();assert.deepEqual(admin('closed'),first);assert.equal(sql('SELECT count(*) FROM owner_admission_events'),auditBefore+1);
 const alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob');assert.notEqual(alice.session.ownerId,bob.session.ownerId);
 assert.equal((await alice.api('/api/deployments/'+instances[0])).status,200);assert.equal((await bob.api('/api/deployments/'+instances[1])).status,200);assert.equal((await alice.api('/api/deployments/'+instances[1])).status,404);
 assert.equal(admin('open').open,true);restored=true;const reopened=admin();assert.deepEqual(admin('open'),reopened);assert.equal(sql('SELECT count(*) FROM owner_admission_events'),auditBefore+2);
 assert.deepEqual(inventory(),before);assert.deepEqual(custody(),nativeBefore);
 const report={id,completedAt:new Date().toISOString(),schema:27,existingOwnerSigninAndScopePreserved:true,repeatedChangesAddedNoAudit:true,effectiveAuditChanges:2,sourceWorkAndNativeCustodyUnchanged:true,ownerAdmissionRestoredOpen:true,newSubjectDenial:'disposable_database_callback_tests_only',publicSignupQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
} finally {if(!restored){assert.equal(admin('open').open,true,'Local owner admission recovery failed');}}
