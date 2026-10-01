// Fixed disposable management origin and local operator exceptions. Never runs checkout.
import assert from 'node:assert/strict';
import {randomUUID,createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {localOwnerClient} from './local-owner-client.mjs';
const id=randomUUID(),work=resolve('.local/pilot-entitlement',id);mkdirSync(work,{recursive:true,mode:0o700});
function command(program,args){const result=spawnSync(program,args,{encoding:'utf8',timeout:30000});assert.equal(result.status,0,'Local qualification command failed');return result.stdout.trim()}
const docker=(...args)=>command('docker',args);
const info=JSON.parse(docker('inspect','pds-ass-local-db-1'))[0];assert.equal(info.Config.Labels['com.docker.compose.project'],'pds-ass-local');assert.ok(docker('context','inspect','--format','{{.Endpoints.docker.Host}}').startsWith('unix:///'));
const sql=query=>docker('exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-v','ON_ERROR_STOP=1','-c',query);
assert.equal(sql('SELECT version FROM schema_version'),'18');
function counts(){return JSON.parse(sql("SELECT json_build_object('deployments',(SELECT count(*) FROM deployments),'events',(SELECT count(*) FROM deployment_events),'deployJobs',(SELECT count(*) FROM river_job WHERE kind='deploy'))"))}
const aliceOrg='da1d3724-993f-443f-a6bf-798c3f8c9f41',bob=JSON.parse(readFileSync('.local/bob-deployment.json','utf8')),bobRequest=JSON.parse(readFileSync('.local/bob-deployment-request.json','utf8'));assert.match(bob.deployment,/^[0-9a-f-]{36}$/);assert.match(bob.organization,/^[0-9a-f-]{36}$/);
const protectedIDs=['bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob.deployment];
function custody(){return protectedIDs.map(instance=>({instance,containers:['tenant','edge','enrollment'].map(role=>docker('inspect','--format','{{.Id}} {{.State.Status}}',`pds-ass-${role}-${instance}`)),files:[instance+'.json',instance+'-tls.pem',instance+'-enrollment/lease.json'].map(file=>createHash('sha256').update(readFileSync(resolve('.local/node-agent/state',file))).digest('hex'))}))}
const before={counts:counts(),custody:custody()};const aliceClient=await localOwnerClient('alice'),bobClient=await localOwnerClient('bob');
async function status(client,org){const result=await client.api(`/api/organizations/${org}/entitlement`);assert.equal(result.status,200);return result.value}
const initial=await status(aliceClient,aliceOrg);assert.equal(initial.state,'unconfigured','Fresh local pilot ledger required');assert.ok(initial.allocatedInstances>=2&&initial.allocatedInstances<3);
assert.equal((await bobClient.api(`/api/organizations/${aliceOrg}/entitlement`)).status,404);
const attempt={kind:'agent',name:'quota-'+id.slice(0,8),region:'local',idempotencyKey:randomUUID()};assert.equal((await aliceClient.api(`/api/organizations/${aliceOrg}/deployments`,attempt)).status,403);
function intent(commandName,org,plan){const args=[commandName,'--organization',org,'--key',randomUUID(),'--reason','qualification'];if(commandName==='grant')args.push('--plan',plan,'--hours','24');const value=JSON.parse(command('./scripts/dev.sh',['entitlement-admin',...args]));return {id:value.id,generation:value.generation,state:value.state,plan:value.plan}}
const grantSingle=intent('grant',aliceOrg,'local-single');const single=await status(aliceClient,aliceOrg);assert.equal(single.instanceLimit,1);assert.equal(single.allocatedInstances,initial.allocatedInstances);assert.equal(single.canDeploy,false);assert.equal((await aliceClient.api(`/api/organizations/${aliceOrg}/deployments`,attempt)).status,409);
const grantSmall=intent('grant',aliceOrg,'local-small');const small=await status(aliceClient,aliceOrg);assert.equal(small.instanceLimit,3);assert.equal(small.availableInstances,3-initial.allocatedInstances);assert.equal(small.canDeploy,true);
const downgrade=intent('grant',aliceOrg,'local-single');const lowered=await status(aliceClient,aliceOrg);assert.equal(lowered.allocatedInstances,initial.allocatedInstances);assert.equal(lowered.canDeploy,false);assert.equal((await aliceClient.api(`/api/organizations/${aliceOrg}/deployments`,attempt)).status,409);
const revoke=intent('revoke',aliceOrg);assert.equal((await status(aliceClient,aliceOrg)).state,'revoked');assert.equal((await aliceClient.api(`/api/organizations/${aliceOrg}/deployments`,attempt)).status,403);
const bobGrant=intent('grant',bob.organization,'local-single');const bobRevoke=intent('revoke',bob.organization);
assert.equal((await status(bobClient,bob.organization)).state,'revoked');
const replay=await bobClient.api(`/api/organizations/${bob.organization}/deployments`,{kind:'agent',name:'isolation-bob',region:'local',idempotencyKey:bobRequest.key});assert.equal(replay.status,200);assert.equal(replay.value.id,bob.deployment);assert.equal(replay.value.state,'ready');
const finalAlice=intent('grant',aliceOrg,'local-small'),finalBob=intent('grant',bob.organization,'local-single');
const after={counts:counts(),custody:custody()};assert.deepEqual(after,before,'Entitlement qualification changed native/durable resources');
const forbidden=await aliceClient.call(`/api/organizations/${aliceOrg}/entitlement`,{method:'POST',headers:{Origin:'http://localhost:5173','Content-Type':'application/json','X-CSRF-Token':aliceClient.session.csrfToken},body:JSON.stringify({plan:'local-small'})});assert.equal(forbidden.status,405);
const report={id,operations:[grantSingle,grantSmall,downgrade,revoke,bobGrant,bobRevoke,finalAlice,finalBob],sourceDurableCountsUnchanged:true,protectedContainersAndCustodyUnchanged:true,foreignOwnerDenied:true,customerGrantMutationDenied:true,deniedWithoutGrant:true,downgradeBlockedGrowth:true,revokedDeploymentReplayRetained:true,paidCheckout:false,completedAt:new Date().toISOString()};writeFileSync(join(work,'report.json'),JSON.stringify(report,null,2),{mode:0o600});console.log(JSON.stringify({id,sourceDurableCountsUnchanged:true,protectedContainersAndCustodyUnchanged:true,foreignOwnerDenied:true,customerGrantMutationDenied:true,downgradeBlockedGrowth:true,revokedDeploymentReplayRetained:true,paidCheckout:false}));
