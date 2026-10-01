// Checks retained counters across a controlled local ingress outage; native storage is untouched.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdirSync,writeFileSync,rmdirSync} from 'node:fs';
import {randomUUID} from 'node:crypto';
import {localOwnerClient} from './local-owner-client.mjs';
import {localNativeClient} from './local-native-client.mjs';
const id=process.argv[2],ownerName=process.argv[3]??'alice';assert.match(id??'',/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);assert.ok(['alice','bob'].includes(ownerName));
const client=await localOwnerClient(ownerName),foreign=await localOwnerClient(ownerName==='alice'?'bob':'alice');
const name='pds-ass-edge-'+id;
function command(cmd,args){const result=spawnSync(cmd,args,{encoding:'utf8',timeout:30000,maxBuffer:1024*1024});assert.equal(result.status,0,'Fixed local usage qualification command');return result.stdout;}
function scan(){command('./scripts/monitor-local.sh',['once']);}
const inspected=JSON.parse(command('docker',['inspect',name]))[0];assert.equal(inspected.Config.Labels['tv.subcult.pds-ass.instance'],id);assert.equal(inspected.Config.Labels['tv.subcult.pds-ass.role'],'ingress');assert.ok(inspected.State.Running);
const root='.local/usage-qualification/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});const lock='.local/usage-qualification/qualification.lock';mkdirSync(lock,{mode:0o700});process.on('exit',()=>rmdirSync(lock));
scan();const before=await client.api('/api/deployments/'+id);assert.equal(before.status,200);const usage=before.value.usage;assert.equal(usage.state,'observed');assert.ok(usage.dataBytes>0 && usage.blobBytes>0 && usage.accounts===1);assert.equal(usage.storageLimit,'unconfigured');assert.ok(!('filesystemAvailableBytes' in usage)&&!('filesystemBytes' in usage));assert.equal((await foreign.api('/api/deployments/'+id)).status,404);
let stopAttempted=false;const restore=()=>{if(stopAttempted)command('docker',['start',name]);};for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>{restore();process.exit(130)});
try{stopAttempted=true;command('docker',['stop','--time','5',name]);for(let attempt=0;attempt<2;attempt++)scan();const failed=await client.api('/api/deployments/'+id);assert.equal(failed.status,200);assert.equal(failed.value.usage.state,'unavailable');assert.equal(failed.value.usage.dataBytes,usage.dataBytes);assert.equal(failed.value.usage.blobBytes,usage.blobBytes);assert.equal(failed.value.usage.checkedAt,usage.checkedAt);}finally{restore();}
const native=localNativeClient(id,before.value.endpoint);for(let attempt=0;;attempt++){try{await native('com.atproto.server.describeServer');break;}catch(error){if(attempt>=49)throw error;await new Promise(resolve=>setTimeout(resolve,100));}}
scan();const recovered=await client.api('/api/deployments/'+id);assert.equal(recovered.status,200);assert.equal(recovered.value.usage.state,'observed');assert.equal(JSON.parse(command('docker',['inspect',name]))[0].Id,inspected.Id);assert.equal(recovered.value.endpoint,before.value.endpoint);
const report={completedAt:new Date().toISOString(),deployment:id,observedTenantBytes:true,accountCount:true,hostHeadroomHidden:true,foreignOwnerDenied:true,outageCountersRetained:true,outageShownUnavailable:true,recoveryObserved:true,stableIngressIdentity:true,storageQuotaEnforced:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{flag:'wx',mode:0o600});console.log(JSON.stringify({...report,report:root+'/report.json'}));
