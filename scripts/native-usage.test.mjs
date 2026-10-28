import assert from 'node:assert/strict';
import test from 'node:test';
import * as fs from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import vm from 'node:vm';
const script=fs.readFileSync('backend/internal/nodeagent/usage-helper.cjs','utf8');
async function fixture(change,settings={}){
 const root=fs.mkdtempSync(join(tmpdir(),'pds-usage-test-'));fs.mkdirSync(root+'/pds');fs.mkdirSync(root+'/blobs');fs.writeFileSync(root+'/pds/account.sqlite','private');fs.writeFileSync(root+'/blobs/blob','image');
 let stdout='',stderr='';const process={argv:['node',...(settings.args??[])],env:{PDS_DEV_MODE:'true',...settings.env},stdout:{write:value=>stdout+=value},stderr:{write:value=>stderr+=value},exitCode:0};
 try {
  change?.(root);
  const remap=path=>{assert.ok(path==='/pds'||path.startsWith('/pds/')||path==='/blobs'||path.startsWith('/blobs/'),'Fixed usage path required');return root+path;};
  const nativeFS={readdirSync:path=>fs.readdirSync(remap(path)),lstatSync:path=>fs.lstatSync(remap(path)),statfsSync:path=>fs.statfsSync(remap(path))};
  const require=name=>{if(name==='node:fs')return nativeFS;assert.equal(name,'node:sqlite');return {DatabaseSync:class{constructor(path,options){assert.equal(path,'/pds/account.sqlite');assert.equal(options.readOnly,true)}prepare(sql){assert.equal(sql,'SELECT count(*) AS count FROM actor');return {get:()=>({count:1})}}close(){}}};};
  await vm.runInNewContext(script,{require,process});return {stdout,stderr,exitCode:process.exitCode};
 } finally {fs.rmSync(root,{recursive:true,force:true});}
}
test('Native usage returns counters without file names, identities or private content',async()=>{const result=await fixture();assert.equal(result.exitCode,0);assert.equal(result.stderr,'');const metrics=JSON.parse(result.stdout);assert.equal(metrics.dataBytes,7);assert.equal(metrics.blobBytes,5);assert.equal(metrics.accounts,1);assert.equal(metrics.dataFiles,1);assert.equal(metrics.blobFiles,1);assert.ok(metrics.filesystemAvailableBytes<=metrics.filesystemBytes);assert.ok(!result.stdout.includes('private')&&!result.stdout.includes('account.sqlite'));});
test('Native usage rejects symlinks and excessive directory nesting without exposing paths',async()=>{for(const change of [root=>fs.symlinkSync('/SECRET/outside',root+'/blobs/link'),root=>{let path=root+'/blobs';for(let n=0;n<14;n++){path+='/nested';fs.mkdirSync(path)}}]){const result=await fixture(change);assert.equal(result.exitCode,1);assert.equal(result.stdout,'');assert.equal(result.stderr,'Native usage measurement unavailable');assert.ok(!result.stderr.includes('SECRET'));}});
test('Production usage requires the exact generated host and service identity',async()=>{
 const host='pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv';
 const env={PDS_DEV_MODE:'false',PDS_HOSTNAME:host,PDS_SERVICE_DID:'did:web:'+host};
 const valid=await fixture(undefined,{env,args:['managed',host]});assert.equal(valid.exitCode,0);assert.equal(JSON.parse(valid.stdout).accounts,1);
 for(const settings of [{env},{env,args:['managed','foreign.subcult.tv']},{env:{...env,PDS_SERVICE_DID:'did:web:foreign.subcult.tv'},args:['managed',host]},{env:{...env,PDS_DEV_MODE:'true'},args:['managed',host]}]){
  const result=await fixture(undefined,settings);assert.equal(result.exitCode,1);assert.equal(result.stdout,'');assert.equal(result.stderr,'Native usage measurement unavailable');
 }
});
