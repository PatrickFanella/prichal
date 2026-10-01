// Bounded pressure in the second populated disposable tenant; never a remote runtime.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdirSync,lstatSync,rmdirSync} from 'node:fs';
import {spawnSync,spawn} from 'node:child_process';
import {randomUUID,createHash} from 'node:crypto';
import {performance} from 'node:perf_hooks';
import {localNativeClient} from './local-native-client.mjs';
const paths=process.argv.slice(2);assert.equal(paths.length,2,'Supply Alice and Bob private local account journeys');
const accounts=paths.map(path=>{assert.match(path,/^\.local\/owner-enrollment\/[a-f0-9-]{36}$/);const file=path+'/account.json',info=lstatSync(file);assert.ok(info.isFile()&&!(info.mode&0o077));return JSON.parse(readFileSync(file))});
const ids=accounts.map(value=>new URL(value.endpoint).hostname.replace(/\.localhost$/,''));assert.notEqual(ids[0],ids[1]);
const clients=ids.map((id,index)=>localNativeClient(id,accounts[index].endpoint)),names=ids.map(id=>'pds-ass-tenant-'+id);
function docker(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:1024*1024});assert.equal(result.status,0,'Fixed local resource qualification operation');return result.stdout}
assert.ok(docker(['context','inspect','--format','{{.Endpoints.docker.Host}}']).trim().startsWith('unix:///'));
const image='ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336';
function inspect(index){const row=JSON.parse(docker(['inspect',names[index]]))[0];assert.equal(row.Config.Image,image);assert.equal(row.Config.Labels['tv.subcult.pds-ass.instance'],ids[index]);assert.ok(row.State.Running);assert.equal(row.HostConfig.Memory,768<<20);assert.equal(row.HostConfig.NanoCpus,1e9);assert.equal(row.HostConfig.PidsLimit,128);assert.ok(!row.HostConfig.Privileged&&!row.HostConfig.Binds?.length);assert.deepEqual(row.HostConfig.CapDrop,['ALL']);assert.deepEqual(row.HostConfig.SecurityOpt,['no-new-privileges:true']);assert.ok(row.Config.Env.includes('PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582'));return row}
const source=ids.map((_,index)=>inspect(index));
const root='.local/resource-pressure/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/resource-pressure/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
const protectedSuffixes=['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'];
function digest(index){const hash=createHash('sha256');for(const suffix of protectedSuffixes){const file='.local/node-agent/state/'+ids[index]+suffix;try{const info=lstatSync(file);assert.ok(info.isFile()&&!(info.mode&0o077));hash.update(readFileSync(file))}catch(error){if(error.code!=='ENOENT'||['.json','-tls.pem'].includes(suffix))throw error}}return hash.digest('hex')}
const hashes=ids.map((_,index)=>digest(index)),exportScript=readFileSync('scripts/native-held-export.cjs','utf8');
function inventory(index){return JSON.parse(docker(['exec',names[index],'node','--no-warnings','-e',exportScript]))}
const before=ids.map((_,index)=>inventory(index));assert.ok(before.every(value=>value.repositories===1&&value.signedRecords>0&&value.referencedBlobs.length>0));
const cgroupScript=String.raw`const fs=require('fs');const read=k=>fs.readFileSync('/sys/fs/cgroup/'+k,'utf8').trim();const counters=k=>Object.fromEntries(read(k).split('\n').map(row=>{const [key,value]=row.split(' ');return [key,Number(value)]}));console.log(JSON.stringify({memoryMax:read('memory.max'),swapMax:read('memory.swap.max'),cpuMax:read('cpu.max'),pidsMax:read('pids.max'),memory:counters('memory.events'),cpu:counters('cpu.stat')}))`;
function cgroup(){return JSON.parse(docker(['exec',names[1],'node','-e',cgroupScript]))}
const limits=cgroup();assert.equal(limits.memoryMax,String(768<<20));assert.equal(limits.swapMax,String(768<<20));assert.equal(limits.cpuMax,'100000 100000');assert.equal(limits.pidsMax,'128');
const neighborSession=await clients[0]('com.atproto.server.createSession',{identifier:accounts[0].handle,password:accounts[0].password});assert.equal(neighborSession.did,accounts[0].did);
const samples=[];let child;
async function neighbor(){const start=performance.now(),session=await clients[0]('com.atproto.server.getSession',undefined,'Bearer '+neighborSession.accessJwt);assert.equal(session.did,accounts[0].did);assert.ok(session.emailConfirmed);samples.push(Math.round(performance.now()-start))}
async function pressure(source,expected){
 const process=spawn('docker',['exec',names[1],'node','-e',source],{stdio:'ignore'});child=process;
 let exited=false;const done=new Promise((resolve,reject)=>{process.once('error',()=>{exited=true;reject(new Error('Local pressure process unavailable'))});process.once('exit',(code,signal)=>{exited=true;resolve({code,signal})})});
 const timeout=setTimeout(()=>process.kill('SIGTERM'),20000);
 try{while(!exited){await neighbor();await new Promise(resolve=>setTimeout(resolve,100))}const result=await done;assert.ok(expected.includes(result.code),'Pressure process did not finish with the expected bounded result');return result.code}finally{clearTimeout(timeout);child=undefined}
}
const cpuSource=`const {Worker}=require('node:worker_threads');const code="const end=Date.now()+5000;let x=1;while(Date.now()<end){x=Math.sqrt(x+Math.random())}";new Worker(code,{eval:true});new Worker(code,{eval:true});`;
// Prefer killing this bounded probe rather than the server. Allocation stops after ten seconds.
const memorySource=`const fs=require('fs');fs.writeFileSync('/proc/self/oom_score_adj','1000');const held=[];let count=0;const deadline=Date.now()+10000;function allocate(){if(Date.now()>deadline||count>=110){process.exit(2)}held.push(Buffer.alloc(16*1024*1024,0x5a));count++;setImmediate(allocate)}allocate();`;
function resume(){if(child)child.kill('SIGTERM');const row=JSON.parse(docker(['inspect',names[1]]))[0];if(!row.State.Running)docker(['start',names[1]])}
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{resume();process.exit(130)});
let cpuAfter,memoryAfter,memoryExit;
try{await pressure(cpuSource,[0]);cpuAfter=cgroup();assert.ok(cpuAfter.cpu.nr_throttled>limits.cpu.nr_throttled,'CPU quota did not throttle the two-worker probe');memoryExit=await pressure(memorySource,[137]);memoryAfter=cgroup();assert.ok(memoryAfter.memory.oom_kill>cpuAfter.memory.oom_kill,'No cgroup OOM kill was observed');assert.ok(memoryAfter.memory.max>cpuAfter.memory.max,'No memory-limit pressure was observed')}finally{resume()}
for(let attempt=0;;attempt++){try{const session=await clients[1]('com.atproto.server.createSession',{identifier:accounts[1].handle,password:accounts[1].password});assert.equal(session.did,accounts[1].did);break}catch(error){if(attempt>=59)throw error;await new Promise(resolve=>setTimeout(resolve,200))}}
assert.deepEqual(ids.map((_,index)=>inventory(index)),before,'Signed repositories or referenced media changed');ids.forEach((_,index)=>{assert.equal(inspect(index).Id,source[index].Id);assert.equal(digest(index),hashes[index])});
const report={completedAt:new Date().toISOString(),deployments:ids,cpuQuota:'1 CPU',memoryLimitBytes:768<<20,swapLimitBytes:768<<20,pidsLimit:128,cpuThrottleDelta:cpuAfter.cpu.nr_throttled-limits.cpu.nr_throttled,oomKillDelta:memoryAfter.memory.oom_kill-cpuAfter.memory.oom_kill,memoryLimitEventDelta:memoryAfter.memory.max-cpuAfter.memory.max,memoryProbeExit:memoryExit,neighborAuthenticatedSamples:samples.length,neighborWorstResponseMilliseconds:Math.max(...samples),bothSignedRepositoriesAndMediaRetained:true,containersAndProtectedStateRetained:true,productionIsolationQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{flag:'wx',mode:0o600});console.log(JSON.stringify({...report,report:root+'/report.json'}));
