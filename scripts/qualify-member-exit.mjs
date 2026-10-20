// Member deactivation qualification uses only fresh isolated PDS accounts.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { generateKeyPairSync, randomBytes, randomUUID } from 'node:crypto';
import { mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const image = 'ghcr.io/bluesky-social/pds:0.4@sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336';
const edgeImage = 'caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d';
const id = randomUUID();
const prefix = `prichal-member-exit-${id}`;
const root = resolve('.local/pds-member-exit', id);
mkdirSync(root, { recursive: true, mode: 0o700 });
const label = `tv.subcult.prichal.member-exit-qualification=${id}`;
const network = `${prefix}-net`;
const volumes = Object.fromEntries(['data', 'blobs', 'protocol'].map(role => [role, `${prefix}-${role}`]));
const names = Object.fromEntries(['pds', 'edge', 'protocol'].map(role => [role, `${prefix}-${role}`]));
const created = [];
let failed = true;
function docker(args, input) {
  const result = spawnSync('docker', args, { input, encoding: 'utf8', timeout: 60000, maxBuffer: 2 * 1024 * 1024 });
  // Docker errors can include environment values; retain no raw diagnostic output.
  assert.equal(result.status, 0, `Disposable Docker operation failed: ${args[0]}`);
  return result.stdout.trim();
}
function inspect(name) { return JSON.parse(docker(['inspect', name]))[0]; }
function owned(name) {
  const row = inspect(name);
  assert.equal(row.Config.Labels['tv.subcult.prichal.member-exit-qualification'], id);
  assert.equal(row.HostConfig.Privileged, false);
  assert.equal(row.HostConfig.NetworkMode, network);
  assert.ok(!row.HostConfig.PortBindings || Object.keys(row.HostConfig.PortBindings).length === 0);
  return row;
}
function stop(name) { owned(name); docker(['stop', '--time', '10', name]); }
function run(role, extra, command = []) {
  const name = names[role];
  docker(['run', '-d', '--name', name, '--label', label, '--network', network,
    '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--memory', role === 'pds' ? '768m' : '192m',
    '--cpus', '1', '--pids-limit', '128', ...extra, role === 'edge' ? edgeImage : image, ...command]);
  created.push(name);
  owned(name);
}
const probe = String.raw`
const fs = require('fs');
const request = JSON.parse(fs.readFileSync(0, 'utf8'));
const timer = setTimeout(() => process.exit(1), 6000);
const req = require('http').request({ hostname: 'edge', port: 8080, path: request.path,
  method: request.method || 'GET', headers: { Host: 'localhost', ...request.headers }
}, response => {
  const chunks = []; let length = 0;
  response.on('data', chunk => { length += chunk.length; if (length > 1024 * 1024) process.exit(1); chunks.push(chunk); });
  response.on('error', () => process.exit(1));
  response.on('end', () => {
    clearTimeout(timer);
    process.stdout.write(JSON.stringify({ status: response.statusCode, body: Buffer.concat(chunks).toString('base64') }));
  });
});
req.on('error', () => process.exit(1));
req.end(request.body === undefined ? undefined : Buffer.from(request.body, 'base64'));`;
function request(path, body, authorization, contentType = 'application/json') {
  const headers = authorization ? { Authorization: authorization } : {};
  if (body != null) headers['Content-Type'] = contentType;
  const bytes = body == null ? undefined : Buffer.isBuffer(body) ? body : Buffer.from(JSON.stringify(body));
  return JSON.parse(docker(['exec', '-i', names.protocol, 'node', '-e', probe], JSON.stringify({
    path, headers, method: body === undefined ? 'GET' : 'POST', ...(bytes === undefined ? {} : { body: bytes.toString('base64') })
  })));
}
function json(response, expected = 200) {
  assert.equal(response.status, expected, 'Disposable protocol status');
  return JSON.parse(Buffer.from(response.body, 'base64'));
}
async function ready() {
  for (let attempt = 0; ; attempt++) {
    assert.ok(owned(names.edge).State.Running, 'Disposable ingress exited during startup');
    assert.ok(owned(names.protocol).State.Running, 'Disposable protocol fixture exited during startup');
    try { json(request('/xrpc/_health')); return; }
    catch (error) { if (attempt >= 29) throw error; await new Promise(resolve => setTimeout(resolve, 250)); }
  }
}
const pdsMounts = ['--mount', `type=volume,src=${volumes.data},dst=/pds`];
const privateKey = generateKeyPairSync('ec', { namedCurve: 'secp256k1' }).privateKey.export({ format: 'jwk' });
const adminPassword = randomBytes(32).toString('hex');
const env = {
  PDS_HOSTNAME: 'localhost', PDS_PORT: '2583', PDS_DEV_MODE: 'true',
  PDS_DATA_DIRECTORY: '/pds', PDS_BLOBSTORE_DISK_LOCATION: '/blobs', PDS_BLOBSTORE_DISK_TMP_LOCATION: '/blobs/tmp',
  PDS_BLOB_UPLOAD_LIMIT: '5242880', PDS_INVITE_REQUIRED: 'true', PDS_RATE_LIMITS_ENABLED: 'true',
  PDS_SERVICE_HANDLE_DOMAINS: '.test', PDS_DID_PLC_URL: 'http://pds-ass-protocol-fixtures:2582',
  PDS_EMAIL_SMTP_URL: 'smtp://protocol-fixtures:2525', PDS_EMAIL_FROM_ADDRESS: 'noreply@fixture.invalid',
  PDS_CRAWLERS: '', LOG_ENABLED: 'false', PDS_ADMIN_PASSWORD: adminPassword,
  PDS_JWT_SECRET: randomBytes(32).toString('hex'),
  PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX: Buffer.from(privateKey.d, 'base64url').toString('hex'),
};
const envPath = `${root}/pds.env`;
writeFileSync(envPath, Object.entries(env).map(([key, value]) => `${key}=${value}`).join('\n') + '\n', { flag: 'wx', mode: 0o600 });
const config = `${root}/Caddyfile`;
writeFileSync(config, '{\n auto_https off\n admin off\n}\n:8080 {\n @allowed host localhost\n handle @allowed {\n reverse_proxy pds:2583\n }\n handle {\n respond "Unknown PDS host" 421\n }\n}\n', { flag: 'wx', mode: 0o600 });
const report = { id, image, completedAt: null, publicIdentity: false, publishedPorts: false, checks: {} };
let finalized = false;
function finalize() {
  if (finalized) return;
  finalized = true;
  const stopFailures = [];
  for (const name of created.toReversed()) {
    try { stop(name); } catch { stopFailures.push(name); }
  }
  if (stopFailures.length) failed = true;
  report.completedAt = new Date().toISOString();
  report.result = failed ? 'FAIL' : 'PASS';
  report.retainedStoppedContainers = created.filter(name => !stopFailures.includes(name));
  report.retainedVolumes = volumes;
  report.stopFailures = stopFailures;
  report.limits = { migrationWindowQualified: false, agentBrokerQualified: false, managementRemovalQualified: false, productionPolicyChanged: false };
  writeFileSync(`${root}/report.json`, JSON.stringify(report, null, 2), { flag: 'wx', mode: 0o600 });
  console.log(`Member-exit qualification ${report.result}: ${root}/report.json`);
}
for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => {
  failed = true;
  finalize();
  process.exit(130);
});
try {
  assert.match(docker(['context', 'inspect', '--format', '{{.Endpoints.docker.Host}}']), /^unix:\/\//, 'Local Docker socket required');
  docker(['network', 'create', '--internal', '--label', label, network]);
  assert.equal(JSON.parse(docker(['network', 'inspect', network]))[0].Internal, true);
  for (const name of Object.values(volumes)) docker(['volume', 'create', '--label', label, name]);
  run('protocol', ['--network-alias', 'protocol-fixtures', '--network-alias', 'pds-ass-protocol-fixtures', '--env', 'PDS_FIXTURE_MODE=local',
    '--mount', `type=bind,src=${resolve('deploy/local/fixtures/protocol-services.mjs')},dst=/fixtures/protocol-services.mjs,readonly`,
    '--mount', `type=volume,src=${volumes.protocol},dst=/fixture-data`], ['node', '/fixtures/protocol-services.mjs']);
  run('pds', ['--network-alias', 'pds', '--env-file', envPath, ...pdsMounts,
    '--mount', `type=volume,src=${volumes.blobs},dst=/blobs`]);
  run('edge', ['--network-alias', 'edge', '--cap-add', 'NET_BIND_SERVICE',
    '--user', `${process.getuid()}:${process.getgid()}`, '--env', 'XDG_CONFIG_HOME=/tmp/config', '--env', 'XDG_DATA_HOME=/tmp/data',
    '--mount', `type=bind,src=${config},dst=/etc/caddy/Caddyfile,readonly`]);
  await ready();
  const admin = `Basic ${Buffer.from(`admin:${adminPassword}`).toString('base64')}`;
  const helper = readFileSync(new URL('../backend/internal/nodeagent/member-exit-helper.cjs', import.meta.url), 'utf8');
  const checks = report.checks;
  function call(method, body, authorization, expected=200) {
    const response=request(`/xrpc/${method}`,body,authorization);if(response.status!==expected)writeFileSync(`${root}/failed-native-response.json`,JSON.stringify({method,status:response.status,body:Buffer.from(response.body,'base64').toString()}),{mode:0o600});assert.equal(response.status,expected,`Disposable ${method} status`);
    const text=Buffer.from(response.body,'base64').toString();return text?JSON.parse(text):undefined;
  }
  const accounts={};
  const secrets=new Set([adminPassword,...Object.values(env).filter(value=>value.length>=32)]);
  for(const name of ['owner','member','preexisting']) {
    const person={handle:`ex-${name.slice(0,3)}-${randomBytes(3).toString('hex')}.test`,email:`${name}@fixture.invalid`,password:randomBytes(32).toString('hex')};
    const invitation=call('com.atproto.server.createInviteCode',{useCount:1},admin);
    report.creatingAccountRole=name;
    const session=call('com.atproto.server.createAccount',{...person,inviteCode:invitation.code});
    accounts[name]={...person,...session};
    for(const value of [person.password,invitation.code,session.accessJwt,session.refreshJwt])secrets.add(value);
  }
  writeFileSync(`${root}/accounts.json`,JSON.stringify(accounts),{flag:'wx',mode:0o600});
  const owner=accounts.owner,member=accounts.member;
  const auth=account=>`Bearer ${account.accessJwt}`;
  function fixed(action,did,expected=0) {
    const result=spawnSync('docker',['exec',names.pds,'node','--no-warnings','-e',helper,action,did],{encoding:'utf8',timeout:15000,maxBuffer:65536});
    assert.equal(result.status,expected,'Fixed member operation status');
    if(expected) {assert.equal(result.stdout,'','Failed helper exposed a result');return;}
    return JSON.parse(result.stdout);
  }
  fixed('deactivate',owner.did,1);fixed('deactivate','did:plc:zzzzzzzzzzzzzzzzzzzzzzzz',1);
  checks.ownerAndUnknownRefused=true;
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL1cAAAAASUVORK5CYII=','base64');
  const uploaded=json(request('/xrpc/com.atproto.repo.uploadBlob',png,auth(member),'image/png')).blob;
  const record={$type:'app.bsky.feed.post',text:'Disposable member exit qualification',createdAt:new Date().toISOString(),embed:{$type:'app.bsky.embed.images',images:[{alt:'Disposable pixel',image:uploaded}]}};
  const post=call('com.atproto.repo.createRecord',{repo:member.did,collection:'app.bsky.feed.post',record},auth(member));
  const snapshotSource=String.raw`const fs=require('fs'),path=require('path'),{createHash}=require('crypto'),{DatabaseSync}=require('node:sqlite');const entries=[];function hash(value){return createHash('sha256').update(value).digest('hex');}function walk(root,relative=''){for(const name of fs.readdirSync(path.join(root,relative)).sort()){const p=path.join(root,relative,name),r=path.join(relative,name),info=fs.lstatSync(p);if(info.isDirectory())walk(root,r);else if(info.isFile()&&!name.endsWith('-shm')&&!name.endsWith('-wal')){let content;if(name==='store.sqlite'){const db=new DatabaseSync(p,{readOnly:true});try{db.exec('BEGIN');content=JSON.stringify(db.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").all().map(({name})=>{if(!/^[a-z_][a-z0-9_]*$/.test(name))throw Error('Unexpected fixture table');return [name,db.prepare('SELECT * FROM "'+name+'"').all().map(row=>JSON.stringify(row,(_,value)=>typeof value==='bigint'?String(value):value)).sort()];}));}finally{db.close();}}else content=fs.readFileSync(p);entries.push([root,r,hash(content)]);}else if(!info.isFile())throw Error('Unexpected fixture file');}}walk('/pds/actors');walk('/blobs');process.stdout.write(JSON.stringify({files:entries.length,sha256:hash(JSON.stringify(entries))}));`;

  function snapshot(){return JSON.parse(docker(['exec',names.pds,'node','-e',snapshotSource]));}
  const actorSource=String.raw`const {DatabaseSync}=require('node:sqlite');const db=new DatabaseSync('/pds/account.sqlite',{readOnly:true});process.stdout.write(JSON.stringify(db.prepare('SELECT did,handle,deactivatedAt,deleteAfter FROM actor ORDER BY createdAt,did').all()));db.close();`;
  function actors(){return JSON.parse(docker(['exec',names.pds,'node','--no-warnings','-e',actorSource]));}
  const before=snapshot(),beforeActors=actors();
  assert.equal(fixed('native_status',member.did).deactivated,false);
  assert.equal(fixed('deactivate',member.did).deactivated,true);
  const after=actors(),memberAfter=after.find(x=>x.did===member.did);
  assert.ok(memberAfter.deactivatedAt);assert.equal(memberAfter.deleteAfter,null);assert.equal(after.length,3);
  assert.deepEqual(after.find(x=>x.did===owner.did),beforeActors.find(x=>x.did===owner.did));
  assert.deepEqual(snapshot(),before,'Repository, blob and signing-key files changed');
  const deactivatedTimestamp=memberAfter.deactivatedAt;
  assert.equal(fixed('deactivate',member.did).deactivated,true);
  assert.equal(actors().find(x=>x.did===member.did).deactivatedAt,deactivatedTimestamp);
  const denied=request('/xrpc/com.atproto.repo.createRecord',{repo:member.did,collection:'app.bsky.feed.post',record},auth(member));
  assert.ok([400,401,403].includes(denied.status),'Deactivated member write must be refused');
  assert.match(Buffer.from(denied.body,'base64').toString(),/deactivat/i,'Native refusal must identify deactivation');
  report.memberWriteStatus=denied.status;
  checks.deactivatedWithoutDeletion=checks.ownerUnchanged=checks.repositoryBlobKeysUnchanged=checks.replayPreservesTimestamp=checks.memberWritesRefused=true;
  const previous=accounts.preexisting;
  call('com.atproto.server.deactivateAccount',{deleteAfter:new Date(Date.now()+365*86400000).toISOString()},auth(previous));
  const priorPolicy=actors().find(x=>x.did===previous.did);assert.ok(priorPolicy.deleteAfter);
  assert.equal(fixed('deactivate',previous.did).deactivated,true);
  assert.deepEqual(actors().find(x=>x.did===previous.did),priorPolicy);
  checks.userDeactivationAndDeletionSchedulePreserved=true;
  const priorIds=Object.fromEntries(created.map(name=>[name,owned(name).Id]));
  docker(['restart',names.pds]);await ready();
  assert.equal(fixed('native_status',member.did).deactivated,true);assert.deepEqual(snapshot(),before);
  for(const name of created)assert.equal(owned(name).Id,priorIds[name]);
  const login=call('com.atproto.server.createSession',{identifier:owner.handle,password:owner.password});assert.equal(login.did,owner.did);
  checks.restartPreservesMemberStateAndOwnerLogin=true;
  report.ownerDID=owner.did;report.memberDID=member.did;report.recordCID=post.cid;report.blobCID=uploaded.ref.$link;report.retainedFileInventory=before;
  for(const name of created){const result=spawnSync('docker',['logs',name],{encoding:'utf8',timeout:10000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Fixture log collection');for(const secret of secrets)if(secret&&secret.length>=6)assert.ok(!(result.stdout+result.stderr).includes(secret),'Credentials appeared in fixture logs');}
  checks.credentialsAbsentFromFixtureLogs=true;
  failed=false;
} catch(error) {
  report.failureKind=error?.name;report.failureLocations=String(error?.stack??'').split('\n').filter(line=>line.includes('qualify-member-exit.mjs')).map(line=>line.match(/qualify-member-exit.mjs:\d+:\d+/)?.[0]).filter(Boolean);
  console.error(error instanceof assert.AssertionError?error.message:'Disposable member deactivation qualification failed');process.exitCode=1;
} finally {finalize();if(failed)process.exitCode=1;}
