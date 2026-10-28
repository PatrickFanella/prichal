// Bounded public protocol observations for the one disposable managed staging fixture.
import assert from 'node:assert/strict';
import {createHash,randomBytes} from 'node:crypto';
import {writeFile} from 'node:fs/promises';
import https from 'node:https';

const host='pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv';
const unknown='pds-11111111111141118111111111111111.subcult.tv';
const origin='https://'+host;
assert.equal(process.argv.length,5);
assert.equal(process.argv[2],'--confirm-staging-02');
assert.equal(process.argv[3],'--out');
const tlsReceipts=[];
function call(hostname,path,method='GET'){
 return new Promise((resolve,reject)=>{
  const request=https.request({hostname,path,method,minVersion:'TLSv1.2',timeout:10000,headers:{'Cache-Control':'no-cache','User-Agent':'prichal-managed-public-qualification/1',...(method==='POST'?{'Content-Type':'application/json'}:{})}},response=>{
   let body='';response.setEncoding('utf8');
   const certificate=response.socket.getPeerCertificate();
   assert.equal(response.socket.authorized,true);
   tlsReceipts.push({host:hostname,protocol:response.socket.getProtocol(),fingerprint256:certificate.fingerprint256,validTo:certificate.valid_to});
   response.on('data',chunk=>{body+=chunk;if(body.length>1<<20)request.destroy(Error('Public response exceeded scope'));});
   response.on('end',()=>resolve({status:response.statusCode,body}));
  });
  request.on('error',reject);request.on('timeout',()=>request.destroy(Error('Public request timed out')));request.end(method==='POST'?'{}':undefined);
 });
}
const health=await call(host,'/xrpc/_health');assert.equal(health.status,200);assert.equal(JSON.parse(health.body).version,'0.4.5036');
const describe=await call(host,'/xrpc/com.atproto.server.describeServer');assert.equal(describe.status,200);assert.equal(JSON.parse(describe.body).did,'did:web:'+host);
const oauth=await call(host,'/.well-known/oauth-authorization-server');assert.equal(oauth.status,200);const metadata=JSON.parse(oauth.body);assert.equal(metadata.issuer,origin);assert.equal(metadata.authorization_endpoint,origin+'/oauth/authorize');assert.equal(metadata.token_endpoint,origin+'/oauth/token');
const repos=await call(host,'/xrpc/com.atproto.sync.listRepos?limit=1');assert.equal(repos.status,200);assert.equal(JSON.parse(repos.body).repos.length,0);
for(const path of ['/xrpc/com.atproto.server.createAccount','/@atproto/oauth-provider/~api/sign-up'])assert.equal((await call(host,path,'POST')).status,403);
assert.equal((await call(unknown,'/xrpc/_health')).status,404);
await new Promise((resolve,reject)=>{
 const key=randomBytes(16).toString('base64');
 const request=https.request({hostname:host,path:'/xrpc/com.atproto.sync.subscribeRepos',timeout:10000,minVersion:'TLSv1.2',headers:{Connection:'Upgrade',Upgrade:'websocket','Sec-WebSocket-Version':'13','Sec-WebSocket-Key':key}});
 request.on('upgrade',(response,socket)=>{
  try{assert.equal(socket.authorized,true);assert.equal(response.statusCode,101);assert.equal(response.headers['sec-websocket-accept'],createHash('sha1').update(key+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64'));resolve();}catch(error){reject(error);}finally{socket.destroy();}
 });
 request.on('response',response=>{response.resume();reject(Error('Public WebSocket refused: '+response.statusCode));});request.on('error',reject);request.on('timeout',()=>request.destroy(Error('Public WebSocket timed out')));request.end();
});
const report={observedAt:new Date().toISOString(),origin,health:'0.4.5036',serviceDID:'did:web:'+host,oauthIssuerExact:true,repositories:0,nativeSignupClosed:true,oauthSignupClosed:true,unknownHostStatus:404,publicWebSocket:true,tls:tlsReceipts,managementReadinessPersisted:false};
await writeFile(process.argv[4],JSON.stringify(report,null,2)+'\n',{mode:0o600,flag:'wx'});
console.log(JSON.stringify({origin,publicProtocolPassed:true,managementReadinessPersisted:false}));
