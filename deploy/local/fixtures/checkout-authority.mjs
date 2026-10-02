// Disposable synthetic billing authority. No connection to Stripe or payment processing.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {createHash,randomUUID,timingSafeEqual} from 'node:crypto';
import {existsSync,lstatSync,readFileSync,renameSync,writeFileSync,unlinkSync} from 'node:fs';
import {join} from 'node:path';

const version='2026-09-30.endive',uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const digest=value=>createHash('sha256').update(value).digest('hex');
const label=key=>'pdsaas_fixture_'+[...createHash('sha256').update(key).digest().subarray(0,8)].map(value=>String.fromCharCode(97+value%26)).join('');
function privateFile(path){const stat=lstatSync(path);assert.ok(stat.isFile()&&!(stat.mode&0o077)&&stat.size<=262144,'Private fixture state unavailable');return readFileSync(path,'utf8');}

// createCheckoutAuthority binds a synthetic instance to numeric loopback and private state.
// Callers supply test configuration, never a production account or a browser-selected catalog.
export function createCheckoutAuthority({root,apiKey,priceId,returnOrigin,clock=()=>Math.floor(Date.now()/1000)}) {
 const stat=lstatSync(root);assert.ok(stat.isDirectory()&&!(stat.mode&0o077),'Private fixture directory required');
 assert.match(apiKey,/^rk_test_[A-Za-z0-9]{32,200}$/);assert.match(priceId,/^price_[A-Za-z0-9]{1,100}$/);
 const origin=new URL(returnOrigin);assert.equal(origin.protocol,'http:');assert.equal(origin.hostname,'127.0.0.1');assert.equal(returnOrigin,origin.origin);
 const path=join(root,'authority.json'),scope=digest(JSON.stringify({key:apiKey,priceId,returnOrigin}));
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
 const server=createServer(async(req,res)=>{
  res.setHeader('Content-Type','application/json');res.setHeader('Cache-Control','no-store');res.setHeader('X-Content-Type-Options','nosniff');res.setHeader('Referrer-Policy','no-referrer');
  const reply=(status,body)=>{res.writeHead(status);res.end(JSON.stringify(body));};
  try{
   assert.equal(server.address()?.address,'127.0.0.1','Fixture must bind numeric loopback');
   const authorization=Buffer.from(req.headers.authorization??''),expected=Buffer.from('Bearer '+apiKey);
   if(authorization.length!==expected.length||!timingSafeEqual(authorization,expected)||req.headers.origin||req.headers.cookie){reply(401,{error:{message:'Fixture authentication required.'}});return;}
   if(req.headers['stripe-version']!==version){reply(400,{error:{message:'Fixture API version rejected.'}});return;}
   const url=new URL(req.url,'http://127.0.0.1'),host='http://127.0.0.1:'+server.address().port;
   if(req.method==='GET'){
    const match=/^\/v1\/checkout\/sessions\/(cs_test_fixture[a-f0-9]{32})$/.exec(url.pathname);
    if(!match||url.searchParams.toString()!=='expand%5B0%5D=line_items'){reply(404,{error:{message:'Fixture read rejected.'}});return;}
    const session=Object.values(ledger.requests).find(row=>row.resource.id===match[1])?.resource;
    if(!session){reply(404,{error:{message:'Fixture session not found.'}});return;}
    reply(200,{...session,status:session.expires_at<=clock()?'expired':session.status,url:session.expires_at>clock()?host+'/checkout/'+session.id:null});return;
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
   reply(200,result.object==='checkout.session'?{...result,url:host+'/checkout/'+result.id}:result);
  }catch{reply(400,{error:{message:'Fixture request or retained state rejected.'}});}
 });
 server.requestTimeout=5000;server.headersTimeout=5000;
 return server;
}
