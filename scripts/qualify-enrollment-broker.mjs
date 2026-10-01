// Exercise the fixed private agent invitation broker without completing a real account.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID, randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync, mkdirSync, renameSync, lstatSync } from 'node:fs';
import { request } from 'node:https';
import { resolve } from 'node:path';
const [id]=process.argv.slice(2);
assert.match(id??'',/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
const run=randomUUID(),directory=resolve('.local/enrollment-broker',run);
mkdirSync(directory,{recursive:true,mode:0o700});assert.ok(!(lstatSync(directory).mode&0o077));
const pki=resolve('.local/node-agent/certs');
function enroll(ticket,extra={}) {
 const result=spawnSync('curl',['--silent','--show-error','--max-time','30','--cacert',pki+'/ca.crt','--cert',pki+'/worker.crt','--key',pki+'/worker.key','-X','POST','-H','Content-Type: application/json','--data',JSON.stringify({ticket,...extra}),'-w','\n%{http_code}',`https://127.0.0.1:9091/v1/instances/${id}/enrollment`],{encoding:'utf8',timeout:35000,maxBuffer:4096});
 assert.equal(result.status,0,'Private enrollment agent request failed');
 const boundary=result.stdout.lastIndexOf('\n'),status=Number(result.stdout.slice(boundary+1));
 return {status,body:result.stdout.slice(0,boundary)};
}
function confirmed(ticket) {
 const response=enroll(ticket);assert.equal(response.status,200,'Private enrollment could not be confirmed');
 const value=JSON.parse(response.body);assert.equal(value.instanceId,id);assert.equal(value.ticket,ticket);
 const credentials=JSON.parse(readFileSync(`.local/node-agent/state/${id}.json`,'utf8'));
 for(const secret of Object.values(credentials)) assert.ok(!response.body.includes(secret),'PDS administration credential escaped the agent');
 return value;
}
const firstTicket=randomUUID();
assert.equal(enroll(firstTicket,{image:'unsupported'}).status,400);
const first=confirmed(firstTicket);assert.equal(first.state,'invitation_ready');
const replay=confirmed(firstTicket);
assert.ok(replay.code===first.code && replay.expiresAt===first.expiresAt,'Stable ticket replay changed its code or expiration');
const nextTicket=randomUUID(),next=confirmed(nextTicket);
assert.equal(next.state,'invitation_ready');assert.ok(next.code!==first.code,'Replacement reused a revoked invitation');
const host=`${id}.localhost`,port=36000+parseInt(id.slice(0,4),16)%10000,cert=readFileSync(`.local/node-agent/${id}-public.crt`);
async function signup(code) { return new Promise((done,reject)=>{
 const body=JSON.stringify({handle:`revoked-${run.slice(0,8)}.test`,email:`revoked-${run.slice(0,8)}@fixture.invalid`,password:randomBytes(24).toString('hex'),inviteCode:code});
 const req=request({hostname:'127.0.0.1',port,servername:host,ca:cert,path:'/xrpc/com.atproto.server.createAccount',method:'POST',headers:{Host:`${host}:${port}`,'Content-Type':'application/json'},timeout:5000},res=>{let text='';res.on('data',value=>text+=value);res.on('end',()=>done({status:res.statusCode,text}));});req.on('error',reject);req.on('timeout',()=>req.destroy(new Error('Local signup timeout')));req.end(body);
}); }
const revoked=await signup(first.code);assert.equal(revoked.status,400,'Revoked invitation remained valid');
const leasePath=`.local/node-agent/state/${id}-enrollment/lease.json`;
const lease=JSON.parse(readFileSync(leasePath,'utf8'));
lease.expiresAt=Date.now()-1000;lease.issuedAt=lease.expiresAt-900000;
writeFileSync(leasePath+'.qualification',JSON.stringify(lease),{mode:0o600,flag:'wx'});renameSync(leasePath+'.qualification',leasePath);
const expired=confirmed(nextTicket);
assert.equal(expired.state,'expired');assert.ok(!expired.code,'Expired replay disclosed a usable code');
assert.equal((await signup(next.code)).status,403,'Expired native code bypassed admission');
writeFileSync(directory+'/private-invitations.json',JSON.stringify({first,next,expired}),{mode:0o600,flag:'wx'});
const report={instance:id,stableReplay:true,revokedCodeDenied:true,expiredCodeDenied:true,unsupportedConfigurationDenied:true,noAdministratorCredentials:true,completedAt:new Date().toISOString()};
writeFileSync(directory+'/report.json',JSON.stringify(report,null,2)+'\n',{mode:0o600,flag:'wx'});
console.log(JSON.stringify(report));
