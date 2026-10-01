import assert from 'node:assert/strict';
import test from 'node:test';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
const script=readFileSync('backend/internal/nodeagent/account-helper.cjs','utf8');
const ticket='aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa',owned='bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb',did='did:plc:aaaaaaaaaaaaaaaaaaaaaaaa';
async function fixture({action='apply',state='suspended',applied=false,ref,ownedRef='',deactivated=false,actors=[{did}],fail=false}={}){
 let stdout='',stderr='',writes=0,takedown={applied,...(ref?{ref}:{})};
 const process={argv:['node',action,state,ticket,ownedRef],env:{PDS_DEV_MODE:'true',PDS_DID_PLC_URL:'http://pds-ass-protocol-fixtures:2582',PDS_ADMIN_PASSWORD:'PRIVATE PASSWORD'},stdout:{write:value=>stdout+=value},stderr:{write:value=>stderr+=value},exitCode:0};
 const require=name=>{assert.equal(name,'node:sqlite');return {DatabaseSync:class{constructor(path,options){assert.equal(path,'/pds/account.sqlite');assert.equal(options.readOnly,true)}prepare(sql){assert.equal(sql,'SELECT did FROM actor LIMIT 2');return {all:()=>actors}}close(){}}}};
 const fetch=async(url,options)=>{
  assert.ok(url.startsWith('http://127.0.0.1:2583/xrpc/com.atproto.admin.'));assert.ok(options.headers.Authorization.startsWith('Basic '));
  if(fail)throw new Error('PRIVATE NATIVE RESPONSE');
  if(options.method==='POST'){writes++;const body=JSON.parse(options.body);assert.deepEqual(body.subject,{$type:'com.atproto.admin.defs#repoRef',did});assert.deepEqual(Object.keys(body).sort(),['subject','takedown']);takedown=body.takedown;}
  return {ok:true,text:async()=>JSON.stringify({subject:{$type:'com.atproto.admin.defs#repoRef',did},takedown,deactivated:{applied:deactivated}})};
 };
 await vm.runInNewContext(script,{process,require,fetch,Buffer,AbortSignal});return {stdout,stderr,writes,takedown,exitCode:process.exitCode};
}
test('Native account takedown is scoped to the sole actor and replay-safe',async()=>{
 const first=await fixture();assert.equal(first.exitCode,0);assert.equal(first.writes,1);assert.equal(JSON.parse(first.stdout).appliedRef,ticket);
 const replay=await fixture({applied:true,ref:ticket});assert.equal(replay.exitCode,0);assert.equal(replay.writes,0);
 const replace=await fixture({applied:true,ref:owned,ownedRef:owned});assert.equal(replace.exitCode,0);assert.equal(replace.takedown.ref,ticket);
});
test('Native resume clears only platform takedown and retains user deactivation',async()=>{
 const resumed=await fixture({state:'running',applied:true,ref:owned,ownedRef:owned,deactivated:true});assert.equal(resumed.exitCode,0);assert.equal(resumed.writes,1);const result=JSON.parse(resumed.stdout);assert.equal(result.state,'running');assert.equal(result.deactivated,true);assert.equal(result.appliedRef,'');assert.ok(!resumed.stdout.includes('PASSWORD'));
 for(const state of ['running','suspended']){const outside=await fixture({state,applied:true,ref:'external-moderation'});assert.equal(outside.exitCode,1);assert.equal(outside.writes,0);assert.equal(outside.stdout,'');}
});
test('Native account helper fails closed on missing actors, excess actors and private errors',async()=>{
 for(const options of [{actors:[]},{actors:[{did},{did}]},{fail:true}]){const result=await fixture(options);assert.equal(result.exitCode,1);assert.equal(result.writes,0);assert.equal(result.stdout,'');assert.equal(result.stderr,'Fixed native moderation failed');}
});

test('Native moderation inspection never writes and returns only public status',async()=>{const result=await fixture({action:'native_status',applied:true,ref:owned,deactivated:true});assert.equal(result.exitCode,0);assert.equal(result.writes,0);assert.equal(JSON.parse(result.stdout).deactivated,true);assert.ok(!result.stdout.includes('PASSWORD'))});
