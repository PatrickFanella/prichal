// Read existing disposable exports only. No capture, target, import or identity mutation.
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {localOwnerClient} from './local-owner-client.mjs';
const id=randomUUID(),root='.local/migration-preflight/'+id;mkdirSync(root,{recursive:true,mode:0o700});
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
function run(args,input){const r=spawnSync('docker',args,{input,encoding:'utf8',timeout:30000,maxBuffer:2*1024*1024});assert.equal(r.status,0,'Local read-only verification failed');return r.stdout.trim();}
function sql(query){return JSON.parse(run(['exec','pds-ass-local-db-1','psql','-U','pds_local','-d','pds_local','-At','-v','ON_ERROR_STOP=1','-c',query]));}
function inventory(){return sql(`SELECT json_build_object('exports',(SELECT json_agg(e ORDER BY id) FROM export_operations e),'events',(SELECT json_agg(e ORDER BY id) FROM export_events e),'jobs',(SELECT count(*) FROM river_job),'deployments',(SELECT json_agg(d ORDER BY id) FROM deployments d),'enrollment',(SELECT json_agg(e ORDER BY id) FROM enrollment_operations e),'accessAudits',(SELECT count(*) FROM organization_access_events),'choices',(SELECT count(*) FROM endpoint_choices))`);}
const deployment={alice:'bca4cb84-cfb2-4e58-afc6-26c5edad9ade',bob:'8d234722-f0ad-47b2-8ef8-10f85d0c13b9'};
const nativeScript=readFileSync('scripts/native-held-export.cjs','utf8'),verifier=readFileSync('scripts/verify-portable-export.cjs','utf8');
function custody(){return Object.values(deployment).map(instance=>({instance,native:JSON.parse(run(['exec','pds-ass-tenant-'+instance,'node','-e',nativeScript])),files:['.json','-tls.pem','-service.json','-account.json','-enrollment/lease.json'].map(suffix=>hash(readFileSync('.local/node-agent/state/'+instance+suffix)))}));}
const before=inventory(),nativeBefore=custody(),clients={alice:await localOwnerClient('alice'),bob:await localOwnerClient('bob')},completed=[];
for(const owner of ['alice','bob']) {
 const listed=await clients[owner].api('/api/deployments/'+deployment[owner]+'/exports');assert.equal(listed.status,200);assert.ok(listed.value.length>0);
 for(const operation of listed.value) {
  assert.equal(operation.downloadAvailable,true,'Retained artifact must remain unexpired; never reset expiry');
  const sealed=hash(readFileSync('.local/exports/'+operation.id+'.sealed'));
  const path='/api/exports/'+operation.id+'/migration-preflight',result=await clients[owner].api(path);assert.equal(result.status,200);
  const r=result.value;assert.equal(r.state,'archive_inventory_only');assert.equal(r.direction,'outbound_from_retained_export');assert.equal(r.canTransfer,false);assert.equal(r.canSwitchIdentity,false);assert.equal(r.signatureVerification,'not_rechecked');assert.equal(r.archiveSha256,operation.sha256);assert.equal(r.archiveBytes,operation.bytes);
  const download=await clients[owner].call('/api/exports/'+operation.id+'/download');assert.equal(download.status,200);const plain=Buffer.from(await download.arrayBuffer());
  const verified=JSON.parse(run(['exec','-i','pds-ass-tenant-'+deployment[owner],'node','--no-warnings','-e',verifier],plain));
  assert.equal(r.did,verified.did);assert.equal(r.archiveVersion,verified.formatVersion);assert.equal(r.capturedRecordCount,verified.signedRecords);assert.equal(r.blobCount,verified.referencedBlobs.length);assert.equal(r.blobBytes,verified.referencedBlobs.reduce((n,b)=>n+b.bytes,0));assert.equal(r.archiveSha256,verified.archiveSha256);assert.equal(r.preferences,verified.formatVersion===2?'captured_app_bsky_only':'not_captured_v1');
  assert.deepEqual((await clients[owner].api(path)).value,r);assert.equal((await clients[owner==='alice'?'bob':'alice'].api(path)).status,404);assert.equal((await clients[owner].call(path,{headers:{Authorization:'Bearer deliberately-invalid'}})).status,403);
  assert.equal(hash(readFileSync('.local/exports/'+operation.id+'.sealed')),sealed);
  completed.push({owner,exportId:operation.id,format:r.archiveVersion,records:r.capturedRecordCount,blobs:r.blobCount,preferences:r.preferences});
 }
}
assert.deepEqual(inventory(),before);assert.deepEqual(custody(),nativeBefore);assert.ok(completed.some(v=>v.format===1)&&completed.some(v=>v.format===2));
const report={id,completedAt:new Date().toISOString(),exports:completed,readOnlyInventoryAndScopePassed:true,independentOfflineSignaturesAndMediaVerified:true,sourceWorkArtifactsAndNativeCustodyUnchanged:true,transferQualified:false,identitySwitchQualified:false};writeFileSync(root+'/report.json',JSON.stringify(report,null,2),{mode:0o600,flag:'wx'});console.log(JSON.stringify(report));
