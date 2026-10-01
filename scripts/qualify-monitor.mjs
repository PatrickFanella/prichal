// Inject an outage only into the placed local qualification ingress.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
function command(program,args,options={}) { const result=spawnSync(program,args,{encoding:'utf8',timeout:60000,maxBuffer:1024*1024,...options}); assert.equal(result.status,0,'Local monitor qualification subprocess failed'); return result.stdout; }
function docker(...args) { return command('docker',args); }
assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').trim().startsWith('unix:///'));
const database=JSON.parse(docker('inspect','pds-ass-local-db-1'))[0];
assert.equal(database.Config.Labels['com.docker.compose.project'],'pds-ass-local');
const id=docker('exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',"SELECT id FROM deployments WHERE kind='agent' AND state='ready'").trim();
assert.match(id,/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
const name=`pds-ass-edge-${id}`;
const before=JSON.parse(docker('inspect',name))[0];
assert.equal(before.Config.Labels['tv.subcult.pds-ass.instance'],id);
assert.equal(before.Config.Labels['tv.subcult.pds-ass.role'],'ingress');
assert.ok(before.State.Running);
const apiUnavailable=await fetch('http://127.0.0.1:8080/api/readiness',{signal:AbortSignal.timeout(2000)}).then(()=>false,()=>true);
assert.ok(apiUnavailable,'Stop the local management API before this independent monitoring drill');
mkdirSync('.local/monitor',{recursive:true,mode:0o700});
const binary=resolve('.local/monitor/monitor');
command('go',['build','-o',binary,'./cmd/monitor'],{cwd:resolve('backend')});
const environment={...process.env,PDS_MODE:'local',PDS_WORKER_AGENT_CA_FILE:resolve('.local/node-agent/certs/ca.crt'),PDS_WORKER_AGENT_CERT_FILE:resolve('.local/node-agent/certs/worker.crt'),PDS_WORKER_AGENT_KEY_FILE:resolve('.local/node-agent/certs/worker.key')};
function scan() { return JSON.parse(command(binary,[],{env:environment})); }
function runtimeActive(findings) { return findings.find(value=>value.kind==='runtime_unhealthy' && value.deploymentId===id)?.active; }
assert.equal(runtimeActive(scan()),false);
let stopped=false;
try {
 stopped=true;
 docker('stop',name);
 assert.equal(runtimeActive(scan()),true);
 assert.equal(runtimeActive(scan()),true);
 const count=docker('exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-c',`SELECT count(*) FROM service_observations WHERE deployment_id='${id}' AND kind='runtime_unhealthy' AND active`).trim();
 assert.equal(count,'1');
} finally { if(stopped) docker('start',name); }
let recovered=false;
for(let attempt=0;attempt<10;attempt++) { if(runtimeActive(scan())===false) { recovered=true; break; } await new Promise(done=>setTimeout(done,300)); }
assert.ok(recovered,'Restarted local ingress did not recover');
assert.equal(JSON.parse(docker('inspect',name))[0].Id,before.Id);
const report={instance:id,apiUnavailable:true,outageDetected:true,duplicateAlertPrevented:true,recoveryDetected:true,stableContainer:true,completedAt:new Date().toISOString()};
writeFileSync(resolve('.local/monitor',`${randomUUID()}.json`),JSON.stringify(report,null,2)+'\n',{mode:0o600,flag:'wx'});
console.log(JSON.stringify(report));
