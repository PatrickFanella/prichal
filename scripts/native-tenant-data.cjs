// Executed only inside the pinned local tenant image; credentials arrive through stdin.
const assert=require('node:assert/strict');
const {readFileSync}=require('node:fs');
const {createRequire}=require('node:module');
const root=createRequire('/app/package.json'),upstream=createRequire(root.resolve('@atproto/pds'));
const {readCar,verifyRepoCar}=upstream('@atproto/repo');
const {cborDecodeMulti}=upstream('@atproto/common');
async function main(){
 const input=JSON.parse(readFileSync(0,'utf8'));
 const api=async(method,body,auth)=>{const r=await fetch('http://127.0.0.1:2583/xrpc/'+method,{method:body===undefined?'GET':'POST',headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...(auth?{Authorization:auth}:{})},...(body===undefined?{}:{body:JSON.stringify(body)}),signal:AbortSignal.timeout(10000)});assert.equal(r.status,200);return r.json()};
 const session=await api('com.atproto.server.createSession',{identifier:input.account.handle,password:input.account.password});assert.equal(session.did,input.account.did);assert.equal(session.emailConfirmed,true);const auth='Bearer '+session.accessJwt;
 let stored=input.record,frames=[],stream;
 if(input.mode==='populate'){
  const media=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j5ioAAAAASUVORK5CYII=','base64');
  const upload=await fetch('http://127.0.0.1:2583/xrpc/com.atproto.repo.uploadBlob',{method:'POST',headers:{Authorization:auth,'Content-Type':'image/png'},body:media});assert.equal(upload.status,200);const {blob}=await upload.json();
  stream=new WebSocket('ws://127.0.0.1:2583/xrpc/com.atproto.sync.subscribeRepos');stream.binaryType='arraybuffer';
  await new Promise((resolve,reject)=>{const timer=setTimeout(()=>{stream.close();reject(new Error('Stream timeout'))},5000);stream.addEventListener('open',()=>{clearTimeout(timer);resolve()},{once:true});stream.addEventListener('error',()=>{clearTimeout(timer);reject(new Error('Stream unavailable'))},{once:true})});
  stream.addEventListener('message',event=>{if(frames.length<64 && event.data.byteLength<1024*1024)frames.push(Buffer.from(event.data))});
  const record={$type:'app.bsky.feed.post',text:'Local tenant isolation '+input.marker,createdAt:new Date().toISOString(),embed:{$type:'app.bsky.embed.images',images:[{alt:'Disposable fixture',image:blob}]}};
  const written=await api('com.atproto.repo.createRecord',{repo:session.did,collection:'app.bsky.feed.post',record},auth);
  stored={did:session.did,uri:written.uri,cid:written.cid,blobCid:blob.ref.$link};
  await new Promise(resolve=>setTimeout(resolve,1500));stream.close();
 }
 const rkey=stored.uri.split('/').at(-1),record=await api(`com.atproto.repo.getRecord?repo=${session.did}&collection=app.bsky.feed.post&rkey=${rkey}`);assert.equal(record.cid,stored.cid);
 const blob=await fetch(`http://127.0.0.1:2583/xrpc/com.atproto.sync.getBlob?did=${session.did}&cid=${stored.blobCid}`);assert.equal(blob.status,200);const bytes=Buffer.from(await blob.arrayBuffer());assert.equal(bytes.toString('base64'),'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j5ioAAAAASUVORK5CYII=');
 const identity=await (await fetch('http://pds-ass-protocol-fixtures:2582/'+session.did+'/data')).json();
 const exported=await fetch('http://127.0.0.1:2583/xrpc/com.atproto.sync.getRepo?did='+session.did);assert.equal(exported.status,200);const car=Buffer.from(await exported.arrayBuffer());await readCar(car);const repo=await verifyRepoCar(car,session.did,identity.verificationMethods.atproto);assert.ok(repo.creates.some(write=>write.collection==='app.bsky.feed.post'&&write.rkey===rkey));
 const wrong=await upstream('@atproto/crypto').Secp256k1Keypair.create();await assert.rejects(verifyRepoCar(car,session.did,wrong.did()));const tampered=Buffer.from(car);tampered[tampered.length-1]^=1;await assert.rejects(readCar(tampered));
 if(input.mode==='populate'){assert.ok(frames.map(frame=>cborDecodeMulti(frame)).some(([header,body])=>header.t==='#commit'&&body.repo===session.did&&body.ops.some(op=>op.path==='app.bsky.feed.post/'+rkey)))}
 console.log(JSON.stringify({record:stored,signatureVerified:true,mediaVerified:true,streamVerified:input.mode==='populate'}));
}
main().catch(()=>{console.error('Local tenant data qualification failed');process.exitCode=1});
