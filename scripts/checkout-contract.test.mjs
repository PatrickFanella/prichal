import test from 'node:test';
import assert from 'node:assert/strict';
import {checkoutIntent,checkoutPlan,checkoutReceipt,checkoutNavigation} from '../web/src/checkout-contract.ts';
const intent={key:'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',priceId:'price_fixtureSessions'};
const org='aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa';
const receipt={id:'bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb',organizationId:org,idempotencyKey:intent.key,priceId:intent.priceId,state:'prepared',createdAt:'2026-10-01T00:00:00Z',canPay:false};
test('checkout receipt and retained intent require exact scope and no payment authority',()=>{
 assert.ok(checkoutIntent(intent));assert.ok(checkoutReceipt(receipt,org,intent));
 for(const value of [null,[],{...intent,extra:true},{...intent,key:'invalid'},{...intent,priceId:'injected/path'}])assert.equal(checkoutIntent(value),false);
 for(const change of [{organizationId:'other'},{idempotencyKey:'foreign'},{priceId:'price_other'},{canPay:true},{state:'paid'},{createdAt:'invalid'},{id:'invalid'},{url:'http://127.0.0.1/private'}])assert.equal(checkoutReceipt({...receipt,...change},org,intent),false);
 assert.ok(checkoutPlan({priceId:intent.priceId,fixture:true,canPrepare:true}));
 assert.equal(checkoutPlan({priceId:intent.priceId,fixture:false,canPrepare:true}),false);
});
test('checkout navigation is transient loopback-only and absent for closed sessions',()=>{
 const value={status:'open',expiresAt:new Date(Date.now()+3600000).toISOString(),url:'http://127.0.0.1:5560/checkout/cs_test_fixture'};
 assert.ok(checkoutNavigation(value));
 for(const url of ['https://foreign.test/private','http://localhost:5560/checkout/cs_test_fixture','http://127.0.0.1:5560/checkout/cs_test_fixture?secret=x','http://127.0.0.1:5560/checkout/cs_test_fixture?','http://user@127.0.0.1:5560/checkout/cs_test_fixture','http://127.0.0.1:5560/checkout/cs_test_fixture#secret','http://127.0.0.1:5560/checkout/%63s_test_fixture'])assert.equal(checkoutNavigation({...value,url}),false);
 for(const status of ['complete','expired']){
  assert.ok(checkoutNavigation({status,expiresAt:value.expiresAt}));
  assert.equal(checkoutNavigation({...value,status}),false);
 }
 assert.equal(checkoutNavigation({...value,expiresAt:new Date(Date.now()-1).toISOString()}),false);
 assert.equal(checkoutNavigation({...value,expiresAt:new Date(Date.now()+26*3600000).toISOString()}),false);
});
