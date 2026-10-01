// Local operator lifecycle qualification. Original tenant storage is always retained.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync,lstatSync,rmdirSync} from 'node:fs';
import {randomUUID,createHash} from 'node:crypto';
import {localOwnerClient} from './local-owner-client.mjs';
import {localNativeClient} from './local-native-client.mjs';
const roots=process.argv.slice(2);assert.equal(roots.length,2);
const accounts=roots.map(path=>{assert.match(path,/^\.local\/owner-enrollment\/[a-f0-9-]{36}$/);const info=lstatSync(path+'/account.json');assert.ok(info.isFile()&&!(info.mode&0o077));return JSON.parse(readFileSync(path+'/account.json'))});
const ids=accounts.map(a=>new URL(a.endpoint).hostname.replace(/\.localhost$/,''));ids.forEach(id=>assert.match(id,/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/));assert.notEqual(ids[0],ids[1]);
function command(cmd,args,input){const result=spawnSync(cmd,args,{input,encoding:'utf8',timeout:40000,maxBuffer:1024*1024});assert.equal(result.status,0,'Fixed local service qualification command');return result.stdout;}
const owner=await localOwnerClient('bob'),foreign=await localOwnerClient('alice');
assert.equal((await owner.api('/api/deployments/'+ids[1])).status,200);assert.equal((await foreign.api('/api/deployments/'+ids[1])).status,404);
const escalation=await owner.call('/api/deployments/'+ids[1]+'/service',{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json','X-CSRF-Token':owner.session.csrfToken},body:JSON.stringify({state:'suspended',role:'operator'})});assert.equal(escalation.status,404);
const names=['pds-ass-tenant-','pds-ass-edge-','pds-ass-enrollment-'].map(prefix=>prefix+ids[1]);
function inspect(){return names.map(name=>{const row=JSON.parse(command('docker',['inspect',name]))[0];assert.equal(row.Config.Labels['tv.subcult.pds-ass.instance'],ids[1]);return row})}
const before=inspect();assert.ok(before.every(row=>row.State.Running));
const digest=()=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+ids[1]+'.json')).update(readFileSync('.local/node-agent/state/'+ids[1]+'-tls.pem')).update(readFileSync('.local/node-agent/state/'+ids[1]+'-enrollment/lease.json')).digest('hex');const custody=digest();
const script=readFileSync('deploy/local/fixtures/blob-inventory.cjs','utf8').replace("'http://protocol-fixtures:2582'","'http://pds-ass-protocol-fixtures:2582'");
const inventory=()=>JSON.parse(command('docker',['exec',names[0],'node','-e',script]));const original=inventory();assert.equal(original.repositories,1);assert.ok(original.signedRecords>0&&original.referencedBlobs.length>0);
const root='.local/service-lifecycle/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/service-lifecycle/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
const suspendKey=randomUUID(),resumeKey=randomUUID();writeFileSync(root+'/requests.json',JSON.stringify({deployment:ids[1],suspendKey,resumeKey}),{mode:0o600,flag:'wx'});
function submit(state,key){return JSON.parse(command('./scripts/dev.sh',['service-admin','request','--deployment',ids[1],'--key',key,'--state',state,'--reason','maintenance']))}
async function wait(op){for(let i=0;i<40;i++){const result=JSON.parse(command('./scripts/dev.sh',['service-admin','status','--operation',op.id]));assert.notEqual(result.state,'failed','Service operation failed');if(result.state==='applied')return result;await new Promise(resolve=>setTimeout(resolve,250))}throw new Error('Service operation not confirmed')}
function agent(method,body){const output=command('curl',['--silent','--show-error','--max-time','30','--cacert','.local/node-agent/certs/ca.crt','--cert','.local/node-agent/certs/worker.crt','--key','.local/node-agent/certs/worker.key','-X',method,'-H','Content-Type: application/json','--data',JSON.stringify(body),'-w','\n%{http_code}','https://127.0.0.1:9091/v1/instances/'+ids[1]+(method==='POST'?'/service':'')]);const lines=output.trim().split('\n');return {status:Number(lines.pop()),body:lines.join('\n')}}
let stopAttempted=false,resumed=false;
async function restore(){if(stopAttempted&&!resumed){await wait(submit('suspended',suspendKey));await wait(submit('running',resumeKey));resumed=true}}
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{restore().then(()=>process.exit(130)).catch(()=>{console.error('Durable local resume requires operator inspection');process.exit(1)})});
let suspension,resume;
try {
 stopAttempted=true;suspension=await wait(submit('suspended',suspendKey));assert.equal(submit('suspended',suspendKey).id,suspension.id);assert.ok(inspect().every(row=>!row.State.Running));
 const reconciled=agent('PUT',{});assert.equal(reconciled.status,200);assert.equal(JSON.parse(reconciled.body).state,'suspended');assert.ok(inspect().every(row=>!row.State.Running));
 await assert.rejects(localNativeClient(ids[1],accounts[1].endpoint)('com.atproto.server.describeServer'));
 const alice=await localNativeClient(ids[0],accounts[0].endpoint)('com.atproto.server.createSession',{identifier:accounts[0].handle,password:accounts[0].password});assert.equal(alice.did,accounts[0].did);
 command('./scripts/monitor-local.sh',['once']);const view=await owner.api('/api/deployments/'+ids[1]);assert.equal(view.value.service.lifecycle,'suspended');assert.equal(view.value.service.state,'suspended');
} finally {await restore()}
resume=submit('running',resumeKey);assert.equal(resume.state,'applied');assert.equal(resume.generation,suspension.generation+1);
const stale=agent('POST',{ticket:suspension.id,generation:suspension.generation,state:'suspended'});assert.equal(stale.status,503);assert.ok(inspect().every(row=>row.State.Running));
const login=await localNativeClient(ids[1],accounts[1].endpoint)('com.atproto.server.createSession',{identifier:accounts[1].handle,password:accounts[1].password});assert.ok(login.did===accounts[1].did&&login.emailConfirmed);
assert.deepEqual(inventory(),original);assert.equal(digest(),custody);assert.deepEqual(inspect().map(row=>row.Id),before.map(row=>row.Id));
command('./scripts/monitor-local.sh',['once']);const after=await owner.api('/api/deployments/'+ids[1]);assert.equal(after.value.service.lifecycle,'running');assert.equal(after.value.service.state,'healthy');
const report={completedAt:new Date().toISOString(),deployment:ids[1],ownerEscalationDenied:true,foreignOwnerDenied:true,durableSuspension:true,reconciliationRespectedHold:true,allTenantProcessesStopped:true,neighborAvailable:true,intentionalSuspensionShown:true,durableResume:true,staleGenerationRejected:true,signedDataAndMediaRetained:true,confirmedAccountRetained:true,containerAndCustodyRetained:true,nativeAccountTakedownQualified:false,productionLifecycleQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify({...report,report:root+'/report.json'}));
