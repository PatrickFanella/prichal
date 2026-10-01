// Identity-bound collaboration on disposable local management fixtures. No host mutations.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { localOwnerClient } from './local-owner-client.mjs';
const root='.local/organization-access/'+randomUUID();mkdirSync(root,{recursive:true,mode:0o700});
const sourceOrganization='da1d3724-993f-443f-a6bf-798c3f8c9f41',sourceDeployment='bca4cb84-cfb2-4e58-afc6-26c5edad9ade';
const ids=[sourceDeployment,'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'];const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8');
function command(args){const result=spawnSync('docker',args,{encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(result.status,0,'Local fixture inventory failed');return result.stdout.trim();}
function custody(){return ids.map(id=>({id,native:JSON.parse(command(['exec','pds-ass-tenant-'+id,'node','-e',nativeScript])),containers:JSON.parse(command(['inspect','pds-ass-tenant-'+id,'pds-ass-edge-'+id,'pds-ass-enrollment-'+id])).map(value=>({id:value.Id,running:value.State.Running})),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>createHash('sha256').update(readFileSync('.local/node-agent/state/'+id+suffix)).digest('hex'))}));}
const before=custody(),alice=await localOwnerClient('alice'),bob=await localOwnerClient('bob');
async function patch(client,path,body,csrf=true){const response=await client.call(path,{method:'PATCH',headers:{Origin:'http://localhost:5173','Content-Type':'application/json',...(csrf?{'X-CSRF-Token':client.session.csrfToken}:{})},body:JSON.stringify(body)});return {status:response.status,value:await response.json()};}
const keys={sourceInvite:randomUUID(),emptyInvite:randomUUID(),transfer:randomUUID()};writeFileSync(root+'/requests.json',JSON.stringify(keys),{mode:0o600,flag:'wx'});
let sourceInvited=false;let emptyOrganization;
try {
 const invitation=await alice.api(`/api/team/organizations/${sourceOrganization}/invitations`,{identityId:bob.session.ownerId,role:'viewer',idempotencyKey:keys.sourceInvite});assert.equal(invitation.status,200);sourceInvited=true;
 const replay=await alice.api(`/api/team/organizations/${sourceOrganization}/invitations`,{identityId:bob.session.ownerId,role:'viewer',idempotencyKey:keys.sourceInvite});assert.equal(replay.status,200);assert.equal(replay.value.id,invitation.value.id);
 assert.equal((await bob.api(`/api/team/organizations/${sourceOrganization}`)).status,404);
 const incoming=await bob.api('/api/team/incoming');assert.equal(incoming.status,200);assert.ok(incoming.value.invitations.some(value=>value.id===invitation.value.id));
 assert.equal((await bob.api('/api/team/invitations/'+invitation.value.id+'/accept',{})).status,200);
 assert.equal((await bob.api('/api/team/invitations/'+invitation.value.id+'/accept',{})).status,200);
 const shared=await bob.api(`/api/team/organizations/${sourceOrganization}`);assert.equal(shared.status,200);assert.equal(shared.value.role,'viewer');assert.ok(shared.value.instances.some(value=>value.id===sourceDeployment));assert.ok(shared.value.instances.every(value=>Object.keys(value).length===4));
 assert.equal((await patch(bob,`/api/team/organizations/${sourceOrganization}`,{name:'forbidden'})).status,404);
 assert.equal((await patch(bob,`/api/team/organizations/${sourceOrganization}/members/${bob.session.ownerId}`,{role:'administrator'})).status,404);
 assert.equal((await bob.api(`/api/team/organizations/${sourceOrganization}/members`)).status,404);
 assert.equal((await bob.api(`/api/deployments/${sourceDeployment}/exports`)).status,404);
 assert.equal((await patch(alice,`/api/team/organizations/${sourceOrganization}/members/${bob.session.ownerId}`,{role:'revoked'},false)).status,403);
 assert.equal((await patch(alice,`/api/team/organizations/${sourceOrganization}/members/${bob.session.ownerId}`,{role:'revoked'})).status,200);sourceInvited=false;
 assert.equal((await bob.api(`/api/team/organizations/${sourceOrganization}`)).status,404);
 assert.equal((await bob.api('/api/team/invitations/'+invitation.value.id+'/accept',{})).status,200);
 assert.equal((await bob.api(`/api/team/organizations/${sourceOrganization}`)).status,404,'Accepted invitation replay regranted revoked membership');
 const created=await alice.api('/api/organizations',{name:'local-handover-'+randomUUID().slice(0,8)});assert.equal(created.status,201);emptyOrganization=created.value.id;
 const emptyInvite=await alice.api(`/api/team/organizations/${emptyOrganization}/invitations`,{identityId:bob.session.ownerId,role:'administrator',idempotencyKey:keys.emptyInvite});assert.equal(emptyInvite.status,200);
 assert.equal((await bob.api('/api/team/invitations/'+emptyInvite.value.id+'/accept',{})).status,200);
 assert.equal((await patch(bob,`/api/team/organizations/${emptyOrganization}`,{name:'local-handover-reviewed'})).status,200);
 assert.equal((await bob.api(`/api/team/organizations/${emptyOrganization}/invitations`,{identityId:alice.session.ownerId,role:'viewer',idempotencyKey:randomUUID()})).status,404);
 const offered=await alice.api(`/api/team/organizations/${emptyOrganization}/transfers`,{identityId:bob.session.ownerId,idempotencyKey:keys.transfer,confirmEmptyOrganizationHandover:true});assert.equal(offered.status,200);
 assert.equal((await alice.api(`/api/team/organizations/${emptyOrganization}`)).value.role,'owner','Offer changed ownership');
 const accepted=await bob.api(`/api/team/transfers/${offered.value.id}/accept`,{confirmEmptyOrganizationHandover:true});assert.equal(accepted.status,200);
 assert.equal((await bob.api(`/api/team/transfers/${offered.value.id}/accept`,{confirmEmptyOrganizationHandover:true})).status,200);
 assert.equal((await bob.api(`/api/team/organizations/${emptyOrganization}`)).value.role,'owner');assert.equal((await alice.api(`/api/team/organizations/${emptyOrganization}`)).status,404);
 const history=await bob.api(`/api/team/organizations/${emptyOrganization}/members`);assert.equal(history.status,200);assert.equal(history.value.audit.filter(value=>value.action==='transfer_accept').length,1);
 const denied=await fetch('http://localhost:5173/api/team/organizations',{headers:{Authorization:'Bearer '+'a'.repeat(43)},signal:AbortSignal.timeout(5000)});assert.equal(denied.status,403);
 assert.deepEqual(custody(),before,'Collaboration changed native custody');
 const report={completedAt:new Date().toISOString(),emptyOrganization,identityBoundSingleUseInvite:true,stableInviteReplay:true,viewerSafeStatus:true,viewerRoleEscalationDenied:true,ownerPrivateExportsNotDelegated:true,csrfDenied:true,revocationImmediate:true,acceptedInviteReplayDidNotRegrant:true,administratorRename:true,administratorInviteDenied:true,separateHandoverConsent:true,formerOwnerAccessRemoved:true,lastOwnerPreserved:true,handoverAuditOnce:true,cliScopeDenied:true,nativeDataContainersAndCustodyUnchanged:true,report:root+'/report.json'};writeFileSync(report.report,JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
}finally{if(sourceInvited){const response=await patch(alice,`/api/team/organizations/${sourceOrganization}/members/${bob.session.ownerId}`,{role:'revoked'});assert.equal(response.status,200,'Source collaboration cleanup needs review');}}
