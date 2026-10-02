import test from 'node:test';
import assert from 'node:assert/strict';
import { lifecycleHistory } from '../web/src/lifecycle-history-contract.ts';
const row = generation => ({ id: `eeeeeeee-eeee-4eee-8eee-${String(generation).padStart(12,'0')}`, kind:'service',generation,state:'applied',desiredState:'running',reason:'maintenance',actor:'operator' });
test('history rejects unsafe pagination, repeated rows and unredacted owner actors', () => {
 const items=Array.from({length:50},(_,i)=>row(100-i)),page={items,hasMore:true,nextBefore:51};
 assert.ok(lifecycleHistory(page,false));
 assert.ok(lifecycleHistory({items:[row(50)],hasMore:false,nextBefore:null},false,51));
 for(const invalid of [{...page,nextBefore:50},{...page,items:items.slice(0,49)},{...page,items:[...items,row(1)]},{items:[row(1),row(2)],hasMore:false,nextBefore:null},{items:[row(2),row(2)],hasMore:false,nextBefore:null},{items:[{...row(1),actor:'private-operator'}],hasMore:false,nextBefore:null},{items:[row(Number.MAX_SAFE_INTEGER+1)],hasMore:false,nextBefore:null}]) assert.equal(lifecycleHistory(invalid,false),false);
 assert.equal(lifecycleHistory({items:[row(51)],hasMore:false,nextBefore:null},false,51),false);
});
test('terminal pages and operator projection must remain internally consistent', () => {
 const page={items:[{...row(1),actor:'local-operator'}],hasMore:false,nextBefore:null};
 assert.ok(lifecycleHistory(page,true));
 assert.equal(lifecycleHistory({...page,nextBefore:1},true),false);
 assert.equal(lifecycleHistory({...page,items:[{...page.items[0],state:'unknown'}]},true),false);
 assert.ok(lifecycleHistory({items:[],hasMore:false,nextBefore:null},false));
 assert.equal(lifecycleHistory({items:[],hasMore:true,nextBefore:1},false),false);
});
