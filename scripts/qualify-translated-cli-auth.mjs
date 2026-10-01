// Local disposable authorization only. No deployment submission or native mutation.
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {spawn,spawnSync} from 'node:child_process';
import {existsSync,lstatSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {localOwnerClient} from './local-owner-client.mjs';
assert.equal(process.platform,'darwin');assert.equal(process.arch,'arm64');
const id=randomUUID(),root=resolve('.local/translated-cli-auth/'+id),binary=resolve('.local/cli-build/pds-ass-darwin-amd64'),api='http://localhost:5173';mkdirSync(root,{recursive:true,mode:0o700});
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const digest=path=>createHash('sha256').update(readFileSync(path)).digest('hex');
const beforeBinary=digest(binary),entry=readFileSync('.local/cli-build/SHA256SUMS','utf8').split('\n').find(v=>v.endsWith('  pds-ass-darwin-amd64'));assert.equal(entry,beforeBinary+'  pds-ass-darwin-amd64');
function command(args,input){const r=spawnSync('docker',args,{input,encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(r.status,0,'Local source inventory failed');return r.stdout.trim();}
function inventory(){return JSON.parse(command(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT json_build_object('deployments',(SELECT json_agg(d ORDER BY id) FROM deployments d),'jobs',(SELECT count(*) FROM river_job),'exports',(SELECT json_agg(e ORDER BY id) FROM export_operations e),'enrollment',(SELECT json_agg(e ORDER BY id) FROM enrollment_operations e),'accessAudits',(SELECT count(*) FROM organization_access_events),'choices',(SELECT count(*) FROM endpoint_choices))`]));}
const instances=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade','8d234722-f0ad-47b2-8ef8-10f85d0c13b9'],nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function custody(){return instances.map(instance=>({instance,native:JSON.parse(command(['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>digest('.local/node-agent/state/'+instance+suffix))}));}
const sourceBefore=inventory(),custodyBefore=custody(),sessions=[];
function cli(args,expected=0){const r=spawnSync('arch',['-x86_64',binary,...args],{input:'',encoding:'utf8',timeout:15000,maxBuffer:1024*1024});assert.equal(r.status,expected,'Translated CLI exit mismatch');for(const session of sessions)assert.ok(!r.stdout.includes(session.credential.token)&&!r.stderr.includes(session.credential.token),'Credential leaked in CLI output');if(expected!==0)assert.equal(r.stdout,'');return r.stdout.trim()?JSON.parse(r.stdout):null;}
async function login(owner){
 const dir=join(root,owner);mkdirSync(dir,{mode:0o700});const browser=await localOwnerClient(owner);
 const child=spawn('arch',['-x86_64',binary,'login','--api-url',api,'--no-browser','--config-dir',dir],{stdio:['ignore','pipe','pipe']});let out='',err='',approved=false,approvalError;
 const timer=setTimeout(()=>child.kill('SIGINT'),45000);
 child.stdout.on('data',b=>{out+=b});child.stderr.on('data',b=>{err+=b;const match=/Open http:\/\/localhost:5173\/device and enter ([A-Z2-7]{4}-[A-Z2-7]{4})/.exec(err);if(match&&!approved){approved=true;void(async()=>{assert.equal((await browser.api('/api/cli/review',{userCode:match[1]})).status,200);assert.equal((await browser.api('/api/cli/approve',{userCode:match[1],approved:true})).status,200);})().catch(e=>{approvalError=e;child.kill('SIGINT')});}});
 let status;try {status=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('exit',resolve);});}finally {clearTimeout(timer);}
 // Register any issued credential for cleanup before checking the outcome.
 if(existsSync(join(dir,'credentials.json'))){const credential=JSON.parse(readFileSync(join(dir,'credentials.json'),'utf8'));assert.ok(uuid.test(credential.credentialId)&&typeof credential.token==='string'&&credential.token.length===43&&credential.apiUrl===api);const session={owner,dir,browser,credential};sessions.push(session);assert.equal(lstatSync(dir).mode&0o777,0o700);assert.equal(lstatSync(join(dir,'credentials.json')).mode&0o777,0o600);assert.ok(!out.includes(credential.token)&&!err.includes(credential.token));}
 if(approvalError)throw approvalError;assert.equal(status,0,'Translated CLI approval failed');assert.equal(JSON.parse(out).signedIn,true);const session=sessions.find(v=>v.owner===owner);assert.ok(session);return session;
}
let complete=false;
try {
 const alice=await login('alice'),bob=await login('bob');
 const aliceOrgs=cli(['organizations','list','--config-dir',alice.dir]),bobOrgs=cli(['organizations','list','--config-dir',bob.dir]);assert.ok(aliceOrgs.some(o=>o.id==='da1d3724-993f-443f-a6bf-798c3f8c9f41'));assert.ok(bobOrgs.some(o=>o.id==='0406b74a-af6f-4555-a7cd-2d2f1f175c1c'));
 assert.equal(cli(['status','--deployment',instances[1],'--config-dir',bob.dir]).id,instances[1]);cli(['status','--deployment',instances[1],'--config-dir',alice.dir],1);
 assert.ok(cli(['list','--organization','0406b74a-af6f-4555-a7cd-2d2f1f175c1c','--config-dir',bob.dir]).some(d=>d.id===instances[1]));
 assert.equal((await alice.browser.api('/api/cli/credentials/'+bob.credential.credentialId+'/revoke',{})).status,404);
 assert.equal((await bob.browser.api('/api/cli/credentials/'+bob.credential.credentialId+'/revoke',{})).status,200);cli(['organizations','list','--config-dir',bob.dir],1);assert.equal(cli(['logout','--config-dir',bob.dir]).signedOut,true);
 assert.equal(cli(['logout','--config-dir',alice.dir]).signedOut,true);
 for(const session of sessions){assert.equal(existsSync(join(session.dir,'credentials.json')),false);const check=await session.browser.api('/api/cli/credentials');assert.equal(check.status,200);assert.ok(!check.value.some(v=>v.id===session.credential.credentialId));}
 assert.equal(digest(binary),beforeBinary);assert.deepEqual(inventory(),sourceBefore);assert.deepEqual(custody(),custodyBefore);complete=true;
 const report={id,completedAt:new Date().toISOString(),platform:'darwin-amd64',runtime:'Rosetta translation',localDeviceAuthorization:true,ownerScopedReads:true,foreignRevocationDenied:true,revocationEnforced:true,logoutRemovedPrivateCredentials:true,credentialsAbsentFromOutput:true,sourceWorkNativeCustodyAndBinaryUnchanged:true,deploymentSubmitted:false,liveOrIntelHardwareQualified:false};writeFileSync(join(root,'report.json'),JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
} finally {
 if(!complete)for(const session of sessions){try {await session.browser.api('/api/cli/credentials/'+session.credential.credentialId+'/revoke',{});cli(['logout','--config-dir',session.dir]);}catch {console.error('Disposable CLI credential cleanup could not be confirmed; inspect the private qualification directory.');}}
}
