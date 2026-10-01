// Audited account hold, fresh-volume recovery, then separate source account resume.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync,lstatSync,rmdirSync} from 'node:fs';
import {randomUUID} from 'node:crypto';
import {localNativeClient} from './local-native-client.mjs';
const journey=process.argv[2];assert.equal(process.argv.length,3);assert.match(journey??'',/^\.local\/owner-enrollment\/[a-f0-9-]{36}$/);
const file=journey+'/account.json';assert.ok(lstatSync(file).isFile()&&!(lstatSync(file).mode&0o077));const account=JSON.parse(readFileSync(file)),id=new URL(account.endpoint).hostname.replace(/\.localhost$/,''),native=localNativeClient(id,account.endpoint);
const root='.local/held-recovery/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/held-recovery/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
const keys={suspend:randomUUID(),resume:randomUUID()};writeFileSync(root+'/requests.json',JSON.stringify({deployment:id,...keys}),{mode:0o600,flag:'wx'});
function command(program,args,timeout=30000){const result=spawnSync(program,args,{encoding:'utf8',timeout,maxBuffer:1024*1024});if(result.status!==0){writeFileSync(root+'/subprocess-failure.log',result.stderr,{mode:0o600});}assert.equal(result.status,0,'Fixed local held recovery operation');return result.stdout}
function submit(state,key){return JSON.parse(command('./scripts/dev.sh',['service-admin','request','--kind','account','--deployment',id,'--key',key,'--state',state,'--reason','safety_review']))}
async function wait(operation){for(let attempt=0;attempt<60;attempt++){const current=JSON.parse(command('./scripts/dev.sh',['service-admin','status','--operation',operation.id]));assert.notEqual(current.state,'failed','Audited account action failed');if(current.state==='applied')return current;await new Promise(resolve=>setTimeout(resolve,250))}throw new Error('Audited account action not confirmed')}
const state='.local/node-agent/state/'+id;const leaseFile=state+'-account.json';assert.ok(lstatSync(leaseFile).isFile()&&!(lstatSync(leaseFile).mode&0o077));const lease=JSON.parse(readFileSync(leaseFile));assert.equal(lease.instanceId,id);assert.equal(lease.did,account.did);assert.equal(lease.state,'running');assert.equal(lease.phase,'applied');
const exportScript=readFileSync('scripts/native-held-export.cjs','utf8');function inventory(){return JSON.parse(command('docker',['exec','pds-ass-tenant-'+id,'node','--no-warnings','-e',exportScript]))}
const before=inventory();let attempted=false,resumed=false,suspension,restoration;
async function resume(){if(attempted&&!resumed){await wait(submit('suspended',keys.suspend));for(let attempt=0;;attempt++){const row=JSON.parse(command('docker',['inspect','pds-ass-tenant-'+id]))[0];if(row.State.Running&&row.State.Health?.Status==='healthy')break;if(attempt>=149)throw new Error('Source health did not settle before account resume');await new Promise(resolve=>setTimeout(resolve,200))}await wait(submit('running',keys.resume));resumed=true}}
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>resume().then(()=>process.exit(130)).catch(()=>{console.error('Inspect retained local account recovery intent');process.exit(1)}));
try{
 attempted=true;suspension=await wait(submit('suspended',keys.suspend));assert.deepEqual(inventory(),before);
 const output=command('node',['scripts/qualify-managed-recovery.mjs',journey,'bob','--held-account'],120000);const line=output.trim().split('\n').at(-1);restoration=JSON.parse(line);assert.ok(restoration.heldAccountRestore&&restoration.restoredAccountTakedownRetained&&restoration.freshStorageRestore&&restoration.sourceProtectedMetadataRetained);assert.equal(restoration.restoredNativeWrite,false);assert.deepEqual(inventory(),before);
}finally{await resume()}
const login=await native('com.atproto.server.createSession',{identifier:account.handle,password:account.password});assert.ok(login.did===account.did&&login.emailConfirmed);assert.deepEqual(inventory(),before);
const report={completedAt:new Date().toISOString(),deployment:id,auditedAccountHold:true,generation:suspension.generation,heldFreshStorageRestore:true,restoredPublicLoginAndRepoDenied:true,restoredOperatorSignedExportAndMedia:true,recoveredPLCSigner:restoration.recoveredPLCSigner,restoredAndSourceWritersFenced:true,sourceResumedThroughSeparateAuditedAccountAction:true,sourceSignedDataAndMediaRetained:true,managedRecoveryReport:restoration.report,customerExportQualified:false,productionRecoveryQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify({...report,report:root+'/report.json'}));
