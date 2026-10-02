// Retain the preceding fixed local guard for rollback before changing its embedded policy.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync, lstatSync } from 'node:fs';
const [id,stage='member-admission']=process.argv.slice(2);
assert.match(id??'',/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
assert.match(stage,/^[a-z][a-z-]{2,40}$/,'Upgrade stage name required');
function docker(...args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:1024*1024});assert.equal(result.status,0,'Local guard preparation Docker operation');return result.stdout;}
assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const name=`pds-ass-enrollment-${id}`;
const guard=JSON.parse(docker('inspect',name))[0];
assert.equal(guard.Config.Labels['tv.subcult.pds-ass.instance'],id);
assert.equal(guard.Config.Labels['tv.subcult.pds-ass.role'],'enrollment-guard');
assert.equal(guard.HostConfig.ReadonlyRootfs,true);
assert.ok(guard.Config.Image.startsWith('ghcr.io/bluesky-social/pds:0.4@sha256:'));
const directory=`.local/local-enrollment-guard-upgrades/${id}/${stage}`;
mkdirSync(directory,{recursive:true,mode:0o700});
assert.ok(lstatSync(directory).isDirectory() && !(lstatSync(directory).mode&0o077));
writeFileSync(directory+'/guard.json',JSON.stringify(guard),{mode:0o600,flag:'wx'});
docker('stop',name);
docker('rename',name,`${name}-before-${stage}`);
console.log('Previous local admission guard retained. Reconcile the same instance to start its updated fixed policy.');
