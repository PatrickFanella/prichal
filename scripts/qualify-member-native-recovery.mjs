// Opt-in companion to the fresh native member-window qualification. Credentials
// arrive only on stdin; this driver never accepts an arbitrary source or runtime.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdirSync,lstatSync,readFileSync,writeFileSync,copyFileSync,openSync,closeSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {randomUUID,randomBytes,createHash} from 'node:crypto';
import {performance} from 'node:perf_hooks';
import {sealRecovery,openRecovery} from './recovery-crypto.mjs';
import {memberRecoveryFiles,restoreMemberRecoveryFiles} from './member-recovery-state.mjs';
import {recoverySubnets,composeLiterals} from './recovery-networks.mjs';

assert.equal(process.env.PDS_QUALIFY_NATIVE_MEMBER_RECOVERY,'true','Explicit disposable native recovery opt-in required');
const [rootText,tenant,...extra]=process.argv.slice(2),root=resolve(rootText??'');
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
assert.equal(extra.length,0);assert.match(tenant??'',uuid);
// The fixture directory has a qualification UUID distinct from its tenant UUID.
assert.ok(root.startsWith(resolve('.local/member-removal-native')+'/')&&uuid.test(root.split('/').at(-1)));
function privateDirectory(path){const info=lstatSync(path);assert.ok(info.isDirectory()&&!(info.mode&0o077));}
privateDirectory(root);
const input=readFileSync(0);assert.ok(input.length<4096);const logins=JSON.parse(input);
assert.ok(Array.isArray(logins)&&logins.length===2&&logins.every(v=>typeof v.handle==='string'&&/^[a-z0-9-]+\.test$/.test(v.handle)&&/^[a-f0-9]{48}$/.test(v.password)&&/^did:plc:[a-z2-7]{24}$/.test(v.did)));
assert.notEqual(logins[0].did,logins[1].did);
const image='ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336';
const id=randomUUID(),prefix='pds-ass-member-recovery-'+id,work=join(root,'cold-recovery');mkdirSync(work,{mode:0o700});
const resources={id,containers:[]},report={result:'failed',tenant,id,stage:'preflight',productionRecoveryQualified:false};
function receipt(){writeFileSync(join(work,'report.json'),JSON.stringify(report,null,2),{mode:0o600});}
function register(name){resources.containers.push(name);writeFileSync(join(work,'resources.json'),JSON.stringify(resources),{mode:0o600});}
function command(args,options={}){const result=spawnSync('docker',args,{encoding:'utf8',timeout:60000,maxBuffer:8<<20,...options});if(result.status!==0){report.failedOperation=args[0];writeFileSync(join(work,'subprocess-error.log'),result.stderr??'',{mode:0o600});}assert.equal(result.status,0,'Fixed member recovery operation failed');return result.stdout;}
const inspect=name=>JSON.parse(command(['inspect',name]))[0];
const names={pds:'pds-ass-tenant-'+tenant,edge:'pds-ass-edge-'+tenant,guard:'pds-ass-enrollment-'+tenant,protocol:'pds-ass-runtime-protocol-fixtures-1'};
const state=join(root,'agent-state'),host=join(work,'host');mkdirSync(host,{mode:0o700});privateDirectory(state);
const cloneNames=[prefix+'-pds-1',prefix+'-protocol-1'];let composePath,started=false;
function fenced(volumes){for(const volume of Object.values(volumes))assert.equal(command(['ps','--filter','volume='+volume,'--format','{{.ID}}']).trim(),'','Checkpoint source still has an active container');}
function stopClones(){for(const name of cloneNames){const value=spawnSync('docker',['inspect',name],{encoding:'utf8'});if(value.status!==0)continue;assert.equal(JSON.parse(value.stdout)[0].Config.Labels['pds-ass.member-recovery'],id);command(['stop','--time','10',name]);}}
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{stopClones();process.exit(130);});
let tool=0;
function run(args,options={}){const name=prefix+'-tool-'+(++tool);register(name);return command(['run','--rm','--name',name,'--label','pds-ass.member-recovery='+id,'--network','none','--memory','192m','--cpus','0.5','--pids-limit','64',...args],options);}
function mounts(volumes,path,readonly=true){return [...Object.entries(volumes).flatMap(([name,volume])=>['--mount',`type=volume,src=${volume},dst=/snapshot/${name}${readonly?',readonly':''}`]),'--mount',`type=bind,src=${path},dst=/snapshot/host${readonly?',readonly':''}`];}
function inventory(volumes,path,filesOnly=false){return JSON.parse(run([...mounts(volumes,path),'--mount',`type=bind,src=${resolve('deploy/local/fixtures/inventory.cjs')},dst=/inventory.cjs,readonly`,'--entrypoint','node',image,'/inventory.cjs',...(filesOnly?['--files-only']:[])]));}
function volumesFor(suffix){const volumes=Object.fromEntries(['data','blobs','protocol'].map(kind=>[kind,prefix+'-'+suffix+'-'+kind]));for(const name of Object.values(volumes))command(['volume','create','--label','pds-ass.member-recovery='+id,name]);return volumes;}
function execJSON(container,program,value){return JSON.parse(command(['exec','-i',container,'node','-e',program],{input:JSON.stringify(value)}));}
const nativeProgram=`const assert=require('node:assert/strict');const input=JSON.parse(require('node:fs').readFileSync(0));
async function rpc(method,body,token){const r=await fetch('http://localhost:2583/xrpc/'+method,{method:'POST',headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:JSON.stringify(body),signal:AbortSignal.timeout(15000)});assert.equal(r.status,200);return r.json();}
(async()=>{for(const account of input.logins){const session=await rpc('com.atproto.server.createSession',{identifier:account.handle,password:account.password});assert.equal(session.did,account.did);
let embed;if(input.blobs){const bytes=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=','base64');const response=await fetch('http://localhost:2583/xrpc/com.atproto.repo.uploadBlob',{method:'POST',headers:{'Content-Type':'image/png',Authorization:'Bearer '+session.accessJwt},body:bytes,signal:AbortSignal.timeout(15000)});assert.equal(response.status,200);const uploaded=await response.json();embed={$type:'app.bsky.embed.images',images:[{alt:'Disposable recovery pixel',image:uploaded.blob}]};}
const record={$type:'app.bsky.feed.post',text:input.text,createdAt:new Date().toISOString(),...(embed?{embed}:{})};const post=await rpc('com.atproto.repo.createRecord',{repo:account.did,collection:'app.bsky.feed.post',record},session.accessJwt);assert.ok(post.uri.startsWith('at://'+account.did+'/'));}console.log(JSON.stringify({authenticatedAccounts:input.logins.length,writes:input.logins.length}));})().catch(()=>process.exitCode=1);`;
const signedProgram=readFileSync('deploy/local/fixtures/blob-inventory.cjs','utf8').replace("'http://protocol-fixtures:2582'","'http://pds-ass-protocol-fixtures:2582'");

