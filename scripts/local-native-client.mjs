// TLS client for disposable UUID.localhost PDS fixtures. No production target support.
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import https from 'node:https';
export function localNativeClient(deployment,endpoint) {
 assert.match(deployment,/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/);
 const base=new URL(endpoint);
 assert.ok(base.protocol==='https:' && base.hostname===deployment+'.localhost' && base.port===String(36000+parseInt(deployment.slice(0,4),16)%10000) && base.pathname==='/' && !base.search && !base.hash && !base.username && !base.password,'Exact local PDS endpoint required');
 const ca=readFileSync(`.local/node-agent/${deployment}-public.crt`);
 return async function native(method,body,authorization,expected=200) {
  assert.match(method,/^com\.atproto\.[A-Za-z0-9.]+(?:\?[A-Za-z0-9._~%=&:-]*)?$/);
  const result=await new Promise((resolve,reject)=>{
   const request=https.request({hostname:'127.0.0.1',port:base.port,servername:base.hostname,ca,path:'/xrpc/'+method,method:body===undefined?'GET':'POST',headers:{Host:base.host,...(body==null?{}:{'Content-Type':'application/json'}),...(authorization?{Authorization:authorization}:{})},timeout:15000,signal:AbortSignal.timeout(15000)},response=>{const chunks=[];let size=0;response.on('error',()=>reject(new Error('Local response interrupted')));response.on('data',chunk=>{size+=chunk.length;if(size>1024*1024)request.destroy(new Error('Local response exceeds bound'));else chunks.push(chunk)});response.on('end',()=>resolve({status:response.statusCode,body:Buffer.concat(chunks).toString()}));});request.on('error',()=>reject(new Error('Local native request unavailable')));request.on('timeout',()=>request.destroy(new Error('Local request timed out')));request.end(body==null?undefined:JSON.stringify(body));
  });
  assert.equal(result.status,expected,'Local native '+method.split('?')[0]+' status');
  try {return result.body?JSON.parse(result.body):undefined;} catch {throw new Error('Local native response is malformed');}
 };
}
