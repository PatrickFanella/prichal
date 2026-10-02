import test from 'node:test';
import assert from 'node:assert/strict';
import {operatorIntent,operatorOperation,operatorReceipt} from '../web/src/operator-lifecycle-contract.ts';
const intent={key:'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',instance:'aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa',kind:'service',state:'suspended',reason:'maintenance'};
const receipt={id:'bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb',deploymentId:intent.instance,idempotencyKey:intent.key,generation:2,kind:intent.kind,desiredState:intent.state,priorState:'running',reason:intent.reason,state:'pending',updatedAt:'2026-10-01T00:00:00Z'};
test('operator receipt must match every original intent field and bounded generation',()=>{
 assert.ok(operatorIntent(intent));assert.ok(operatorReceipt(receipt,intent));
 for(const change of [{idempotencyKey:'cccccccc-cccc-4ccc-8ccc-cccccccccccc'},{deploymentId:'cccccccc-cccc-4ccc-8ccc-cccccccccccc'},{kind:'account'},{desiredState:'running'},{reason:'safety_review'},{generation:Number.MAX_SAFE_INTEGER+1},{generation:0},{state:'unknown'},{updatedAt:'invalid'}])assert.equal(operatorReceipt({...receipt,...change},intent),false);
});
test('mutation acknowledgement is scoped without inventing a returned request key',()=>{
 const {idempotencyKey,...operation}=receipt;assert.ok(operatorOperation(operation,intent));assert.equal(operatorReceipt(operation,intent),false);
 for(const value of [null,[],{...intent,extra:'unexpected'},{...intent,key:'invalid'},{...intent,kind:'other'},{...intent,state:'deleted'},{...intent,reason:'arbitrary'}])assert.equal(operatorIntent(value),false);
});
