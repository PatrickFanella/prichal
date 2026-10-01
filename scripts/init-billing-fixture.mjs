import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdirSync,existsSync,writeFileSync,readFileSync,lstatSync} from 'node:fs';
const root='.local/billing-fixture';mkdirSync(root,{recursive:true,mode:0o700});assert.ok(!(lstatSync(root).mode&0o077));
for(const [name,prefix] of [['webhook.key','whsec_'],['api.key','rk_test_']]){const path=root+'/'+name;if(!existsSync(path))writeFileSync(path,prefix+randomBytes(32).toString('hex'),{mode:0o600,flag:'wx'});const info=lstatSync(path);assert.ok(info.isFile()&&!(info.mode&0o077)&&info.size<256);assert.ok(readFileSync(path,'utf8').startsWith(prefix));}
const current=root+'/subscription.json';if(!existsSync(current))writeFileSync(current,JSON.stringify({id:'sub_fixtureAlice',object:'subscription',customer:'cus_fixtureAlice',livemode:false,status:'active',items:{object:'list',has_more:false,data:[{id:'si_fixtureAlice',object:'subscription_item',quantity:1,current_period_end:Math.floor(Date.now()/1000)+86400,price:{id:'price_fixtureSingle',object:'price',livemode:false}}]}}),{mode:0o600,flag:'wx'});
