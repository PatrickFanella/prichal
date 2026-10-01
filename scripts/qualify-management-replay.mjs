// Replay a lost checkpoint in an already verified, held local management copy.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { readFileSync, readdirSync, lstatSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
const root=resolve('.');
const directory=resolve('.local/management-recovery');
const candidates=readdirSync(directory).filter(name=>/^[a-f0-9-]{36}$/.test(name)).map(name=>join(directory,name,'report.json')).filter(path=>{ try { return lstatSync(path).isFile(); } catch { return false; } });
candidates.sort((a,b)=>lstatSync(b).mtimeMs-lstatSync(a).mtimeMs);
assert.ok(candidates.length,'Held management recovery report required');
const reportPath=candidates[0];
assert.ok(!(lstatSync(reportPath).mode & 0o077),'Private report required');
const report=JSON.parse(readFileSync(reportPath,'utf8'));
assert.match(report.database,/^pds_recovery_[a-f0-9]{32}$/);
assert.equal(report.state,'held'); assert.equal(report.workerRefused,true);
function command(program,args,options={}) { const result=spawnSync(program,args,{encoding:'utf8',timeout:60000,maxBuffer:1024*1024,...options}); assert.equal(result.status,0,'Management replay subprocess failed'); return result.stdout; }
function docker(...args) { return command('docker',args); }
assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const db=JSON.parse(docker('inspect','pds-ass-local-db-1'))[0];
assert.equal(db.Config.Labels['com.docker.compose.project'],'pds-ass-local');
function sql(database,query) { return docker('exec','pds-ass-local-db-1','psql','-U','pds_local','-d',database,'-At','-v','ON_ERROR_STOP=1','-c',query).trim(); }
assert.equal(sql(report.database,'SELECT state FROM management_recovery'),'held');
const sourceBefore=sql('pds_local',`SELECT encode(sha256(convert_to(string_agg(row_to_json(d)::text,E'\n' ORDER BY id),'UTF8')),'hex') FROM deployments d`);
const placement=JSON.parse(sql(report.database,`SELECT json_build_object('id',p.deployment_id,'node',p.node_id) FROM instance_placements p`));
assert.match(placement.id,/^[a-f0-9-]{36}$/); assert.match(placement.node,/^[a-f0-9-]{36}$/);
const names=[`pds-ass-tenant-${placement.id}`,`pds-ass-edge-${placement.id}`];
const before=JSON.parse(docker('inspect',...names));
assert.ok(before.every(value=>value.State.Running));
const protectedPaths=[join(root,'.local/node-agent/state',placement.id+'.json'),join(root,'.local/node-agent/state',placement.id+'-tls.pem')];
const hashes=protectedPaths.map(path=>createHash('sha256').update(readFileSync(path)).digest('hex'));
// This one fixture predates management placement. It is retained with an explicit
// local-only ledger entry, never treated as a managed deployment or removed.
const retained='aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa';
const old=JSON.parse(docker('inspect',`pds-ass-tenant-${retained}`))[0];
assert.equal(old.Config.Labels['tv.subcult.pds-ass.instance'],retained);
assert.ok(old.Config.Env.includes('PDS_DID_PLC_URL=http://127.0.0.1:9'));
assert.equal(sql(report.database,`SELECT count(*) FROM deployments WHERE id='${retained}'`),'0');
const url=new URL(process.env.PDS_DATABASE_URL);
assert.equal(url.hostname,'127.0.0.1'); assert.equal(url.port,'35433'); assert.equal(url.pathname,'/pds_local');
url.pathname='/'+report.database;
const environment={...process.env,PDS_MODE:'local',PDS_DATABASE_URL:url.toString(),PDS_WORKER_AGENT_CA_FILE:join(root,'.local/node-agent/certs/ca.crt'),PDS_WORKER_AGENT_CERT_FILE:join(root,'.local/node-agent/certs/worker.crt'),PDS_WORKER_AGENT_KEY_FILE:join(root,'.local/node-agent/certs/worker.key'),PDS_RECOVERY_RETAINED_NODE_ID:placement.node,PDS_RECOVERY_RETAINED_INSTANCE_ID:retained,PDS_RECOVERY_REPLAY_INSTANCE_ID:placement.id};
function recover(action) { command('go',['run','./cmd/management-recovery',action],{cwd:resolve('backend'),env:environment}); }
recover('acknowledge-local-fixture');
assert.equal(sql(report.database,'SELECT state FROM management_recovery'),'held');
recover('reconcile');
recover('qualify-replay');
assert.equal(sql(report.database,'SELECT state FROM management_recovery'),'reconciled');
assert.equal(sql(report.database,`SELECT state FROM deployments WHERE id='${placement.id}'`),'ready');
const after=JSON.parse(docker('inspect',...names));
assert.deepEqual(after.map(value=>value.Id),before.map(value=>value.Id),'Replay recreated a runtime');
assert.deepEqual(protectedPaths.map(path=>createHash('sha256').update(readFileSync(path)).digest('hex')),hashes,'Replay changed runtime identity');
assert.equal(sql('pds_local',`SELECT encode(sha256(convert_to(string_agg(row_to_json(d)::text,E'\n' ORDER BY id),'UTF8')),'hex') FROM deployments d`),sourceBefore);
writeFileSync(join(resolve(reportPath,'..'),'replay-report.json'),JSON.stringify({database:report.database,instance:placement.id,retainedFixture:retained,stableContainers:true,stableCredentialsAndTLS:true,sourceDeploymentRowsUnchanged:true,state:'ready',completedAt:new Date().toISOString()},null,2)+'\n',{mode:0o600,flag:'wx'});
console.log(JSON.stringify({database:report.database,state:'ready',stableContainers:true,stableCredentialsAndTLS:true,sourceDeploymentRowsUnchanged:true}));
