// Disposable synthetic billing authority. No connection to Stripe or payment processing.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {createHash,createHmac,randomUUID,timingSafeEqual} from 'node:crypto';
import {existsSync,lstatSync,readFileSync,renameSync,writeFileSync,unlinkSync} from 'node:fs';
import {join} from 'node:path';

const version='2026-09-30.endive',uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const digest=value=>createHash('sha256').update(value).digest('hex');
const label=key=>'pdsaas_fixture_'+[...createHash('sha256').update(key).digest().subarray(0,8)].map(value=>String.fromCharCode(97+value%26)).join('');
const escape=value=>String(value).replace(/[&<>"']/g,character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character]));
function privateFile(path){const stat=lstatSync(path);assert.ok(stat.isFile()&&!(stat.mode&0o077)&&stat.size<=262144,'Private fixture state unavailable');return readFileSync(path,'utf8');}

// createCheckoutAuthority binds a synthetic instance to numeric loopback and private state.
// Callers supply test configuration, never a production account or a browser-selected catalog.
export function createCheckoutAuthority({root,apiKey,priceId,returnOrigin,controlKey,webhookSecret,clock=()=>Math.floor(Date.now()/1000)}) {
 const stat=lstatSync(root);assert.ok(stat.isDirectory()&&!(stat.mode&0o077),'Private fixture directory required');
 assert.match(apiKey,/^rk_test_[A-Za-z0-9]{32,200}$/);assert.match(priceId,/^price_[A-Za-z0-9]{1,100}$/);
 const origin=new URL(returnOrigin);assert.equal(origin.protocol,'http:');assert.equal(origin.hostname,'127.0.0.1');assert.equal(returnOrigin,origin.origin);
 const controls=controlKey!==undefined||webhookSecret!==undefined;
 if(controls){assert.match(controlKey,/^fixture_control_[A-Za-z0-9]{32,200}$/);assert.match(webhookSecret,/^whsec_[A-Za-z0-9]{32,200}$/);}
 const path=join(root,'authority.json'),scope=digest(JSON.stringify({key:apiKey,priceId,returnOrigin,...(controls?{controlKey,webhookSecret}:{})}));
 let ledger={scope,requests:{}};
 if(existsSync(path)){ledger=JSON.parse(privateFile(path));assert.equal(ledger.scope,scope,'Fixture scope changed');assert.ok(ledger.requests&&Object.keys(ledger.requests).length<=100);}
 function persist(next){const temporary=join(root,'.authority-'+randomUUID()+'.json');try{writeFileSync(temporary,JSON.stringify(next),{mode:0o600,flag:'wx'});renameSync(temporary,path);ledger=next;}finally{if(existsSync(temporary))unlinkSync(temporary);}}
 function resource(kind,key,form){
  const requestKey=kind+':'+key,body=digest(form.toString()),prior=ledger.requests[requestKey];
  if(prior){assert.equal(prior.body,body,'Fixture retry body changed');return prior.resource;}
  assert.ok(Object.keys(ledger.requests).length<100,'Fixture capacity exhausted');
  let resource;
  if(kind==='customer')resource={id:'cus_fixture'+digest(key).slice(0,32),object:'customer',livemode:false};
  else resource={id:'cs_test_fixture'+digest(key).slice(0,32),object:'checkout.session',livemode:false,customer:form.get('customer'),client_reference_id:form.get('client_reference_id'),mode:'subscription',ui_mode:'hosted_page',status:'open',expires_at:clock()+3600,success_url:returnOrigin+'/billing/return',cancel_url:returnOrigin+'/billing/cancel',line_items:{object:'list',has_more:false,data:[{object:'item',quantity:1,price:{object:'price',id:priceId,livemode:false}}]}};
  persist({...ledger,requests:{...ledger.requests,[requestKey]:{body,resource}}});return resource;
 }
 function completion(input){
  assert.deepEqual(Object.keys(input).sort(),['nonce','paymentState','sessionId','status','type']);assert.match(input.nonce,uuid);
  assert.ok(['checkout.session.completed','checkout.session.async_payment_succeeded','checkout.session.async_payment_failed'].includes(input.type));
  assert.ok(['paid','unpaid','no_payment_required'].includes(input.paymentState));
  assert.ok(['incomplete','incomplete_expired','trialing','active','past_due','canceled','unpaid','paused'].includes(input.status));
  if(input.type.endsWith('succeeded'))assert.equal(input.paymentState,'paid');
  if(input.type.endsWith('failed'))assert.equal(input.paymentState,'unpaid');
  const body=digest(JSON.stringify(Object.fromEntries(Object.keys(input).sort().map(key=>[key,input[key]])))),prior=ledger.completions?.[input.nonce];
  if(prior){assert.equal(prior.body,body,'Completion retry changed');return prior.event;}
  assert.ok(Object.keys(ledger.completions??{}).length<100,'Completion capacity exhausted');
  const entry=Object.entries(ledger.requests).find(([,row])=>row.resource.object==='checkout.session'&&row.resource.id===input.sessionId);assert.ok(entry,'Session not retained');
  const [key,row]=entry,session=row.resource;
  if(input.type==='checkout.session.completed')assert.equal(session.status,'open');else assert.equal(session.status,'complete');
  if(session.status==='open')assert.ok(session.expires_at>clock(),'Session expired');
  const subscription=session.subscription??'sub_fixture'+digest(session.id).slice(0,32);
  const completed={...session,status:'complete',subscription,payment_status:input.paymentState};
  const graph={id:subscription,object:'subscription',livemode:false,customer:session.customer,status:input.status,items:{object:'list',has_more:false,data:[{id:'si_fixture'+digest(session.id).slice(0,32),object:'subscription_item',subscription,quantity:1,current_period_end:clock()+86400,price:{object:'price',id:priceId,livemode:false}}]}};
  const event={id:'evt_fixture'+digest(input.nonce).slice(0,32),object:'event',api_version:version,livemode:false,created:clock(),type:input.type,data:{object:{id:session.id,object:'checkout.session',livemode:false,customer:session.customer,subscription,client_reference_id:session.client_reference_id,mode:'subscription',ui_mode:'hosted_page',status:'complete'}}};
  persist({...ledger,requests:{...ledger.requests,[key]:{...row,resource:completed}},subscriptions:{...ledger.subscriptions,[subscription]:graph},completions:{...ledger.completions,[input.nonce]:{body,event}}});return event;
 }
 const server=createServer(async(req,res)=>{
  res.setHeader('Content-Type','application/json');res.setHeader('Cache-Control','no-store');res.setHeader('X-Content-Type-Options','nosniff');res.setHeader('Referrer-Policy','no-referrer');
  const reply=(status,body)=>{res.writeHead(status);res.end(JSON.stringify(body));};
  try{
   assert.equal(server.address()?.address,'127.0.0.1','Fixture must bind numeric loopback');
   const url=new URL(req.url,'http://127.0.0.1'),host='http://127.0.0.1:'+server.address().port;
   // Browser navigation reads a synthetic receipt only. It has no control or API credential.
   if(controls&&req.method==='GET'&&/^\/checkout\/cs_test_fixture[a-f0-9]{32}$/.test(url.pathname)&&!url.search){
    if(req.headers.host!=='127.0.0.1:'+server.address().port||Object.hasOwn(req.headers,'origin')||Object.hasOwn(req.headers,'cookie')||Object.hasOwn(req.headers,'authorization')){reply(401,{error:{message:'Fixture navigation rejected.'}});return;}
    const session=Object.values(ledger.requests).find(row=>row.resource.object==='checkout.session'&&row.resource.id===url.pathname.slice(10))?.resource;
    if(!session){reply(404,{error:{message:'Fixture session not found.'}});return;}
    const status=session.status==='open'&&session.expires_at<=clock()?'expired':session.status;
    res.setHeader('Content-Type','text/html; charset=utf-8');res.setHeader('Content-Security-Policy',"default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'");
    res.writeHead(200);res.end('<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Disposable checkout review</title></head><body><main><h1>Disposable checkout review</h1><p>This local fixture processes no payments. Do not enter payment details.</p><p>Fixture session: '+escape(status)+'.</p>'+(status==='complete'?'<p>Fixture payment evidence: '+escape(session.payment_status)+'. This page grants no hosting access.</p>':'<p>Completion requires the separate local test operator control.</p>')+'<p><a href="'+escape(returnOrigin+(status==='complete'?'/billing/return':'/billing/cancel'))+'">Return to management</a></p></main></body></html>');return;
   }
   const control=url.pathname==='/_fixture/checkout/completion';
   const authorization=Buffer.from(req.headers.authorization??''),expected=Buffer.from('Bearer '+(control&&controls?controlKey:apiKey));
   if(authorization.length!==expected.length||!timingSafeEqual(authorization,expected)||Object.hasOwn(req.headers,'origin')||Object.hasOwn(req.headers,'cookie')){reply(401,{error:{message:'Fixture authentication required.'}});return;}
   if(control){
    if(!controls||req.method!=='POST'||url.search||req.headers['content-type']!=='application/json'){reply(404,{error:{message:'Fixture control unavailable.'}});return;}
    let raw='';for await(const chunk of req){raw+=chunk.toString();assert.ok(Buffer.byteLength(raw)<=8192);}
    const event=completion(JSON.parse(raw)),body=JSON.stringify(event),timestamp=clock();
    reply(200,{fixture:true,payments:false,eventId:event.id,body,signature:'t='+timestamp+',v1='+createHmac('sha256',webhookSecret).update(timestamp+'.'+body).digest('hex')});return;
   }
   if(req.headers['stripe-version']!==version){reply(400,{error:{message:'Fixture API version rejected.'}});return;}
   if(req.method==='GET'){
    const subscription=/^\/v1\/subscriptions\/(sub_fixture[a-f0-9]{32})$/.exec(url.pathname);
    if(subscription&&controls&&!url.search&&ledger.subscriptions?.[subscription[1]]){reply(200,ledger.subscriptions[subscription[1]]);return;}
    const match=/^\/v1\/checkout\/sessions\/(cs_test_fixture[a-f0-9]{32})$/.exec(url.pathname);
    if(!match||url.searchParams.toString()!=='expand%5B0%5D=line_items'){reply(404,{error:{message:'Fixture read rejected.'}});return;}
    const session=Object.values(ledger.requests).find(row=>row.resource.id===match[1])?.resource;
    if(!session){reply(404,{error:{message:'Fixture session not found.'}});return;}
    const open=session.status==='open'&&session.expires_at>clock();
    reply(200,{...session,status:session.status==='open'&&!open?'expired':session.status,url:open?host+'/checkout/'+session.id:null});return;
   }
   if(req.method!=='POST'||url.search||!['/v1/customers','/v1/checkout/sessions'].includes(url.pathname)||req.headers['content-type']!=='application/x-www-form-urlencoded'){reply(404,{error:{message:'Fixture request rejected.'}});return;}
   let raw='';for await(const chunk of req){raw+=chunk.toString();assert.ok(Buffer.byteLength(raw)<=8192,'Fixture body too large');}
   const form=new URLSearchParams(raw);assert.equal([...form.keys()].length,new Set(form.keys()).size,'Duplicate fixture field');form.sort();
   const key=req.headers['idempotency-key'];assert.equal(typeof key,'string');
   let result;
   if(url.pathname==='/v1/customers'){
    assert.ok(key.startsWith('pdsaas-customer-')&&uuid.test(key.slice(16)));assert.equal(form.toString(),'description=Managed+PDS+hosting+fixture');result=resource('customer',key,form);
   }else{
    assert.ok(key.startsWith('pdsaas-checkout-')&&uuid.test(key.slice(16)));
    const reference=key.slice(16);assert.ok(Object.values(ledger.requests).some(row=>row.resource.object==='customer'&&row.resource.id===form.get('customer')),'Customer not retained');
    const expectedForm=new URLSearchParams({customer:form.get('customer'),client_reference_id:reference,mode:'subscription',ui_mode:'hosted_page','line_items[0][price]':priceId,'line_items[0][quantity]':'1',success_url:returnOrigin+'/billing/return',cancel_url:returnOrigin+'/billing/cancel',integration_identifier:label(reference)});expectedForm.sort();assert.equal(form.toString(),expectedForm.toString());result=resource('checkout',key,form);
   }
   reply(200,result.object==='checkout.session'?{...result,url:result.status==='open'&&result.expires_at>clock()?host+'/checkout/'+result.id:null}:result);
  }catch{reply(400,{error:{message:'Fixture request or retained state rejected.'}});}
 });
 server.requestTimeout=5000;server.headersTimeout=5000;
 return server;
}
