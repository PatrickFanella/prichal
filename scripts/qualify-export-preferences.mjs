// Normal and held exports with one reversible private preference on disposable actors.
import assert from 'node:assert/strict';import {spawnSync} from 'node:child_process';import {readFileSync,writeFileSync,mkdirSync,rmdirSync} from 'node:fs';import {randomUUID,createHash} from 'node:crypto';import {localOwnerClient} from './local-owner-client.mjs';import {localNativeClient} from './local-native-client.mjs';
assert.equal(process.argv.length,2);const root='.local/export-preferences/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/customer-exports/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
let interrupted=false,cleaning=false;for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{interrupted=true;});
function command(program,args,input){if(interrupted&&!cleaning)throw new Error('Local preference qualification interrupted');const result=spawnSync(program,args,{input,encoding:'utf8',timeout:30000,maxBuffer:1024*1024});if(result.status!==0)writeFileSync(root+'/subprocess-failure.log',result.stderr,{mode:0o600});assert.equal(result.status,0,'Fixed preference qualification subprocess');return result.stdout;}
const inventoryScript=readFileSync('scripts/native-held-export.cjs','utf8'),fixture=readFileSync('scripts/native-preference-fixture.cjs','utf8'),verifier=readFileSync('scripts/verify-portable-export.cjs','utf8');
const cases=[{owner:'alice',journey:'.local/owner-enrollment/3ef3731b-9684-4dd8-926e-1d14f93ea1e0',held:false},{owner:'bob',journey:'.local/owner-enrollment/f2b4432f-6047-47d2-9646-b8f892193757',held:true}];const completed=[];
function submit(id,state,key){return JSON.parse(command('./scripts/dev.sh',['service-admin','request','--kind','account','--deployment',id,'--key',key,'--state',state,'--reason','safety_review']));}
async function wait(operation){for(let i=0;i<80;i++){const current=JSON.parse(command('./scripts/dev.sh',['service-admin','status','--operation',operation.id]));assert.notEqual(current.state,'failed');if(current.state==='applied')return current;await new Promise(resolve=>setTimeout(resolve,250));}throw new Error('Account action not confirmed');}
for(const item of cases){
 const account=JSON.parse(readFileSync(item.journey+'/account.json')),id=new URL(account.endpoint).hostname.replace(/\.localhost$/,''),ticket=randomUUID(),exportKey=randomUUID(),holdKey=randomUUID(),resumeKey=randomUUID();writeFileSync(root+'/'+item.owner+'-requests.json',JSON.stringify({deployment:id,ticket,exportKey,holdKey,resumeKey}),{mode:0o600,flag:'wx'});
 const native=localNativeClient(id,account.endpoint),owner=await localOwnerClient(item.owner),foreign=await localOwnerClient(item.owner==='alice'?'bob':'alice');
 const inventory=()=>JSON.parse(command('docker',['exec','pds-ass-tenant-'+id,'node','--no-warnings','-e',inventoryScript]));const preferences=action=>JSON.parse(command('docker',['exec','pds-ass-tenant-'+id,'node','--no-warnings','-e',fixture,action,account.did,ticket]));
 const before=inventory(),prefsBefore=preferences('inventory');assert.equal(prefsBefore.count,0,'This reversible drill requires an empty private fixture');
 const custodyPaths=[`.local/node-agent/state/${id}.json`,`.local/node-agent/state/${id}-tls.pem`,`.local/node-agent/state/${id}-enrollment/lease.json`];const custody=()=>custodyPaths.map(path=>createHash('sha256').update(readFileSync(path)).digest('hex'));const protectedBefore=custody();
 let seeded=false,attempted=false,resumed=false,result,hold;
 try{
  seeded=true;const seededPrefs=preferences('seed');assert.equal(seededPrefs.count,1);
  if(item.held){attempted=true;hold=await wait(submit(id,'suspended',holdKey));await native('com.atproto.server.createSession',{identifier:account.handle,password:account.password},undefined,401);}
  const request=await owner.api(`/api/deployments/${id}/exports`,{idempotencyKey:exportKey});assert.equal(request.status,200);
  let operation;for(let i=0;i<80;i++){const response=await owner.api('/api/exports/'+request.value.id);assert.equal(response.status,200);operation=response.value;assert.notEqual(operation.state,'failed');if(operation.state==='ready')break;await new Promise(resolve=>setTimeout(resolve,250));}assert.equal(operation.state,'ready');
  assert.equal((await foreign.call('/api/exports/'+operation.id+'/download')).status,404);
  const response=await owner.call('/api/exports/'+operation.id+'/download');assert.equal(response.status,200);const archive=Buffer.from(await response.arrayBuffer());assert.equal(createHash('sha256').update(archive).digest('hex'),operation.sha256);writeFileSync(root+'/'+item.owner+'.tar',archive,{mode:0o600,flag:'wx'});
  const verified=JSON.parse(command('docker',['exec','-i','pds-ass-tenant-'+id,'node','--no-warnings','-e',verifier],archive));assert.equal(verified.formatVersion,2);assert.equal(verified.privatePreferences,1);assert.equal(verified.preferenceSha256,seededPrefs.sha256);assert.equal(verified.signedRecords,before.signedRecords);assert.deepEqual(verified.referencedBlobs,before.referencedBlobs);
  assert.ok(!readFileSync('.local/exports/'+operation.id+'.sealed').includes(Buffer.from(ticket)));assert.deepEqual(preferences('inventory'),seededPrefs);assert.deepEqual(inventory(),before);result={owner:item.owner,exportId:operation.id,bytes:operation.bytes,sha256:operation.sha256,signedRecords:verified.signedRecords,blobs:verified.referencedBlobs.length,preferences:1,held:item.held,holdGeneration:hold?.generation??null};
 }finally{
  cleaning=true;
  if(attempted&&!resumed){await wait(submit(id,'suspended',holdKey));for(let i=0;;i++){const state=JSON.parse(command('docker',['inspect','--format','{{json .State}}','pds-ass-tenant-'+id]));if(state.Running&&state.Health?.Status==='healthy')break;if(i>=149)throw new Error('Account service health unsettled');await new Promise(resolve=>setTimeout(resolve,200));}await wait(submit(id,'running',resumeKey));resumed=true;}
  if(seeded)preferences('restore');
  cleaning=false;
 }
 assert.deepEqual(preferences('inventory'),prefsBefore);assert.deepEqual(inventory(),before);assert.deepEqual(custody(),protectedBefore);completed.push(result);
}
const report={completedAt:new Date().toISOString(),exports:completed,privatePreferencesIndependentlyVerified:true,foreignOwnersDenied:true,sourceRecordsMediaPreferencesAndCustodyRetained:true,fixturePreferenceSeedAndRestore:true,publicAppPreferenceAPIQualified:false,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
