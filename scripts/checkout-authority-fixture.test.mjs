import test from 'node:test';
import {request as httpRequest} from 'node:http';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,rmSync,chmodSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {createHash,createHmac,randomBytes,randomUUID} from 'node:crypto';
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

async function complete(instance,config,input,headers={}) {
 return fetch(instance.origin+'/_fixture/checkout/completion',{method:'POST',headers:{Authorization:'Bearer '+config.controlKey,'Content-Type':'application/json',...headers},body:JSON.stringify(input),signal:AbortSignal.timeout(5000)});
}
test('separate host-only completion control retains exact events and current graph across restart',async()=>{
 const config={...settings(),controlKey:'fixture_control_'+randomBytes(32).toString('hex'),webhookSecret:'whsec_'+randomBytes(32).toString('hex')};
 let now=1700000000,instance=await start({...config,clock:()=>now});
 try{
  const customer=await (await request(instance,config,'/v1/customers',new URLSearchParams({description:'Managed PDS hosting fixture'}),'pdsaas-customer-'+key)).json();
  const session=await (await request(instance,config,'/v1/checkout/sessions',checkoutForm(customer.id),'pdsaas-checkout-'+key)).json();
  const input={nonce:randomUUID(),sessionId:session.id,type:'checkout.session.completed',paymentState:'unpaid',status:'active'};
  const before=readFileSync(join(config.root,'authority.json'),'utf8');
  for(const headers of [{Authorization:'Bearer '+config.apiKey},{Origin:''},{Cookie:''}])assert.equal((await complete(instance,config,input,headers)).status,401);
  for(const invalid of [{...input,sessionId:'cs_test_foreign'},{...input,type:'checkout.session.async_payment_succeeded',paymentState:'paid'},{...input,owner:'arbitrary'},{...input,paymentState:'unknown'}])assert.equal((await complete(instance,config,invalid)).status,400);
  assert.equal(readFileSync(join(config.root,'authority.json'),'utf8'),before);
  const results=await Promise.all(Array.from({length:8},()=>complete(instance,config,input).then(async response=>{assert.equal(response.status,200);return response.json();})));
  assert.ok(results.every(result=>result.body===results[0].body));const evidence=results[0];assert.equal(evidence.fixture,true);assert.equal(evidence.payments,false);
  assert.equal(evidence.signature,'t='+now+',v1='+createHmac('sha256',config.webhookSecret).update(now+'.'+evidence.body).digest('hex'));
  const event=JSON.parse(evidence.body),subscription=event.data.object.subscription;
  assert.equal(event.data.object.customer,customer.id);assert.equal(event.data.object.client_reference_id,key);
  assert.equal((await complete(instance,config,{...input,status:'past_due'})).status,400);
  await close(instance.server);now+=7200;instance=await start({...config,clock:()=>now});
  const retry=await (await complete(instance,config,input)).json();assert.equal(retry.body,evidence.body);assert.ok(retry.signature.startsWith('t='+now+','));
  const recovered=await (await request(instance,config,'/v1/checkout/sessions/'+session.id+'?expand%5B0%5D=line_items')).json();assert.equal(recovered.status,'complete');assert.equal(recovered.url,null);assert.equal(recovered.expires_at,session.expires_at);
  const graph=await (await request(instance,config,'/v1/subscriptions/'+subscription)).json();assert.equal(graph.customer,customer.id);assert.equal(graph.status,'active');assert.equal(graph.items.data[0].subscription,subscription);assert.equal(graph.items.data[0].price.id,config.priceId);
  assert.equal((await request(instance,config,'/v1/subscriptions/'+subscription+'?expand=x')).status,404);
  assert.equal((await complete(instance,config,{...input,nonce:randomUUID(),type:'checkout.session.async_payment_failed',status:'past_due'})).status,200);
  assert.equal((await (await request(instance,config,'/v1/subscriptions/'+subscription)).json()).status,'past_due');
  const ledger=JSON.parse(readFileSync(join(config.root,'authority.json'),'utf8'));assert.equal(Object.keys(ledger.requests).length,2);assert.equal(Object.keys(ledger.completions).length,2);assert.equal(Object.keys(ledger.subscriptions).length,1);
  assert.throws(()=>createCheckoutAuthority({...config,controlKey:'fixture_control_'+randomBytes(32).toString('hex')}));
  assert.throws(()=>createCheckoutAuthority({...config,webhookSecret:'whsec_'+randomBytes(32).toString('hex')}));
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});
test('completion control defaults closed and cannot enable an existing registry scope',async()=>{
 const config=settings(),instance=await start(config);
 try{
  const response=await fetch(instance.origin+'/_fixture/checkout/completion',{method:'POST',headers:{Authorization:'Bearer '+config.apiKey,'Content-Type':'application/json'},body:'{}'});assert.equal(response.status,404);
  await request(instance,config,'/v1/customers',new URLSearchParams({description:'Managed PDS hosting fixture'}),'pdsaas-customer-'+key);
  assert.throws(()=>createCheckoutAuthority({...config,controlKey:'fixture_control_'+randomBytes(32).toString('hex'),webhookSecret:'whsec_'+randomBytes(32).toString('hex')}));
  assert.throws(()=>createCheckoutAuthority({...config,controlKey:'fixture_control_'+randomBytes(32).toString('hex')}));
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});

test('hosted navigation is a read-only synthetic page without credentials or browser controls',async()=>{
 const config={...settings(),controlKey:'fixture_control_'+randomBytes(32).toString('hex'),webhookSecret:'whsec_'+randomBytes(32).toString('hex')};
 let now=1700000000;const instance=await start({...config,clock:()=>now});
 try{
  const customer=await (await request(instance,config,'/v1/customers',new URLSearchParams({description:'Managed PDS hosting fixture'}),'pdsaas-customer-'+key)).json();
  const session=await (await request(instance,config,'/v1/checkout/sessions',checkoutForm(customer.id),'pdsaas-checkout-'+key)).json();
  const page=async()=>{const response=await fetch(session.url);assert.equal(response.status,200);assert.equal(response.headers.get('content-type'),'text/html; charset=utf-8');assert.equal(response.headers.get('cache-control'),'no-store');assert.ok(response.headers.get('content-security-policy').includes("form-action 'none'"));return response.text();};
  const before=readFileSync(join(config.root,'authority.json'),'utf8');const open=await page();assert.ok(open.includes('Fixture session: open.'));assert.ok(open.includes(config.returnOrigin+'/billing/cancel'));
  for(const sensitive of [config.apiKey,config.controlKey,config.webhookSecret,customer.id,session.id,'<script','<form','<input'])assert.ok(!open.includes(sensitive));
  for(const headers of [{Cookie:''},{Origin:''},{Authorization:'Bearer '+config.apiKey}])assert.equal((await fetch(session.url,{headers})).status,401);
  const wrongHost=await new Promise((resolve,reject)=>{const request=httpRequest(session.url,{headers:{Host:'untrusted.test'}},response=>{response.resume();resolve(response.statusCode);});request.on('error',reject);request.end();});assert.equal(wrongHost,401);
  assert.equal((await fetch(session.url,{method:'POST'})).status,401);
  assert.equal((await fetch(session.url+'?complete=true')).status,401);
  assert.equal(readFileSync(join(config.root,'authority.json'),'utf8'),before);
  now+=7200;assert.ok((await page()).includes('Fixture session: expired.'));
  assert.equal((await complete(instance,config,{nonce:randomUUID(),sessionId:session.id,type:'checkout.session.completed',paymentState:'unpaid',status:'active'})).status,400);
  now-=7200;assert.equal((await complete(instance,config,{nonce:randomUUID(),sessionId:session.id,type:'checkout.session.completed',paymentState:'unpaid',status:'past_due'})).status,200);
  const completedBefore=readFileSync(join(config.root,'authority.json'),'utf8'),completed=await page();assert.ok(completed.includes('Fixture session: complete.'));assert.ok(completed.includes('Fixture payment evidence: unpaid.'));assert.ok(completed.includes(config.returnOrigin+'/billing/return'));assert.equal(readFileSync(join(config.root,'authority.json'),'utf8'),completedBefore);
 }finally{await close(instance.server);rmSync(config.root,{recursive:true,force:true});}
});
