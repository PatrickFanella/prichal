// Explicit disposable authority runner. Arguments contain paths/configuration, never credentials.
import assert from 'node:assert/strict';
import {lstatSync,readFileSync} from 'node:fs';
import {createCheckoutAuthority} from './checkout-authority.mjs';
try {
 const [root,keyFile,priceId,returnOrigin,...extra]=process.argv.slice(2);assert.equal(extra.length,0);assert.ok(root&&keyFile&&priceId&&returnOrigin);
 const stat=lstatSync(keyFile);assert.ok(stat.isFile()&&!(stat.mode&0o077)&&stat.size<=256);
 const apiKey=readFileSync(keyFile,'utf8').trim(),server=createCheckoutAuthority({root,apiKey,priceId,returnOrigin});
 server.listen(0,'127.0.0.1',()=>console.log(JSON.stringify({fixture:true,origin:'http://127.0.0.1:'+server.address().port,payments:false})));
 process.on('SIGINT',()=>server.close());process.on('SIGTERM',()=>server.close());
} catch {console.error('Disposable checkout authority unavailable.');process.exitCode=1;}
