import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, renameSync, lstatSync, mkdirSync } from 'node:fs';
import { request } from 'node:https';
import { spawnSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { resolve, join } from 'node:path';
const [id]=process.argv.slice(2);
assert.match(id??'',/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
const port=36000+parseInt(id.slice(0,4),16)%10000;
const host=`${id}.localhost`;
const cert=readFileSync(`.local/node-agent/${id}-public.crt`);
const directory=resolve('.local/node-agent/state',id+'-enrollment');
assert.ok(lstatSync(directory).isDirectory() && !(lstatSync(directory).mode&0o077));
function docker(...args) { const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:1024*1024}); assert.equal(result.status,0,'Enrollment ingress qualification Docker operation'); return result.stdout; }
assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const tenant=JSON.parse(docker('inspect',`pds-ass-tenant-${id}`))[0];
assert.equal(tenant.Config.Labels['tv.subcult.pds-ass.instance'],id);
assert.ok(tenant.Config.Env.includes('PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582'));
for(const bindings of Object.values(tenant.NetworkSettings.Ports)) assert.ok(!bindings?.length,'Direct PDS publishing bypasses admission');
const guardName=`pds-ass-enrollment-${id}`;
const guard=JSON.parse(docker('inspect',guardName))[0];
assert.equal(guard.Config.Labels['tv.subcult.pds-ass.instance'],id);
assert.equal(guard.Config.Labels['tv.subcult.pds-ass.role'],'enrollment-guard');
assert.ok(guard.State.Running);
function lease(value) {
 const encoded=JSON.stringify(value),temporary=join(directory,randomUUID()+'.tmp');
 writeFileSync(temporary,encoded,{mode:0o600,flag:'wx'}); renameSync(temporary,join(directory,'lease.json'));
 if(stopped) return;
 const digest=createHash('sha256').update(encoded).digest('hex');
 const observed=docker('exec',guardName,'node','-e',"const fs=require('node:fs'),crypto=require('node:crypto');process.stdout.write(crypto.createHash('sha256').update(fs.readFileSync('/enrollment/lease.json')).digest('hex'))").trim();
 assert.equal(observed,digest,'Published policy is not yet visible to the isolated guard');
}
function call(path,method='POST') { return new Promise((done,reject)=>{
 const req=request({hostname:'127.0.0.1',port,servername:host,ca:cert,path,method,headers:{Host:`${host}:${port}`,'Content-Type':'application/json'},timeout:5000},response=>{response.resume();response.on('end',()=>done(response.statusCode));});
 req.on('error',reject);req.on('timeout',()=>req.destroy(new Error('Local ingress timeout')));req.end(method==='POST'?'{}':undefined);
}); }
const paths=['/xrpc/com.atproto.server.createAccount','/XRPC/COM.ATPROTO.SERVER.CREATEACCOUNT','/xrpc/com%2Eatproto%2Eserver%2EcreateAccount/','/@atproto/oauth-provider/~api/sign-up'];
const now=Date.now();
const base={instanceId:id,ticket:randomUUID(),state:'open',issuedAt:now,expiresAt:now+900000};
let stopped=false;
try {
 for(const bad of [{...base,state:'closed'},{...base,instanceId:randomUUID()},{...base,issuedAt:now-900000,expiresAt:now-1},{...base,issuedAt:now+60000,expiresAt:now+120000},{...base,expiresAt:now+900001}]) {
  lease(bad);for(const path of paths) assert.equal(await call(path),403,'Registration lease failed open');
 }
 lease(base);
 assert.equal(await call(paths[0]),400,'Current lease did not reach native signup validation');
 stopped=true;docker('stop',guardName);
 for(const path of paths) assert.equal(await call(path),502,'Stopped admission guard failed open');
 assert.equal(await call('/xrpc/_health','GET'),200);
} finally {
 try { lease({...base,state:'closed'}); } finally { if(stopped) docker('start',guardName); }
}
let recovered=false;
for(let attempt=0;attempt<20;attempt++) { if(await call(paths[0])===403) { recovered=true;break; } await new Promise(done=>setTimeout(done,100)); }
assert.ok(recovered,'Restarted admission guard did not become ready');
const tlsPath=`.local/node-agent/state/${id}-tls.pem`;
const proof=JSON.parse(readFileSync(`.local/local-identity-upgrades/${id}/identity-proof.json`,'utf8'));
assert.equal(createHash('sha256').update(readFileSync(tlsPath)).digest('hex'),proof.tlsSha256);
assert.equal(proof.endpoint,`PDS_HOSTNAME=${host}:${port}`);
mkdirSync('.local/enrollment-ingress',{recursive:true,mode:0o700});
const report={instance:id,wrongTenantDenied:true,expiredDenied:true,futureDenied:true,overlongDenied:true,aliasesGuarded:true,stoppedGuardDenied:true,noDirectPDSPort:true,stableTLS:true,stableEndpoint:true,completedAt:new Date().toISOString()};
writeFileSync(`.local/enrollment-ingress/${randomUUID()}.json`,JSON.stringify(report,null,2)+'\n',{mode:0o600,flag:'wx'});
console.log(JSON.stringify(report));
