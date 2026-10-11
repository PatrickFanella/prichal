// Read-only/public-negative qualification of the one closed Kvant pilot.
import assert from 'node:assert/strict';
import https from 'node:https';
import {randomBytes} from 'node:crypto';
const origin='https://prichal-staging-pds.subcult.tv';
async function call(path,options={}) {
 const response=await fetch(origin+path,{...options,redirect:'manual',signal:AbortSignal.timeout(15000),headers:{'User-Agent':'prichal-qualification/1',...options.headers}});
 const text=await response.text();return{status:response.status,body:text};
}
const health=await call('/xrpc/_health');assert.equal(health.status,200);assert.equal(JSON.parse(health.body).version,'0.4.5036');
const server=await call('/xrpc/com.atproto.server.describeServer');assert.equal(server.status,200);assert.equal(JSON.parse(server.body).did,'did:web:prichal-staging-pds.subcult.tv');
const oauth=await call('/.well-known/oauth-authorization-server');assert.equal(oauth.status,200);const metadata=JSON.parse(oauth.body);assert.equal(metadata.issuer,origin);assert.equal(metadata.authorization_endpoint,origin+'/oauth/authorize');assert.equal(metadata.token_endpoint,origin+'/oauth/token');
const inventory=await call('/xrpc/com.atproto.sync.listRepos?limit=10');assert.equal(inventory.status,200);assert.equal(JSON.parse(inventory.body).repos.length,0);
for(const path of ['/xrpc/com.atproto.server.createAccount','/@atproto/oauth-provider/~api/sign-up']) {
 const denied=await call(path,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});assert.equal(denied.status,403,'Public signup must remain closed');
}
await new Promise((resolve,reject)=>{
 const req=https.request(origin+'/xrpc/com.atproto.sync.subscribeRepos',{headers:{Connection:'Upgrade',Upgrade:'websocket','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':randomBytes(16).toString('base64'),'User-Agent':'prichal-qualification/1'},timeout:10000});
 req.on('upgrade',(response,socket)=>{socket.destroy();response.statusCode===101?resolve():reject(Error('WebSocket upgrade failed'));});req.on('response',response=>{response.resume();reject(Error('WebSocket rejected'));});req.on('error',reject);req.on('timeout',()=>req.destroy(Error('WebSocket timed out')));req.end();
});
console.log(JSON.stringify({origin,health:'0.4.5036',serviceDID:'did:web:prichal-staging-pds.subcult.tv',oauthIssuerExact:true,accounts:0,xrpcSignupClosed:true,oauthSignupClosed:true,publicWebSocket:true}));
