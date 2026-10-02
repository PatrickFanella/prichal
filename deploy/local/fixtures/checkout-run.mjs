// Explicit disposable authority runner. Arguments contain paths/configuration, never credentials.
import assert from 'node:assert/strict';
import {lstatSync,readFileSync} from 'node:fs';
import {createCheckoutAuthority} from './checkout-authority.mjs';
try {
 const [root,keyFile,priceId,returnOrigin,portText='0',controlFile,webhookFile,...extra]=process.argv.slice(2);assert.equal(extra.length,0);assert.ok(root&&keyFile&&priceId&&returnOrigin);assert.match(portText,/^(0|[1-9][0-9]{0,4})$/);const port=Number(portText);assert.ok(port<=65535);assert.equal(Boolean(controlFile),Boolean(webhookFile));
 function secret(path){const stat=lstatSync(path);assert.ok(stat.isFile()&&!(stat.mode&0o077)&&stat.size<=256);return readFileSync(path,'utf8').trim();}
 const apiKey=secret(keyFile),server=createCheckoutAuthority({root,apiKey,priceId,returnOrigin,...(controlFile?{controlKey:secret(controlFile),webhookSecret:secret(webhookFile)}:{})});
 server.on('error',()=>{console.error('Disposable checkout authority unavailable.');process.exitCode=1;});
 server.listen(port,'127.0.0.1',()=>console.log(JSON.stringify({fixture:true,origin:'http://127.0.0.1:'+server.address().port,payments:false})));
 process.on('SIGINT',()=>server.close());process.on('SIGTERM',()=>server.close());
} catch {console.error('Disposable checkout authority unavailable.');process.exitCode=1;}
