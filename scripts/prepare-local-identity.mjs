// Enable isolated identity fixtures only on an empty, stopped qualification runtime.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync, renameSync, lstatSync, readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
const [id]=process.argv.slice(2);
assert.match(id??'',/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
function docker(...args) { const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:1024*1024}); assert.equal(result.status,0,'Local identity preparation Docker operation'); return result.stdout; }
assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const tenant=`pds-ass-tenant-${id}`,edge=`pds-ass-edge-${id}`;
const snapshots=JSON.parse(docker('inspect',tenant,edge));
for(const container of snapshots) assert.equal(container.Config.Labels['tv.subcult.pds-ass.instance'],id);
const runtime=snapshots.find(container=>container.Name==='/'+tenant);
assert.ok(runtime.Config.Env.includes('PDS_DEV_MODE=true') && runtime.Config.Env.includes('PDS_DID_PLC_URL=http://127.0.0.1:9') && runtime.Config.Env.includes('PDS_INVITE_REQUIRED=true'));
const count=docker('exec',tenant,'node','-e',"const{DatabaseSync}=require('node:sqlite');const db=new DatabaseSync('/pds/account.sqlite',{readOnly:true});process.stdout.write(String(db.prepare('SELECT count(*) AS count FROM actor').get().count))");
assert.equal(count,'0','Populated PDS identity cannot use empty-fixture preparation');
const directory=`.local/local-identity-upgrades/${id}`;
mkdirSync(directory,{recursive:true,mode:0o700});
assert.ok(lstatSync(directory).isDirectory() && !(lstatSync(directory).mode&0o077));
writeFileSync(`${directory}/containers.json`,JSON.stringify(snapshots),{mode:0o600,flag:'wx'});
const tlsPath=`.local/node-agent/state/${id}-tls.pem`;
writeFileSync(`${directory}/identity-proof.json`,JSON.stringify({endpoint:runtime.Config.Env.find(value=>value.startsWith('PDS_HOSTNAME=')),tlsSha256:createHash('sha256').update(readFileSync(tlsPath)).digest('hex')}),{mode:0o600,flag:'wx'});
docker('stop',edge,tenant);
docker('rename',edge,edge+'-before-local-identity');
docker('rename',tenant,tenant+'-before-local-identity');
const config=`.local/node-agent/state/${id}-edge/Caddyfile`;
renameSync(config,config+'.before-local-identity');
console.log('Empty fixture archived with volumes, credentials, endpoint and TLS identity retained. Reconcile with isolated local identity mode.');
