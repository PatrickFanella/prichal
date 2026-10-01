// Retained local tenant data, network and crash qualification. Never a production host.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdirSync,lstatSync,rmdirSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {randomUUID,createHash} from 'node:crypto';
import {localOwnerClient} from './local-owner-client.mjs';
import {localNativeClient} from './local-native-client.mjs';
const roots=process.argv.slice(2);assert.equal(roots.length,2);
const fixture=roots.map(path=>{assert.match(path,/^\.local\/owner-enrollment\/[a-f0-9-]{36}$/);const info=lstatSync(path+'/account.json');assert.ok(info.isFile()&&(info.mode&0o077)===0);return JSON.parse(readFileSync(path+'/account.json'))});
const ids=fixture.map(value=>new URL(value.endpoint).hostname.replace(/\.localhost$/,''));ids.forEach(id=>assert.match(id,/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/));assert.notEqual(ids[0],ids[1]);
const names=ids.map(id=>'pds-ass-tenant-'+id);
function docker(args,input){const r=spawnSync('docker',args,{input,encoding:'utf8',timeout:30000,maxBuffer:1024*1024});assert.equal(r.status,0,'Fixed local Docker qualification operation');return r.stdout;}
function inspect(index){const row=JSON.parse(docker(['inspect',names[index]]))[0];assert.equal(row.Config.Labels['tv.subcult.pds-ass.instance'],ids[index]);assert.ok(row.State.Running);assert.equal(row.HostConfig.Memory,768*1024*1024);assert.equal(row.HostConfig.NanoCpus,1e9);assert.equal(row.HostConfig.PidsLimit,128);assert.ok(!row.HostConfig.Privileged && !row.HostConfig.Binds?.length);assert.deepEqual(row.HostConfig.CapDrop,['ALL']);assert.deepEqual(row.HostConfig.SecurityOpt,['no-new-privileges:true']);assert.equal(Object.keys(row.NetworkSettings.Networks).length,1);assert.ok(Object.values(row.NetworkSettings.Ports).every(value=>!value?.length));assert.deepEqual(row.Mounts.map(value=>value.Name).sort(),[names[index]+'-blobs',names[index]+'-data'].sort());return row;}
const containers=ids.map((_,index)=>inspect(index));
const networks=containers.map((row,index)=>{const name=names[index]+'-network';const network=JSON.parse(docker(['network','inspect',name]))[0];assert.ok(network.Internal);return row.NetworkSettings.Networks[name]});
assert.notEqual(networks[0].IPAddress,networks[1].IPAddress);
const digest=id=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+'.json')).update(readFileSync('.local/node-agent/state/'+id+'-tls.pem')).digest('hex');const before=ids.map(digest);
const root='.local/tenant-isolation/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const lock='.local/tenant-isolation/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
const source=readFileSync('scripts/native-tenant-data.cjs','utf8');
function data(index,mode,record){return JSON.parse(docker(['exec','-i',names[index],'node','-e',source],JSON.stringify({account:fixture[index],mode,record,marker:randomUUID()})));}
const populated=ids.map((_,index)=>data(index,'populate'));writeFileSync(root+'/records.json',JSON.stringify(populated),{flag:'wx',mode:0o600});
const alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob');assert.equal((await alice.api('/api/deployments/'+ids[1])).status,404);assert.equal((await bob.api('/api/deployments/'+ids[0])).status,404);
const clients=ids.map((id,index)=>localNativeClient(id,fixture[index].endpoint));const sessions=await Promise.all(clients.map((client,index)=>client('com.atproto.server.createSession',{identifier:fixture[index].handle,password:fixture[index].password})));
for(let index=0;index<2;index++){const rejected=await clients[1-index]('com.atproto.repo.createRecord',{repo:fixture[1-index].did,collection:'app.bsky.feed.post',record:{$type:'app.bsky.feed.post',text:'Unauthorized probe',createdAt:new Date().toISOString()}},'Bearer '+sessions[index].accessJwt,400);assert.equal(rejected.error,'InvalidToken','Foreign native session must fail token validation');}
const probe=`const net=require('node:net');async function blocked(host,port){return await new Promise(resolve=>{let connected=false;const socket=net.connect({host,port});const done=()=>{socket.destroy();resolve(!connected)};socket.setTimeout(1500,done);socket.on('connect',()=>{connected=true;done()});socket.on('error',done)})}Promise.all([blocked(${JSON.stringify(networks[1].IPAddress)},2583),blocked('169.254.169.254',80)]).then(results=>{if(!results.every(Boolean))process.exitCode=1;else console.log(JSON.stringify({neighborBlocked:true,metadataBlocked:true}))});`;
const blocked=JSON.parse(docker(['exec',names[0],'node','-e',probe]));assert.ok(blocked.neighborBlocked&&blocked.metadataBlocked);
let crashAttempted=false;
const restore=()=>{if(crashAttempted)docker(['start',names[1]]);};
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{restore();process.exit(130)});
try {
 crashAttempted=true;docker(['kill','--signal','KILL',names[1]]);
 for(let attempt=0;attempt<3;attempt++){const login=await clients[0]('com.atproto.server.createSession',{identifier:fixture[0].handle,password:fixture[0].password});assert.equal(login.did,fixture[0].did);}
 data(0,'read',populated[0].record);
}finally{restore();}
for(let attempt=0;;attempt++){try{await clients[1]('com.atproto.server.createSession',{identifier:fixture[1].handle,password:fixture[1].password});break;}catch(error){if(attempt>=49)throw error;await new Promise(resolve=>setTimeout(resolve,200));}}
data(1,'read',populated[1].record);
ids.forEach((id,index)=>{assert.ok(digest(id)===before[index],'Identity material changed');assert.equal(inspect(index).Id,containers[index].Id)});
const report={completedAt:new Date().toISOString(),deployments:ids,populatedTenants:2,signedReposAndMedia:true,subscriptionCommits:true,ownerScope:true,foreignSessionRejected:true,separateVolumes:true,separatePrivateNetworks:true,noNativeHostPorts:true,configuredResourceBounds:true,neighborDirectAccessBlocked:true,metadataAccessBlocked:true,neighborSurvivedCrash:true,crashedTenantDataRetained:true,identityAndContainersRetained:true,productionIsolationQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{flag:'wx',mode:0o600});console.log(JSON.stringify({...report,report:root+'/report.json'}));
