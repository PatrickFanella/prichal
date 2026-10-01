import assert from 'node:assert/strict';
import test from 'node:test';
import * as fs from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import vm from 'node:vm';
const script=fs.readFileSync('backend/internal/nodeagent/usage-helper.cjs','utf8');
async function fixture(change){
 const root=fs.mkdtempSync(join(tmpdir(),'pds-usage-test-'));fs.mkdirSync(root+'/pds');fs.mkdirSync(root+'/blobs');fs.writeFileSync(root+'/pds/account.sqlite','private');fs.writeFileSync(root+'/blobs/blob','image');
 let stdout='',stderr='';const process={env:{PDS_DEV_MODE:'true'},stdout:{write:value=>stdout+=value},stderr:{write:value=>stderr+=value},exitCode:0};
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
