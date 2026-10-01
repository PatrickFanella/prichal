// Disposable loopback qualification. The operator pauses only the local worker for prepare.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, lstatSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { localOwnerClient } from './local-owner-client.mjs';
const api='http://localhost:5173', binary=resolve('.local/bin/pds-ass');
const base=resolve('.local/cli-qualification');mkdirSync(base,{recursive:true,mode:0o700});
function cli(args,expected=0){const r=spawnSync(binary,args,{encoding:'utf8',timeout:30000});assert.equal(r.status,expected,'CLI exit status mismatch');return {out:r.stdout,err:r.stderr,value:r.stdout.trim()?JSON.parse(r.stdout):null};}
async function login(owner,dir){
 mkdirSync(dir,{recursive:true,mode:0o700});const browser=await localOwnerClient(owner);
 const child=spawn(binary,['login','--api-url',api,'--no-browser','--config-dir',dir],{stdio:['ignore','pipe','pipe']});let out='',err='',approved=false,approvalError;
 const timer=setTimeout(()=>child.kill('SIGINT'),45000);
 child.stdout.on('data',b=>{out+=b});child.stderr.on('data',b=>{err+=b;const match=/Open http:\/\/localhost:5173\/device and enter ([A-Z2-7]{4}-[A-Z2-7]{4})/.exec(err);if(match&&!approved){approved=true;void(async()=>{const review=await browser.api('/api/cli/review',{userCode:match[1]});assert.equal(review.status,200);const decision=await browser.api('/api/cli/approve',{userCode:match[1],approved:true});assert.equal(decision.status,200)})().catch(e=>{approvalError=e;child.kill('SIGINT')})}});
 const status=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('exit',resolve)});clearTimeout(timer);if(approvalError)throw approvalError;assert.equal(status,0,'CLI browser approval failed');
 const credential=JSON.parse(readFileSync(join(dir,'credentials.json'),'utf8'));assert.equal(credential.token.length,43);assert.ok(!(lstatSync(join(dir,'credentials.json')).mode&0o077));assert.ok(!out.includes(credential.token)&&!err.includes(credential.token),'Credential appeared in output');
 assert.equal(JSON.parse(out).signedIn,true);return {browser,credential};
}
if(process.argv[2]==='prepare'){
 assert.equal(process.env.PDS_LOCAL_WORKER_PAUSED,'true','Pause only the local worker for the pending-job drill');
 const id=randomUUID(),work=join(base,id);mkdirSync(work,{mode:0o700});const aliceDir=join(work,'alice'),bobDir=join(work,'bob');
 const alice=await login('alice',aliceDir),bob=await login('bob',bobDir);
 const aOrgs=cli(['organizations','list','--config-dir',aliceDir]).value,bOrgs=cli(['organizations','list','--config-dir',bobDir]).value;assert.ok(aOrgs.length&&bOrgs.length);
 const aliceOrg=aOrgs.find(o=>o.id==='da1d3724-993f-443f-a6bf-798c3f8c9f41');assert.ok(aliceOrg,'Expected disposable Alice organization required');
 const bobDeployment=bOrgs.flatMap(o=>cli(['list','--organization',o.id,'--config-dir',bobDir]).value).find(d=>d.kind==='agent'&&d.state==='ready');assert.ok(bobDeployment,'Populated Bob native placement required');
 assert.equal(cli(['status','--deployment',bobDeployment.id,'--config-dir',bobDir]).value.id,bobDeployment.id);
 cli(['status','--deployment',bobDeployment.id,'--config-dir',aliceDir],1);
 const blocked=await fetch(api+'/api/organizations',{method:'POST',headers:{Authorization:'Bearer '+alice.credential.token,'Content-Type':'application/json'},body:JSON.stringify({name:'must-not-create'})});assert.equal(blocked.status,403);
 const key=randomUUID(),name='cli-'+id.slice(0,8);const args=['deploy','--organization',aliceOrg.id,'--name',name,'--kind','test','--idempotency-key',key,'--config-dir',aliceDir,'--wait'];
 const child=spawn(binary,args,{stdio:['ignore','pipe','pipe']});let err='',out='',interrupted=false,deployment;
 child.stderr.on('data',b=>{err+=b;const match=/Deployment ([0-9a-f-]{36}): queued/.exec(err);if(match&&!interrupted){deployment=match[1];interrupted=true;child.kill('SIGINT')}});child.stdout.on('data',b=>{out+=b});
 const timer=setTimeout(()=>child.kill('SIGINT'),15000);const status=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('exit',resolve)});clearTimeout(timer);assert.equal(status,130,'Expected interrupted CLI wait');assert.ok(interrupted&&deployment);assert.ok(!err.includes(alice.credential.token)&&!out.includes(alice.credential.token));
 const retry=cli(args.filter(arg=>arg!=='--wait'),3).value;assert.equal(retry.id,deployment,'Replay created another deployment');assert.equal(retry.state,'queued');
 const report={id,work,aliceDir,bobDir,key,name,organization:aliceOrg.id,deployment,bobDeployment:bobDeployment.id,interruptedExit:130,stableReplay:true,ownerIsolation:true,scopedMutationDenied:true,credentialsPrivate:true,phase:'pending',preparedAt:new Date().toISOString()};writeFileSync(join(work,'report.json'),JSON.stringify(report,null,2),{mode:0o600});writeFileSync(join(base,'latest.json'),JSON.stringify({work}),{mode:0o600});
 console.log(JSON.stringify({id,phase:'pending',interruptedExit:130,stableReplay:true,ownerIsolation:true,scopedMutationDenied:true,credentialsPrivate:true}));
}else if(process.argv[2]==='resume'){
 const {work}=JSON.parse(readFileSync(join(base,'latest.json'),'utf8'));assert.ok(work.startsWith(base+'/'));const report=JSON.parse(readFileSync(join(work,'report.json'),'utf8'));assert.equal(report.phase,'pending');
 const ready=cli(['status','--deployment',report.deployment,'--wait','--config-dir',report.aliceDir]).value;assert.equal(ready.state,'ready');assert.equal(ready.kind,'test');
 const matches=cli(['list','--organization',report.organization,'--config-dir',report.aliceDir]).value.filter(d=>d.id===report.deployment);assert.equal(matches.length,1);
 const alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob');const aliceCredential=JSON.parse(readFileSync(join(report.aliceDir,'credentials.json'),'utf8'));
 assert.equal((await bob.api('/api/cli/credentials/'+aliceCredential.credentialId+'/revoke',{})).status,404);
 assert.equal((await alice.api('/api/cli/credentials/'+aliceCredential.credentialId+'/revoke',{})).status,200);cli(['organizations','list','--config-dir',report.aliceDir],1);cli(['logout','--config-dir',report.aliceDir]);cli(['logout','--config-dir',report.bobDir]);
 Object.assign(report,{phase:'verified',ready:true,singleDeployment:true,individualRevocation:true,foreignRevocationDenied:true,completedAt:new Date().toISOString()});writeFileSync(join(work,'report.json'),JSON.stringify(report,null,2),{mode:0o600});console.log(JSON.stringify({id:report.id,phase:'verified',ready:true,singleDeployment:true,individualRevocation:true,foreignRevocationDenied:true,simulationOnly:true}));
}else throw new Error('Use prepare or resume on disposable local infrastructure');
