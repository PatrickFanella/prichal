// Fixed public staging origin; upstream remains a separate loopback API.
import {createServer,request} from 'node:http';
import {createReadStream,existsSync,statSync} from 'node:fs';
import {resolve,extname} from 'node:path';
import {fileURLToPath} from 'node:url';
const origin='https://prichal-staging.subcult.tv';
const root=resolve(fileURLToPath(new URL('.',import.meta.url)),'web');
const types={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.woff2':'font/woff2','.svg':'image/svg+xml'};
createServer((req,res)=>{
 if(req.headers.host!=='prichal-staging.subcult.tv'){res.writeHead(421);res.end('Unknown host');return;}
 res.setHeader('X-Content-Type-Options','nosniff');res.setHeader('X-Frame-Options','DENY');res.setHeader('Referrer-Policy','same-origin');res.setHeader('Strict-Transport-Security','max-age=31536000');
 let url;try{url=new URL(req.url,origin);}catch{res.writeHead(400);res.end();return;}
 if(url.origin!==origin){res.writeHead(400);res.end();return;}
 if(url.pathname.startsWith('/api/')){
  const headers={...req.headers,host:'prichal-staging.subcult.tv'};
  delete headers['x-forwarded-host'];delete headers['forwarded'];
  const upstream=request({hostname:'127.0.0.1',port:8088,path:url.pathname+url.search,method:req.method,headers},response=>{res.writeHead(response.statusCode,response.headers);response.pipe(res);});
  upstream.setTimeout(15000,()=>upstream.destroy());upstream.on('error',()=>{if(!res.headersSent)res.writeHead(502);res.end('Staging API unavailable');});req.on('aborted',()=>upstream.destroy());req.pipe(upstream);return;
 }
 if(req.method!=='GET'&&req.method!=='HEAD'){res.writeHead(405);res.end();return;}
 let path;try{path=resolve(root,'.'+decodeURIComponent(url.pathname));}catch{res.writeHead(400);res.end();return;}
 if(!path.startsWith(root+'/')&&path!==root){res.writeHead(404);res.end();return;}
 if(!existsSync(path)||!statSync(path).isFile())path=resolve(root,'index.html');
 res.writeHead(200,{'Content-Type':types[extname(path)]??'application/octet-stream','Cache-Control':'no-store'});
 if(req.method==='HEAD')res.end();else createReadStream(path).pipe(res);
}).listen(5178,'127.0.0.1',()=>console.log('Public staging upstream on loopback 5178'));
