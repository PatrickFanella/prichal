// Fixed isolated restore consumer: use only recovered edge trust and loopback 36725.
import assert from 'node:assert/strict';
import https from 'node:https';
import {randomBytes,X509Certificate,createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';
const origin='https://prichal-staging-pds.subcult.tv';
const root='/mnt/fast1/apps/prichal-public-pds-restore-20261012';
const bundle=readFileSync(root+'/extracted/home/onnwee/.local/state/prichal-public-pds-recovery/20261012-operated/external/almaz-ingress.crt','utf8');
// Use the newest captured leaf explicitly when preceding same-key trust also exists.
const candidates=[...bundle.matchAll(/-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/g)].map(match=>({pem:match[0],certificate:new X509Certificate(match[0])}));
assert.ok(candidates.length>0&&candidates.length<=2);
candidates.sort((a,b)=>Date.parse(a.certificate.validFrom)-Date.parse(b.certificate.validFrom));
const selected=candidates.at(-1);const ca=selected.pem;
assert.equal(createHash('sha256').update(selected.certificate.publicKey.export({type:'spki',format:'der'})).digest('hex'),'acc683b7e2fb49fb4596f18d63847c75aae0ce9d4ea62626363343eb793a2d8d');
const transport={hostname:'127.0.0.1',port:36725,servername:'9f148c55-da61-486b-97f4-50df26b65893.localhost',ca};
async function call(path,options={}) {
 return new Promise((resolve,reject)=>{
  const request=https.request({...transport,path,method:options.method||'GET',headers:{Host:'prichal-staging-pds.subcult.tv','User-Agent':'prichal-restore-qualification/1',...options.headers},timeout:15000},response=>{
   let body='';response.on('data',chunk=>{body+=chunk;if(body.length>65536)request.destroy(Error('Oversized response'));});response.on('end',()=>resolve({status:response.statusCode,body}));
  });
  request.on('error',reject);request.on('timeout',()=>request.destroy(Error('Restore request timed out')));request.end(options.body);
 });
}
const health=await call('/xrpc/_health');assert.equal(health.status,200);assert.equal(JSON.parse(health.body).version,'0.4.5036');
const server=await call('/xrpc/com.atproto.server.describeServer');assert.equal(server.status,200);assert.equal(JSON.parse(server.body).did,'did:web:prichal-staging-pds.subcult.tv');
const oauth=await call('/.well-known/oauth-authorization-server');assert.equal(oauth.status,200);const metadata=JSON.parse(oauth.body);assert.equal(metadata.issuer,origin);assert.equal(metadata.authorization_endpoint,origin+'/oauth/authorize');assert.equal(metadata.token_endpoint,origin+'/oauth/token');
const inventory=await call('/xrpc/com.atproto.sync.listRepos?limit=10');assert.equal(inventory.status,200);assert.equal(JSON.parse(inventory.body).repos.length,0);
for(const path of ['/xrpc/com.atproto.server.createAccount','/@atproto/oauth-provider/~api/sign-up']) {
 const denied=await call(path,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});assert.equal(denied.status,403,'Public signup must remain closed');
}
await new Promise((resolve,reject)=>{
 const req=https.request({...transport,path:'/xrpc/com.atproto.sync.subscribeRepos',headers:{Host:'prichal-staging-pds.subcult.tv',Connection:'Upgrade',Upgrade:'websocket','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':randomBytes(16).toString('base64'),'User-Agent':'prichal-qualification/1'},timeout:10000});
 req.on('upgrade',(response,socket)=>{socket.destroy();response.statusCode===101?resolve():reject(Error('WebSocket upgrade failed'));});req.on('response',response=>{response.resume();reject(Error('WebSocket rejected'));});req.on('error',reject);req.on('timeout',()=>req.destroy(Error('WebSocket timed out')));req.end();
});
console.log(JSON.stringify({origin,health:'0.4.5036',serviceDID:'did:web:prichal-staging-pds.subcult.tv',oauthIssuerExact:true,repositories:0,xrpcSignupClosed:true,oauthSignupClosed:true,privateWebSocket:true,isolatedRestore:true}));
