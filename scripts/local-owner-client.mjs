// Disposable local OIDC fixture client. Never use it for a live management origin.
import assert from 'node:assert/strict';
const origin='http://localhost:5173';
const issuer='http://localhost:5556';
export async function localOwnerClient(owner='alice') {
 assert.ok(['alice','bob'].includes(owner),'Known disposable owner required');
 const cookies=new Map();
 async function call(url,options={}) {
  const target=new URL(url,origin);assert.ok([origin,issuer].includes(target.origin),'Local fixture origin required');
  const selected=[...cookies.values()].filter(value=>value.hostname===target.hostname && target.pathname.startsWith(value.path)).map(value=>`${value.name}=${value.value}`).join('; ');
  const response=await fetch(target,{...options,redirect:'manual',headers:{...(selected?{Cookie:selected}:{}),...options.headers},signal:AbortSignal.timeout(15000)});
  for(const header of response.headers.getSetCookie()) {
   const parts=header.split(';').map(value=>value.trim()),boundary=parts[0].indexOf('=');
   const name=parts[0].slice(0,boundary),value=parts[0].slice(boundary+1),path=parts.find(value=>value.toLowerCase().startsWith('path='))?.slice(5)??'/';
   cookies.set(`${target.hostname}/${path}/${name}`,{name,value,path,hostname:target.hostname});
  }
  return response;
 }
 let response=await call('/api/auth/login');assert.equal(response.status,303,'Local owner sign-in failed');
 const authorize=response.headers.get('Location');assert.ok(authorize?.startsWith(issuer+'/'),'Unexpected local authorization origin');
 response=await call(authorize);assert.equal(response.status,200,'Local authorization form failed');
 const form=await response.text(),token=/name="request" value="([^"]+)"/.exec(form)?.[1];assert.ok(token,'Local authorization request missing');
 response=await call(issuer+'/approve',{method:'POST',headers:{Origin:issuer,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({request:token,owner}).toString()});assert.equal(response.status,303,'Local owner approval failed');
 const callback=response.headers.get('Location');assert.ok(callback?.startsWith(origin+'/api/auth/callback?'),'Unexpected management callback');
 response=await call(callback);assert.equal(response.status,303,'Local owner callback failed');
 response=await call('/api/session');assert.equal(response.status,200,'Local owner session missing');
 const session=await response.json();assert.ok(typeof session.ownerId==='string' && typeof session.csrfToken==='string','Local owner session invalid');
 async function api(path,body) {
  const response=await call(path,body===undefined?{}:{method:'POST',headers:{Origin:origin,'Content-Type':'application/json','X-CSRF-Token':session.csrfToken},body:JSON.stringify(body)});
  const value=await response.json();return {status:response.status,value};
 }
 return {api,session,call};
}
