// Runs only inside the designated native container, through its private TLS ingress.
import assert from 'node:assert/strict';
import https from 'node:https';
import {createHash,randomBytes} from 'node:crypto';
const id='2395105c-9958-47b5-ae8b-5b0424cbfc4f';
const host='pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv';
const origin='https://'+host;
const certificate=Buffer.from(process.argv[2]??'','base64');
assert.equal(createHash('sha256').update(certificate).digest('hex'),'851997f4b23c86fe6bd981e525c5f2bb22ec7bb2e9797a26518284c732624cac','Retained ingress certificate required');
function options(path,extra={}){return {hostname:'pds-ass-edge-'+id,port:8443,path,servername:id+'.localhost',ca:certificate,minVersion:'TLSv1.3',headers:{Host:host,'User-Agent':'prichal-managed-qualification/1',...extra.headers},method:extra.method??'GET',timeout:5000};}
function call(path,extra={}){return new Promise((resolve,reject)=>{
 const request=https.request(options(path,extra),response=>{let body='';response.setEncoding('utf8');response.on('data',chunk=>{body+=chunk;if(body.length>1<<20)request.destroy(Error('Native response exceeded scope'));});response.on('end',()=>resolve({status:response.statusCode,body}));});
 request.on('error',reject);request.on('timeout',()=>request.destroy(Error('Private ingress timed out')));request.end(extra.body);
});}
const health=await call('/xrpc/_health');assert.equal(health.status,200);assert.equal(JSON.parse(health.body).version,'0.4.5036');
const server=await call('/xrpc/com.atproto.server.describeServer');assert.equal(server.status,200);assert.equal(JSON.parse(server.body).did,'did:web:'+host);
const oauth=await call('/.well-known/oauth-authorization-server');assert.equal(oauth.status,200);const metadata=JSON.parse(oauth.body);assert.equal(metadata.issuer,origin);assert.equal(metadata.authorization_endpoint,origin+'/oauth/authorize');assert.equal(metadata.token_endpoint,origin+'/oauth/token');
const repos=await call('/xrpc/com.atproto.sync.listRepos?limit=1');assert.equal(repos.status,200);assert.equal(JSON.parse(repos.body).repos.length,0);
for(const path of ['/xrpc/com.atproto.server.createAccount','/@atproto/oauth-provider/~api/sign-up']){const denied=await call(path,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});assert.equal(denied.status,403,'Native account creation remains closed');}
const unknown=await call('/xrpc/_health',{headers:{Host:'unregistered-managed.subcult.tv'}});assert.equal(unknown.status,421);
await new Promise((resolve,reject)=>{
 const request=https.request(options('/xrpc/com.atproto.sync.subscribeRepos',{headers:{Connection:'Upgrade',Upgrade:'websocket','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':randomBytes(16).toString('base64')}}));
 request.on('upgrade',(response,socket)=>{socket.destroy();response.statusCode===101?resolve():reject(Error('Private WebSocket upgrade failed'));});request.on('response',response=>{response.resume();reject(Error('Private WebSocket rejected'));});request.on('error',reject);request.on('timeout',()=>request.destroy(Error('Private WebSocket timed out')));request.end();
});
console.log(JSON.stringify({origin,transport:'private native ingress',health:'0.4.5036',serviceDID:'did:web:'+host,oauthIssuerExact:true,repositories:0,nativeSignupClosed:true,oauthSignupClosed:true,privateUnknownHostRejected:true,privateWebSocket:true,publicReadinessQualified:false}));
