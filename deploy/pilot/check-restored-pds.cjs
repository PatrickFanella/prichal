// Runs only inside the disconnected restore container; secrets arrive via stdin.
const fs=require('node:fs'),assert=require('node:assert/strict'),crypto=require('node:crypto');
assert.equal(process.env.PDS_DEV_MODE,'false');
assert.equal(process.env.PDS_HOSTNAME,'prichal-pds.subcult.tv');
assert.ok(Object.values(require('node:os').networkInterfaces()).flat().every(address=>address.internal),'Disconnected restore network required');
const input=JSON.parse(fs.readFileSync(0,'utf8'));assert.equal(input.did,'did:plc:gh4owm5uqgahod5wzoxqebhk');const base='http://127.0.0.1:2583/xrpc/';
async function request(method,body,token,raw=false){const r=await fetch(base+method,{method:body===undefined?'GET':'POST',headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...(token?{Authorization:'Bearer '+token}:{})},...(body===undefined?{}:{body:JSON.stringify(body)}),signal:AbortSignal.timeout(10000)});assert.equal(r.status,200,'Disconnected native request status');return raw?Buffer.from(await r.arrayBuffer()):r.json();}
(async()=>{
for(let i=0;;i++){try{await request('_health');break;}catch{if(i===29)throw Error('Disconnected native health unavailable');await new Promise(r=>setTimeout(r,200));}}
const session=await request('com.atproto.server.createSession',{identifier:input.account.email,password:input.account.password});
assert.equal(session.did,input.did);assert.equal(session.handle,'prichal-test.subcult.tv');assert.equal(session.emailConfirmed,true);
const found=await request('com.atproto.repo.getRecord?'+new URLSearchParams({repo:input.did,collection:'app.bsky.feed.post',rkey:input.rkey}));assert.equal(found.cid,input.cid);assert.deepEqual(found.value,input.record);
const blob=await request('com.atproto.sync.getBlob?'+new URLSearchParams({did:input.did,cid:input.blobCid}),undefined,undefined,true);assert.equal(crypto.createHash('sha256').update(blob).digest('hex'),input.blobSha256);
const car=await request('com.atproto.sync.getRepo?'+new URLSearchParams({did:input.did}),undefined,undefined,true);assert.equal(crypto.createHash('sha256').update(car).digest('hex'),input.carSha256);
const inventory=await request('com.atproto.sync.listRepos?limit=100');assert.equal(inventory.repos.length,1);assert.equal(inventory.repos[0].did,input.did);
const {createRequire}=require('node:module');const root=createRequire('/app/package.json');const u=createRequire(root.resolve('@atproto/pds'));const repo=await u('@atproto/repo').verifyRepoCar(car,input.did,input.key);assert.ok(repo.creates.some(w=>w.collection==='app.bsky.feed.post'&&w.rkey===input.rkey));
console.log(JSON.stringify({nativeLoginRestored:true,emailConfirmationRestored:true,handleRestored:true,recordValueCidRestored:true,blobBytesHashRestored:true,carHashRestored:true,repositorySignatureVerified:true,exactlyOneNativeRepository:true}));
})().catch(()=>{console.error('Disconnected populated restore qualification failed');process.exitCode=1;});
