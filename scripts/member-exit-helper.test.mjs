import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
const source=readFileSync(new URL("../backend/internal/nodeagent/member-exit-helper.cjs",import.meta.url),"utf8");
const owner="did:plc:aaaaaaaaaaaaaaaaaaaaaaaa",member="did:plc:bbbbbbbbbbbbbbbbbbbbbbbb";
async function run(args,{local=true,found=true,invited=true,deactivated=false,deleteAfter=null,takedown=false,foreignResponse=false,failWrite=false,lostResponse=false}={}){
 let row={did:member,handle:"member.test",deactivatedAt:deactivated?"retained-time":null,deleteAfter};let stdout="",stderr="",exitCode;const calls=[];
 await runInNewContext(source,{Buffer,AbortSignal,
 process:{argv:["helper",...args],env:{PDS_DEV_MODE:String(local),PDS_DID_PLC_URL:"http://pds-ass-protocol-fixtures:2582",PDS_ADMIN_PASSWORD:"PRIVATE-ADMIN"},stdout:{write:x=>stdout+=x},stderr:{write:x=>stderr+=x},set exitCode(x){exitCode=x;}},
 require(name){assert.equal(name,"node:sqlite");return {DatabaseSync:class{constructor(path,options){assert.equal(path,"/pds/account.sqlite");assert.equal(options.readOnly,true);}exec(q){assert.equal(q,"BEGIN");}prepare(q){if(q.startsWith("SELECT did FROM actor ORDER"))return {get:()=>({did:owner})};if(q.startsWith("SELECT did,handle"))return {get:()=>found?{...row}:undefined};if(q.startsWith("SELECT usedBy"))return {get:()=>invited?{usedBy:member}:undefined};throw Error("Unexpected SQL");}close(){}}};},
 async fetch(url,options){const body=options.body?JSON.parse(options.body):undefined;calls.push({method:options.method,body});if(options.method==="POST"){assert.equal(url,"http://127.0.0.1:2583/xrpc/com.atproto.admin.updateSubjectStatus");assert.deepEqual(body,{subject:{$type:"com.atproto.admin.defs#repoRef",did:member},deactivated:{applied:true}});if(failWrite)return {ok:false};row.deactivatedAt="new-time";if(lostResponse)throw Error("PRIVATE-ADMIN");return {ok:true,text:async()=>"{}"};}return {ok:true,text:async()=>JSON.stringify({subject:{$type:"com.atproto.admin.defs#repoRef",did:foreignResponse?owner:row.did},takedown:{applied:takedown},deactivated:{applied:row.deactivatedAt!==null}})};}
 });assert.ok(!stdout.includes("PRIVATE-ADMIN")&&!stderr.includes("PRIVATE-ADMIN"));return {value:stdout?JSON.parse(stdout):undefined,stderr,exitCode,calls,row};
}
test("Member deactivation refuses owner, unknown/uninvited identities and nonlocal or arbitrary requests",async()=>{
 for(const [args,options] of [[["deactivate",owner],{}],[["deactivate",member],{found:false}],[["deactivate",member],{invited:false}],[["deactivate",member],{local:false}],[["activate",member],{}],[["deactivate",member,"extra"],{}],[["deactivate","invalid"],{}]]){const r=await run(args,options);assert.equal(r.exitCode,1);assert.equal(r.calls.length,0);}
});
test("Native member deactivation is fixed, confirmed and preserves existing user deactivation",async()=>{
 const r=await run(["deactivate",member]);assert.equal(r.value.deactivated,true);assert.equal(r.calls.filter(x=>x.method==="POST").length,1);assert.equal(r.row.deleteAfter,null);
 const repeated=await run(["deactivate",member],{deactivated:true,deleteAfter:"retained-user-deletion-time"});assert.equal(repeated.value.deactivated,true);assert.equal(repeated.calls.filter(x=>x.method==="POST").length,0);assert.equal(repeated.row.deactivatedAt,"retained-time");assert.equal(repeated.row.deleteAfter,"retained-user-deletion-time");
 const read=await run(["native_status",member]);assert.equal(read.value.deactivated,false);assert.equal(read.calls.length,1);
});
test("Native policy conflicts, foreign status and failed/uncertain writes never claim success",async()=>{
 for(const options of [{takedown:true},{deleteAfter:"existing-policy"},{foreignResponse:true},{failWrite:true},{lostResponse:true}]){const r=await run(["deactivate",member],options);assert.equal(r.exitCode,1);assert.equal(r.value,undefined);}
});