try {
 assert.ok(command(['context','inspect','--format','{{.Endpoints.docker.Host}}']).trim().startsWith('unix:///'));
 const pds=inspect(names.pds),protocol=inspect(names.protocol);
 assert.equal(pds.Config.Image,image);assert.equal(protocol.Config.Image,image);assert.ok(pds.State.Running&&protocol.State.Running);
 assert.equal(protocol.Config.Labels['com.docker.compose.project'],'pds-ass-runtime');assert.equal(protocol.Config.Labels['com.docker.compose.service'],'protocol-fixtures');assert.deepEqual(protocol.Config.Cmd,['node','/fixtures/protocol-services.mjs']);
 for(const name of [names.pds,names.edge,names.guard])assert.equal(inspect(name).Config.Labels['tv.subcult.pds-ass.instance'],tenant);
 assert.deepEqual(pds.Mounts.map(v=>v.Name).sort(),[names.pds+'-data',names.pds+'-blobs'].sort());
 const protocolVolume=protocol.Mounts.find(v=>v.Destination==='/fixture-data');assert.equal(protocolVolume.Name,'pds-ass-runtime_protocol-data');
 const source={data:names.pds+'-data',blobs:names.pds+'-blobs',protocol:protocolVolume.Name};
 const environment=Object.fromEntries(pds.Config.Env.map(value=>{const i=value.indexOf('=');return [value.slice(0,i),value.slice(i+1)];}));
 assert.equal(environment.PDS_DEV_MODE,'true');assert.equal(environment.PDS_DID_PLC_URL,'http://pds-ass-protocol-fixtures:2582');assert.ok(new URL('https://'+environment.PDS_HOSTNAME).hostname===tenant+'.localhost');
 const credentialsPath=join(state,tenant+'.json'),credentialsInfo=lstatSync(credentialsPath);assert.ok(credentialsInfo.isFile()&&!(credentialsInfo.mode&0o077));const credentials=JSON.parse(readFileSync(credentialsPath));assert.ok(credentials.jwt===environment.PDS_JWT_SECRET&&credentials.admin===environment.PDS_ADMIN_PASSWORD&&credentials.rotation===environment.PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX);
 report.stage='source-native-data';receipt();execJSON(names.pds,nativeProgram,{logins,blobs:true,text:'Source cold recovery '+id});
 const signedBefore=JSON.parse(command(['exec',names.pds,'node','-e',signedProgram]));assert.equal(signedBefore.repositories,2);assert.ok(signedBefore.referencedBlobs.length>=2);
 const files=[[credentialsPath,'credentials.json'],[join(state,tenant+'-tls.pem'),'tls.pem'],[join(state,tenant+'-enrollment','lease.json'),'lease.json'],...memberRecoveryFiles(state,tenant)];assert.equal(files.length,6);
 for(const [path,name] of files){const info=lstatSync(path);assert.ok(info.isFile()&&!(info.mode&0o077));copyFileSync(path,join(host,name));}
 const hash=value=>createHash('sha256').update(value).digest('hex'),protectedHashes=files.map(([path])=>hash(readFileSync(path)));
 const protocolSource=readFileSync('deploy/local/fixtures/protocol-services.mjs');assert.ok(protocolSource.equals(readFileSync(protocol.Mounts.find(v=>v.Destination==='/fixtures/protocol-services.mjs').Source)));writeFileSync(join(host,'protocol-services.mjs'),protocolSource,{mode:0o600});
 writeFileSync(join(host,'runtime.json'),JSON.stringify({environment,image}),{mode:0o600});
 report.stage='cold-source-checkpoint';receipt();const recoveryStarted=performance.now();command(['stop','--time','10',names.edge,names.guard,names.pds,names.protocol]);fenced(source);const capturedAt=new Date().toISOString();
 const coldBefore=inventory(source,host,true),staging=volumesFor('stage');
 const raw=join(work,'cold-source.tar'),rawFD=openSync(raw,'wx',0o600);try{run([...mounts(source,host),'--entrypoint','tar',image,'-C','/snapshot','-cf','-','data','blobs','protocol'],{stdio:['ignore',rawFD,'pipe']});}finally{closeSync(rawFD);}
 run([...mounts(staging,host,false),'--mount',`type=bind,src=${raw},dst=/source.tar,readonly`,'--entrypoint','tar',image,'-C','/snapshot','-xf','/source.tar']);
 run([...mounts(staging,host,false),'--mount',`type=bind,src=${resolve('deploy/local/fixtures/normalize-snapshot.cjs')},dst=/normalize.cjs,readonly`,'--entrypoint','node',image,'/normalize.cjs']);
 const normalized=inventory(staging,host);assert.ok(normalized.databases>=6);
 const plain=join(work,'normalized.tar'),plainFD=openSync(plain,'wx',0o600);try{run([...mounts(staging,host),'--entrypoint','tar',image,'-C','/snapshot','-cf','-','data','blobs','protocol','host'],{stdio:['ignore',plainFD,'pipe']});}finally{closeSync(plainFD);}
 const key=join(work,'recovery.key'),bundle=join(work,'checkpoint.pdsbk'),opened=join(work,'authenticated.tar');writeFileSync(key,randomBytes(32),{mode:0o600});await sealRecovery(plain,bundle,key,{version:1,id,scope:'pds-local',createdAt:new Date().toISOString()});await openRecovery(bundle,opened,key,{id,scope:'pds-local'});
 report.stage='fresh-storage-restore';receipt();const fresh=volumesFor('restore'),restoredHost=join(work,'restored-host');mkdirSync(restoredHost,{mode:0o700});
 run([...mounts(fresh,restoredHost,false),'--mount',`type=bind,src=${opened},dst=/backup.tar,readonly`,'--entrypoint','tar',image,'-C','/snapshot','-xf','/backup.tar']);assert.deepEqual(inventory(fresh,restoredHost),normalized);
 const candidate=join(work,'agent-candidate');mkdirSync(candidate,{mode:0o700});const members=restoreMemberRecoveryFiles(restoredHost,candidate,tenant);assert.equal(members.files,3);
 const networks=JSON.parse(command(['network','inspect',...command(['network','ls','-q']).trim().split(/\s+/)]));const [subnet]=recoverySubnets(networks.flatMap(n=>(n.IPAM?.Config??[]).map(v=>v.Subnet).filter(Boolean)));
 const bounded={restart:'no',pull_policy:'never',mem_limit:'768m',cpus:1,pids_limit:128,labels:{'pds-ass.member-recovery':id}};
 const compose={name:prefix,services:{pds:{...bounded,image,environment,volumes:[{type:'volume',source:'data',target:'/pds'},{type:'volume',source:'blobs',target:'/blobs'}],networks:['private']},protocol:{...bounded,image,command:protocol.Config.Cmd,environment:{PDS_FIXTURE_MODE:'local'},volumes:[{type:'volume',source:'protocol',target:'/fixture-data'},{type:'bind',source:join(restoredHost,'protocol-services.mjs'),target:'/fixtures/protocol-services.mjs',read_only:true}],networks:{private:{aliases:['pds-ass-protocol-fixtures']}},mem_limit:'192m',cpus:0.25,pids_limit:64}},networks:{private:{internal:true,ipam:{config:[{subnet}]}}},volumes:Object.fromEntries(Object.entries(fresh).map(([name,value])=>[name,{name:value,external:true}]))};
 composePath=join(work,'restore.compose.json');writeFileSync(composePath,composeLiterals(compose),{mode:0o600});for(const name of cloneNames)register(name);fenced(source);started=true;command(['compose','-f',composePath,'-p',prefix,'up','-d']);
 for(const name of cloneNames){const value=inspect(name);assert.equal(value.Config.Labels['pds-ass.member-recovery'],id);assert.ok(Object.values(value.NetworkSettings.Ports??{}).every(v=>!v?.length));assert.equal(Object.keys(value.NetworkSettings.Networks).length,1);assert.equal(JSON.parse(command(['network','inspect',Object.keys(value.NetworkSettings.Networks)[0]]))[0].Internal,true);}
 report.stage='restored-native-qualification';receipt();let signedRestored,startupReadRetries=0;
 for(let attempt=0;;attempt++){try{signedRestored=JSON.parse(command(['exec',cloneNames[0],'node','-e',signedProgram]));break;}catch(error){if(attempt>=39)throw error;startupReadRetries++;await new Promise(done=>setTimeout(done,250));}}
 assert.deepEqual(signedRestored,signedBefore);const writes=execJSON(cloneNames[0],nativeProgram,{logins,blobs:false,text:'Restored cold recovery '+id});assert.equal(writes.authenticatedAccounts,2);
 const afterWrites=JSON.parse(command(['exec',cloneNames[0],'node','-e',signedProgram]));assert.equal(afterWrites.signedRecords,signedBefore.signedRecords+2);assert.deepEqual(afterWrites.referencedBlobs,signedBefore.referencedBlobs);
 const coldRecoveryMilliseconds=Math.round(performance.now()-recoveryStarted);
 stopClones();fenced(fresh);fenced(source);assert.deepEqual(inventory(source,host,true),coldBefore);assert.deepEqual(files.map(([path])=>hash(readFileSync(path))),protectedHashes);
 delete report.failedOperation;
 Object.assign(report,{result:'passed',stage:'complete',completedAt:new Date().toISOString(),capturedAt,coldRecoveryMilliseconds,startupReadRetries,sourceWritersFenced:true,sourceFilesUnchanged:true,sourceProtectedMetadataRetained:true,authenticatedEncryptedCheckpoint:true,checkpointBytes:readFileSync(bundle).length,checkpointSha256:hash(readFileSync(bundle)),sqliteDatabases:normalized.databases,signedRepositories:signedBefore.repositories,signedRecords:signedBefore.signedRecords,referencedBlobs:signedBefore.referencedBlobs.length,restoredOriginalAccountLogins:2,restoredSignedWrites:2,memberHostFilesRetained:3,restoredContainersStopped:true,isolatedPrivateNetwork:true,includesSharedLocalIdentityFixture:true,agentActivated:false,publicEndpointCutoverQualified:false,independentCustodyQualified:false});receipt();console.log(JSON.stringify(report));
} catch {receipt();console.error('Disposable member cold recovery failed; inspect the private stage receipt.');process.exitCode=1;}
finally{if(started)stopClones();}
