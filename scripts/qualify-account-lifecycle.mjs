// Disposable local native moderation and retained backup/export eligibility.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync,lstatSync,rmdirSync,copyFileSync,chmodSync} from 'node:fs';
import {randomUUID,randomBytes,createHash} from 'node:crypto';
import {resolve,join} from 'node:path';
import {localOwnerClient} from './local-owner-client.mjs';
import {localNativeClient} from './local-native-client.mjs';
import {sealRecovery,openRecovery} from './recovery-crypto.mjs';
const roots=process.argv.slice(2);assert.equal(roots.length,2);
const accounts=roots.map(path=>{assert.match(path,/^\.local\/owner-enrollment\/[a-f0-9-]{36}$/);const info=lstatSync(path+'/account.json');assert.ok(info.isFile()&&!(info.mode&0o077));return JSON.parse(readFileSync(path+'/account.json'))});
const ids=accounts.map(a=>new URL(a.endpoint).hostname.replace(/\.localhost$/,''));ids.forEach(id=>assert.match(id,/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/));assert.notEqual(ids[0],ids[1]);
function command(cmd,args,input){const result=spawnSync(cmd,args,{input,encoding:'utf8',timeout:40000,maxBuffer:1024*1024});assert.equal(result.status,0,'Fixed local account qualification command');return result.stdout;}
const owner=await localOwnerClient('bob'),foreign=await localOwnerClient('alice');assert.equal((await owner.api('/api/deployments/'+ids[1])).status,200);assert.equal((await foreign.api('/api/deployments/'+ids[1])).status,404);
const escalation=await owner.call('/api/deployments/'+ids[1]+'/account',{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json','X-CSRF-Token':owner.session.csrfToken},body:JSON.stringify({role:'operator',state:'suspended'})});assert.equal(escalation.status,404);
const names=['pds-ass-tenant-','pds-ass-edge-','pds-ass-enrollment-'].map(prefix=>prefix+ids[1]);
function inspect(){return names.map(name=>{const row=JSON.parse(command('docker',['inspect',name]))[0];assert.equal(row.Config.Labels['tv.subcult.pds-ass.instance'],ids[1]);return row})}
const before=inspect();assert.ok(before.every(row=>row.State.Running));assert.deepEqual(before[0].Mounts.map(m=>m.Name).sort(),[names[0]+'-data',names[0]+'-blobs'].sort());
const image=before[0].Config.Image;assert.match(image,/^ghcr\.io\/bluesky-social\/pds:0\.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336$/);
const digest=()=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+ids[1]+'.json')).update(readFileSync('.local/node-agent/state/'+ids[1]+'-tls.pem')).update(readFileSync('.local/node-agent/state/'+ids[1]+'-enrollment/lease.json')).digest('hex');const custody=digest();
const exportScript=readFileSync('scripts/native-held-export.cjs','utf8');const inventory=()=>JSON.parse(command('docker',['exec',names[0],'node','-e',exportScript]));const original=inventory();assert.ok(original.signedRecords>0&&original.referencedBlobs.length>0);
const native=localNativeClient(ids[1],accounts[1].endpoint),session=await native('com.atproto.server.createSession',{identifier:accounts[1].handle,password:accounts[1].password});
const root=resolve('.local/account-lifecycle/'+randomUUID());mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/account-lifecycle/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
const keys={suspend:randomUUID(),resume:randomUUID(),stop:randomUUID(),start:randomUUID()};writeFileSync(join(root,'requests.json'),JSON.stringify({deployment:ids[1],...keys}),{mode:0o600,flag:'wx'});
function submit(kind,state,key){return JSON.parse(command('./scripts/dev.sh',['service-admin','request','--kind',kind,'--deployment',ids[1],'--key',key,'--state',state,'--reason',kind==='account'?'safety_review':'maintenance']))}
async function wait(op){for(let i=0;i<40;i++){const result=JSON.parse(command('./scripts/dev.sh',['service-admin','status','--operation',op.id]));assert.notEqual(result.state,'failed','Account operation failed');if(result.state==='applied')return result;await new Promise(resolve=>setTimeout(resolve,250))}throw new Error('Account operation not confirmed')}
function agent(method,body){const output=command('curl',['--silent','--show-error','--max-time','30','--cacert','.local/node-agent/certs/ca.crt','--cert','.local/node-agent/certs/worker.crt','--key','.local/node-agent/certs/worker.key','-X',method,...(body?['-H','Content-Type: application/json','--data',JSON.stringify(body)]:[]),'-w','\n%{http_code}','https://127.0.0.1:9091/v1/instances/'+ids[1]+'/account']);const lines=output.trim().split('\n');return {status:Number(lines.pop()),body:lines.join('\n')}}
let accountAttempted=false,serviceAttempted=false,serviceResumed=false,accountResumed=false;
async function restore(){if(serviceAttempted&&!serviceResumed){await wait(submit('service','suspended',keys.stop));await wait(submit('service','running',keys.start));serviceResumed=true}if(accountAttempted&&!accountResumed){await wait(submit('account','suspended',keys.suspend));await wait(submit('account','running',keys.resume));accountResumed=true}}
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{restore().then(()=>process.exit(130)).catch(()=>{console.error('Durable local moderation cleanup requires operator inspection');process.exit(1)})});
let suspension,coldBytes;
try {
 accountAttempted=true;suspension=await wait(submit('account','suspended',keys.suspend));assert.equal(submit('account','suspended',keys.suspend).id,suspension.id);
 const status=agent('GET');assert.equal(status.status,200);const accountStatus=JSON.parse(status.body);assert.equal(accountStatus.did,accounts[1].did);assert.equal(accountStatus.state,'suspended');assert.equal(accountStatus.generation,suspension.generation);
 const deniedLogin=await native('com.atproto.server.createSession',{identifier:accounts[1].handle,password:accounts[1].password},undefined,401);assert.equal(deniedLogin.error,'AccountTakedown');
 const deniedWrite=await native('com.atproto.repo.createRecord',{repo:accounts[1].did,collection:'app.bsky.feed.post',record:{$type:'app.bsky.feed.post',text:'Denied held write',createdAt:new Date().toISOString()}},'Bearer '+session.accessJwt,401);assert.equal(deniedWrite.error,'AccountTakedown');
 const deniedServe=await native('com.atproto.sync.getRepo?did='+accounts[1].did,undefined,undefined,400);assert.equal(deniedServe.error,'RepoTakendown');
 assert.deepEqual(inventory(),original);assert.ok(inspect().every(row=>row.State.Running));
 const alice=await localNativeClient(ids[0],accounts[0].endpoint)('com.atproto.server.createSession',{identifier:accounts[0].handle,password:accounts[0].password});assert.equal(alice.did,accounts[0].did);
 command('./scripts/monitor-local.sh',['once']);const view=await owner.api('/api/deployments/'+ids[1]);assert.equal(view.value.service.account,'suspended');assert.equal(view.value.service.lifecycle,'running');assert.equal(view.value.service.state,'healthy');
 serviceAttempted=true;await wait(submit('service','suspended',keys.stop));assert.ok(inspect().every(row=>!row.State.Running));
 for(const suffix of ['data','blobs'])assert.equal(command('docker',['ps','--filter','volume='+names[0]+'-'+suffix,'--format','{{.ID}}']).trim(),'');
 const host=join(root,'host');mkdirSync(host,{mode:0o700});for(const suffix of ['.json','-tls.pem','-service.json','-account.json'])copyFileSync('.local/node-agent/state/'+ids[1]+suffix,join(host,'instance'+suffix));
 copyFileSync('.local/node-agent/state/'+ids[1]+'-edge/Caddyfile',join(host,'Caddyfile'));copyFileSync('.local/node-agent/state/'+ids[1]+'-enrollment/lease.json',join(host,'lease.json'));
 const archive=join(root,'held.tar');command('docker',['run','--rm','--network','none','--memory','192m','--cpus','0.5','--mount',`type=volume,src=${names[0]}-data,dst=/snapshot/data,readonly`,'--mount',`type=volume,src=${names[0]}-blobs,dst=/snapshot/blobs,readonly`,'--mount',`type=bind,src=${host},dst=/snapshot/host,readonly`,'--mount',`type=bind,src=${root},dst=/out`,'--entrypoint','tar',image,'-C','/snapshot','-cf','/out/held.tar','data','blobs','host']);
 chmodSync(archive,0o600);const key=join(root,'key');writeFileSync(key,randomBytes(32),{mode:0o600,flag:'wx'});await sealRecovery(archive,join(root,'held.pdsbackup'),key,{version:1,id:randomUUID(),scope:'pds-local',createdAt:new Date().toISOString()});await openRecovery(join(root,'held.pdsbackup'),join(root,'verified.tar'),key);
 const hash=path=>createHash('sha256').update(readFileSync(path)).digest('hex');assert.equal(hash(archive),hash(join(root,'verified.tar')));coldBytes=lstatSync(archive).size;
 await wait(submit('service','running',keys.start));serviceResumed=true;const stillHeld=agent('GET');assert.equal(stillHeld.status,200);assert.equal(JSON.parse(stillHeld.body).state,'suspended');assert.deepEqual(inventory(),original);
} finally {await restore()}
const stale=agent('POST',{ticket:suspension.id,generation:suspension.generation,state:'suspended'});assert.equal(stale.status,503);
const login=await native('com.atproto.server.createSession',{identifier:accounts[1].handle,password:accounts[1].password});assert.ok(login.did===accounts[1].did&&login.emailConfirmed);assert.deepEqual(inventory(),original);assert.equal(digest(),custody);assert.deepEqual(inspect().map(row=>row.Id),before.map(row=>row.Id));
command('./scripts/monitor-local.sh',['once']);const final=await owner.api('/api/deployments/'+ids[1]);assert.equal(final.value.service.account,'running');assert.equal(final.value.service.lifecycle,'running');assert.equal(final.value.service.state,'healthy');
const report={completedAt:new Date().toISOString(),deployment:ids[1],durableNativeTakedown:true,normalLoginDenied:true,retainedSessionWritesDenied:true,publicRepoServingDenied:true,operatorSignedExportEligible:true,encryptedColdCaptureWhileHeld:true,coldArchiveBytes:coldBytes,serviceResumeRetainedAccountTakedown:true,neighborAvailable:true,ownerEscalationDenied:true,foreignOwnerDenied:true,staleAccountGenerationDenied:true,confirmedAccountDataMediaAndCustodyRetained:true,customerExportQualified:false,heldRestoreQualified:false,productionLifecycleQualified:false};writeFileSync(join(root,'report.json'),JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify({...report,report:join(root,'report.json')}));
