import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,rmSync,chmodSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {createHash,randomBytes} from 'node:crypto';
import {createCheckoutAuthority} from '../deploy/local/fixtures/checkout-authority.mjs';
const key='eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee';
const version='2026-09-30.endive';
function settings(){return {root:mkdtempSync(join(tmpdir(),'pds-checkout-fixture-')),apiKey:'rk_test_'+randomBytes(32).toString('hex'),priceId:'price_fixtureSessions',returnOrigin:'http://127.0.0.1:5173'};}
async function start(config){const server=createCheckoutAuthority(config);server.listen(0,'127.0.0.1');await once(server,'listening');return {server,origin:'http://127.0.0.1:'+server.address().port};}
async function close(server){await new Promise((resolve,reject)=>server.close(error=>error?reject(error):resolve()));}
async function request(instance,config,path,form,idempotency,headers={}){return fetch(instance.origin+path,{method:form?'POST':'GET',headers:{Authorization:'Bearer '+config.apiKey,'Stripe-Version':version,...(form?{'Content-Type':'application/x-www-form-urlencoded','Idempotency-Key':idempotency}:{}),...headers},body:form?.toString(),signal:AbortSignal.timeout(5000)});}
function checkoutForm(customer){return new URLSearchParams({customer,client_reference_id:key,mode:'subscription',ui_mode:'hosted_page','line_items[0][price]':'price_fixtureSessions','line_items[0][quantity]':'1',success_url:'http://127.0.0.1:5173/billing/return',cancel_url:'http://127.0.0.1:5173/billing/cancel',integration_identifier:'pdsaas_fixture_'+[...createHash('sha256').update(key).digest().subarray(0,8)].map(value=>String.fromCharCode(97+value%26)).join('')});}

test('synthetic customer and checkout retain exact identity across retries and fixture restart',async()=>{
 const config=settings();let instance=await start(config);
 try{
  const form=new URLSearchParams({description:'Managed PDS hosting fixture'});
  const customers=await Promise.all(Array.from({length:8},()=>request(instance,config,'/v1/customers',form,'pdsaas-customer-'+key).then(async response=>{assert.equal(response.status,200);return response.json();})));
  assert.ok(customers.every(customer=>customer.id===customers[0].id));
  const customer=customers[0],body=checkoutForm(customer.id);
  const sessions=await Promise.all(Array.from({length:8},()=>request(instance,config,'/v1/checkout/sessions',body,'pdsaas-checkout-'+key).then(async response=>{assert.equal(response.status,200);return response.json();})));
  assert.ok(sessions.every(session=>session.id===sessions[0].id&&session.expires_at===sessions[0].expires_at));
  const session=sessions[0];assert.equal(session.customer,customer.id);assert.equal(session.client_reference_id,key);
  const before=JSON.parse(readFileSync(join(config.root,'authority.json'),'utf8'));assert.equal(Object.keys(before.requests).length,2);assert.ok(!JSON.stringify(before).includes('http://127.0.0.1:')||!JSON.stringify(before).includes('/checkout/'));
  await close(instance.server);instance=await start(config);
  const again=await (await request(instance,config,'/v1/checkout/sessions',body,'pdsaas-checkout-'+key)).json();assert.equal(again.id,session.id);assert.equal(again.expires_at,session.expires_at);
  const retrieved=await (await request(instance,config,'/v1/checkout/sessions/'+session.id+'?expand%5B0%5D=line_items')).json();assert.equal(retrieved.id,session.id);assert.equal(retrieved.line_items.data[0].price.id,config.priceId);
  assert.deepEqual(JSON.parse(readFileSync(join(config.root,'authority.json'),'utf8')),before);
  body.set('line_items[0][quantity]','2');assert.equal((await request(instance,config,'/v1/checkout/sessions',body,'pdsaas-checkout-'+key)).status,400);
  assert.deepEqual(JSON.parse(readFileSync(join(config.root,'authority.json'),'utf8')),before);
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});

test('fixture refuses browser credentials, wrong version, raw content and unretained customers',async()=>{
 const config=settings(),instance=await start(config);
 try{
  const form=new URLSearchParams({description:'Managed PDS hosting fixture'});
  for(const headers of [{Authorization:'Bearer wrong'},{Origin:'http://127.0.0.1:5173'},{Cookie:'private=marker'},{'Stripe-Version':'wrong'},{'Content-Type':'text/plain'}])assert.notEqual((await request(instance,config,'/v1/customers',form,'pdsaas-customer-'+key,headers)).status,200);
  for(const body of ['description=Managed+PDS+hosting+fixture&email=private%40test','description=Managed+PDS+hosting+fixture&description=other','x='+ 'x'.repeat(8193)])assert.equal((await request(instance,config,'/v1/customers',new URLSearchParams(body),'pdsaas-customer-'+key)).status,400);
  assert.equal((await request(instance,config,'/v1/customers',form,'new-replacement-key')).status,400);
  assert.equal((await request(instance,config,'/v1/checkout/sessions',checkoutForm('cus_foreign'),'pdsaas-checkout-'+key)).status,400);
  assert.equal((await request(instance,config,'/v1/checkout/sessions/cs_test_other?expand%5B0%5D=line_items')).status,404);
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});

test('private fixture scope and expiration cannot silently create replacement resources',async()=>{
 const config=settings();let now=1700000000;const instance=await start({...config,clock:()=>now});
 try{
  const customer=await (await request(instance,config,'/v1/customers',new URLSearchParams({description:'Managed PDS hosting fixture'}),'pdsaas-customer-'+key)).json();
  const body=checkoutForm(customer.id),session=await (await request(instance,config,'/v1/checkout/sessions',body,'pdsaas-checkout-'+key)).json();
  const before=readFileSync(join(config.root,'authority.json'),'utf8');now+=7200;
  const expired=await (await request(instance,config,'/v1/checkout/sessions/'+session.id+'?expand%5B0%5D=line_items')).json();assert.equal(expired.status,'expired');assert.equal(expired.url,null);
  const retry=await (await request(instance,config,'/v1/checkout/sessions',body,'pdsaas-checkout-'+key)).json();assert.equal(retry.expires_at,session.expires_at);assert.equal(readFileSync(join(config.root,'authority.json'),'utf8'),before);
  assert.throws(()=>createCheckoutAuthority({...config,priceId:'price_other'}));
  assert.throws(()=>createCheckoutAuthority({...config,apiKey:'rk_test_'+randomBytes(32).toString('hex')}));
  chmodSync(join(config.root,'authority.json'),0o644);assert.throws(()=>createCheckoutAuthority(config));
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});
