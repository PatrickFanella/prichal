// Synthetic loopback Stripe API shape. No payment processing or connection to Stripe.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFileSync,lstatSync} from 'node:fs';
import {timingSafeEqual} from 'node:crypto';
const root='.local/billing-fixture',path=root+'/api.key';assert.ok(lstatSync(path).isFile()&&!(lstatSync(path).mode&0o077));const key=readFileSync(path,'utf8').trim();
const server=createServer((req,res)=>{res.setHeader('Content-Type','application/json');res.setHeader('Cache-Control','no-store');const authorization=Buffer.from(req.headers.authorization??''),expected=Buffer.from('Bearer '+key);if(authorization.length!==expected.length||!timingSafeEqual(authorization,expected)){res.writeHead(401);res.end('{"error":{"message":"Fixture authentication required."}}');return}
 if(req.method!=='GET'||req.url!=='/v1/subscriptions/sub_fixtureAlice'||req.headers['stripe-version']!=='2026-09-30.endive'){res.writeHead(404);res.end('{"error":{"message":"Fixture request rejected."}}');return}
 try{const info=lstatSync(root+'/subscription.json');assert.ok(info.isFile()&&!(info.mode&0o077)&&info.size<8192);const subscription=JSON.parse(readFileSync(root+'/subscription.json','utf8'));assert.equal(subscription.livemode,false);assert.equal(subscription.id,'sub_fixtureAlice');res.end(JSON.stringify(subscription));}catch{res.writeHead(503);res.end('{"error":{"message":"Fixture state unavailable."}}');}
});server.requestTimeout=5000;server.headersTimeout=5000;server.listen(5560,'127.0.0.1',()=>console.log(JSON.stringify({fixture:true,port:5560,payments:false})));process.on('SIGINT',()=>server.close());process.on('SIGTERM',()=>server.close());
