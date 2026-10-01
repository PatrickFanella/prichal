// Owner-to-native-PDS journey on a retained disposable local deployment only.
import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import https from 'node:https';
import { localOwnerClient } from './local-owner-client.mjs';
const deployment = process.argv[2];
assert.match(deployment ?? '', /^[a-f0-9-]{36}$/);
const owner = await localOwnerClient('alice'), other = await localOwnerClient('bob');
const directory = 'http://localhost:2582';
assert.equal((await (await fetch(directory + '/_health')).json()).localFixture, true);
const resume = process.argv[3];
assert.ok(!resume || /^\.local\/owner-enrollment\/[a-f0-9-]{36}$/.test(resume), 'Private local journey path required');
const root = resume ?? `.local/owner-enrollment/${randomUUID()}`;
mkdirSync(root, { recursive: true, mode: 0o700 });
const ca = readFileSync(`.local/node-agent/${deployment}-public.crt`);
let endpoint;
async function native(method, body, authorization, expected = 200) {
 const url = new URL('/xrpc/' + method, endpoint);
 const result = await new Promise((resolve, reject) => {
  const request = https.request({ hostname: '127.0.0.1', port: url.port, servername: url.hostname, ca, path: url.pathname + url.search, method: body === undefined ? 'GET' : 'POST', headers: { Host: url.host, ...(body == null ? {} : {'Content-Type':'application/json'}), ...(authorization ? {Authorization:authorization} : {}) }, timeout:15000 }, response => {
   const chunks=[]; let bytes=0;
   response.on('data', chunk => { bytes+=chunk.length; if(bytes>1024*1024) request.destroy(new Error('Native response exceeds qualification bound')); else chunks.push(chunk); });
   response.on('end',()=>resolve({status:response.statusCode,body:Buffer.concat(chunks).toString()}));
  });
  request.on('timeout',()=>request.destroy(new Error('Native request timed out')));request.on('error',reject);
  request.end(body==null?undefined:JSON.stringify(body));
 });
 assert.equal(result.status,expected,'Native '+method.split('?')[0]+' status');
 return expected===200 && result.body ? JSON.parse(result.body) : undefined;
}
async function wait(id, state) {
 for(let n=0;n<100;n++) {
  const result=await owner.api('/api/enrollments/'+id);
  assert.equal(result.status,200,'Owner enrollment read');
  if(result.value.state===state) return result.value;
  assert.ok(!['failed','expired'].includes(result.value.state),'Enrollment could not complete');
  await new Promise(resolve=>setTimeout(resolve,500));
 }
 throw new Error('Owner enrollment checkpoint timed out');
}
let operation;
let checks;
if (!resume) {
const key=randomUUID();
const requested=await owner.api(`/api/deployments/${deployment}/enrollment`,{idempotencyKey:key,recover:false});
assert.equal(requested.status,200,'Owner invitation submission');
operation=await wait(requested.value.id,'invitation_ready');
endpoint=new URL(operation.accountUrl).origin;
assert.equal(new URL(endpoint).hostname,`${deployment}.localhost`);
assert.equal(new URL(endpoint).protocol,'https:');
const replay=await owner.api(`/api/deployments/${deployment}/enrollment`,{idempotencyKey:key,recover:false});
assert.equal(replay.status,200);assert.ok(replay.value.code===operation.code && replay.value.expiresAt===operation.expiresAt,'Replay changed invitation');
assert.equal((await other.api('/api/enrollments/'+operation.id)).status,404);
assert.equal((await other.api(`/api/deployments/${deployment}/enrollment`,{idempotencyKey:randomUUID(),recover:true})).status,404);
const oldCode=operation.code;
const recovery=await owner.api(`/api/deployments/${deployment}/enrollment`,{idempotencyKey:randomUUID(),recover:true});
assert.equal(recovery.status,200,'Owner invitation recovery');
operation=await wait(recovery.value.id,'invitation_ready');
assert.ok(operation.code!==oldCode,'Recovery retained old invitation');
await native('com.atproto.server.createAccount',{handle:'stale-code.test',email:'stale-code@fixture.invalid',password:randomBytes(24).toString('hex'),inviteCode:oldCode},undefined,400);
checks={ownerReplay:true,foreignOwnerDenied:true,recoveryRevokedOldCode:true};
writeFileSync(root+'/journey.json',JSON.stringify({deployment,operationId:operation.id,checks}),{flag:'wx',mode:0o600});
} else {
 const retained=JSON.parse(readFileSync(root+'/journey.json'));
 assert.equal(retained.deployment,deployment,'Wrong retained deployment');
 checks=retained.checks ?? {}; // Legacy interrupted drills cannot claim unrecorded checks.
 const found=await owner.api('/api/enrollments/'+retained.operationId);
 assert.equal(found.status,200,'Retained enrollment');
 operation=found.value;
 assert.ok(['invitation_ready','account_created'].includes(operation.state),'Retained journey already completed or expired');
 endpoint=new URL(operation.accountUrl).origin;
}
assert.equal(new URL(endpoint).hostname,`${deployment}.localhost`);
assert.equal(new URL(endpoint).protocol,'https:');
const tag=randomBytes(5).toString('hex');
let account={handle:`creator-${tag}.test`,email:`creator-${tag}@fixture.invalid`,password:randomBytes(24).toString('hex')};
if(existsSync(root+'/account.json')) account=JSON.parse(readFileSync(root+'/account.json'));
let session;
if(operation.state==='account_created') {
 account=JSON.parse(readFileSync(root+'/account.json'));
 session=await native('com.atproto.server.createSession',{identifier:account.handle,password:account.password});
} else {
 writeFileSync(root+'/account.json',JSON.stringify(account),{mode:0o600});
 session=await native('com.atproto.server.createAccount',{...account,inviteCode:operation.code});
}
assert.match(session.did,/^did:plc:[a-z2-7]{24}$/);
writeFileSync(root+'/account.json',JSON.stringify({...account,did:session.did,endpoint}),{mode:0o600});
// Admission closes on the repository independently of a management refresh.
await native('com.atproto.server.createAccount',{},undefined,403);
if(operation.state==='invitation_ready') assert.equal((await owner.api('/api/enrollments/'+operation.id)).value.state,'invitation_ready');
assert.equal((await owner.api('/api/enrollments/'+operation.id+'/refresh',{})).status,200);
const created=await wait(operation.id,'account_created');
assert.ok(!created.code && created.did===session.did && created.handle===account.handle);
const authorization='Bearer '+session.accessJwt;
await native('com.atproto.server.requestEmailConfirmation',null,authorization);
const messages=await (await fetch(directory+'/_fixture/mail?recipient='+encodeURIComponent(account.email))).json();
assert.equal(messages.length,1,'Local confirmation mail');
const mail=messages[0].message.replace(/=\r\n/g,'').replace(/=([0-9A-F]{2})/gi,(_,hex)=>String.fromCharCode(parseInt(hex,16)));
const token=/verify-email\?code=([A-Za-z0-9-]+)/.exec(mail)?.[1];
assert.ok(token,'Local confirmation token missing');
await native('com.atproto.server.confirmEmail',{email:account.email,token},authorization);
await native('com.atproto.server.confirmEmail',{email:account.email,token},authorization,400);
const login=await native('com.atproto.server.createSession',{identifier:account.handle,password:account.password});
assert.ok(login.did===session.did && login.emailConfirmed===true,'Native account login or confirmation failed');
await new Promise(resolve=>setTimeout(resolve,5100));
assert.equal((await owner.api('/api/enrollments/'+operation.id+'/refresh',{})).status,200);
const verified=await wait(operation.id,'verified');assert.ok(!verified.code && verified.emailConfirmed);
const document=await (await fetch(directory+'/'+session.did)).json();
assert.equal(document.service.find(value=>value.id.endsWith('#atproto_pds')).serviceEndpoint,endpoint);
assert.equal((await owner.api(`/api/deployments/${deployment}/enrollment`,{idempotencyKey:randomUUID(),recover:true})).status,409,'Completed account reopened enrollment');
const report={completedAt:new Date().toISOString(),deployment,operationId:operation.id,...checks,firstAccountAdmissionClosed:true,emailConfirmed:true,nativeLogin:true,stableIdentityEndpoint:true,managementState:verified.state,liveFederationQualified:false};
writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{flag:'wx',mode:0o600});
console.log(JSON.stringify({...report,report:root+'/report.json'}));
