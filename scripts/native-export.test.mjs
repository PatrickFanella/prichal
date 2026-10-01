import test from 'node:test';import assert from 'node:assert/strict';import {createRequire} from 'node:module';
const {boundedBody,references}=createRequire(import.meta.url)('../backend/internal/nodeagent/export-helper.cjs');
const cid='b'+ 'a'.repeat(58),blob=size=>({$type:'blob',ref:{$link:cid},size,mimeType:'image/png'});
test('portable references reject unsafe paths, depth and aggregate content before fetching',()=>{
 assert.equal(references([{record:{nested:blob(4)}}]).size,1);const other={...blob(5*1024*1024),ref:{$link:'b'+'c'.repeat(58)}};assert.throws(()=>references([{record:blob(5*1024*1024)},{record:other}]));assert.throws(()=>references([{record:{$type:'blob',ref:{$link:'../../keys'},size:1,mimeType:'x'}}]));assert.throws(()=>references([{record:blob(6*1024*1024)}]));assert.throws(()=>references([{record:blob(2)},{record:blob(3)}]));const deep={};let next=deep;for(let i=0;i<70;i++){next.value={};next=next.value}assert.throws(()=>references([{record:deep}]));
});
test('portable HTTP reads enforce size while streaming and reject unsuccessful responses',async()=>{
 assert.deepEqual(await boundedBody(new Response('abc'),3),Buffer.from('abc'));await assert.rejects(boundedBody(new Response('abcd'),3));await assert.rejects(boundedBody(new Response('private',{status:401}),100));
});
