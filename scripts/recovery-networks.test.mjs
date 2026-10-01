import test from 'node:test';
import assert from 'node:assert/strict';
import {recoverySubnets,composeLiterals} from './recovery-networks.mjs';
test('recovery subnets avoid occupied parents and partial overlaps',()=>{
 assert.deepEqual(recoverySubnets(['172.17.0.0/16','10.252.0.128/25','10.252.1.0/24','fd00::/64']),['10.252.2.0/24','10.252.3.0/24']);
 assert.deepEqual(recoverySubnets(['10.252.0.0/23']),['10.252.2.0/24','10.252.3.0/24']);
 assert.throws(()=>recoverySubnets(['10.252.0.0/16']));assert.throws(()=>recoverySubnets(['10.0.999.1/24']));assert.throws(()=>recoverySubnets(['0.0.0.0/0']));
});
test('Compose serialization protects dollar expressions in runtime code and values',()=>{
 const rendered=JSON.parse(composeLiterals({command:['node','-e','console.log(`${instance}:$x:$$`)'],environment:{SECRET:'a$b'}}));
 assert.equal(rendered.command[2],'console.log(`$${instance}:$$x:$$$$`)');assert.equal(rendered.environment.SECRET,'a$$b');
});
