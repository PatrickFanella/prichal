// Immutable local staging build with a bounded same-origin API proxy.
import { createServer, request } from 'node:http';
import { createReadStream, existsSync, statSync } from 'node:fs';
import { resolve, extname } from 'node:path';
import { fileURLToPath } from 'node:url';
const root=resolve(fileURLToPath(new URL('.',import.meta.url)),'web');
const types={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.woff2':'font/woff2','.svg':'image/svg+xml'};
createServer((req,res)=>{
 let url;try{url=new URL(req.url,'http://127.0.0.1:5177');}catch{res.writeHead(400);res.end();return;}
 if(url.pathname.startsWith('/api/')){
  const upstream=request({hostname:'127.0.0.1',port:8087,path:url.pathname+url.search,method:req.method,headers:{...req.headers,host:'127.0.0.1:5177'}},response=>{res.writeHead(response.statusCode,response.headers);response.pipe(res);});
  upstream.setTimeout(15000,()=>upstream.destroy());upstream.on('error',()=>{if(!res.headersSent)res.writeHead(502);res.end('Staging API unavailable');});req.on('aborted',()=>upstream.destroy());req.pipe(upstream);return;
 }
 if(req.method!=='GET'&&req.method!=='HEAD'){res.writeHead(405);res.end();return;}
 let path;try{path=resolve(root,'.'+decodeURIComponent(url.pathname));}catch{res.writeHead(400);res.end();return;}
 if(!path.startsWith(root+'/')&&path!==root){res.writeHead(404);res.end();return;}
 if(!existsSync(path)||!statSync(path).isFile())path=resolve(root,'index.html');
 res.writeHead(200,{'Content-Type':types[extname(path)]??'application/octet-stream','Cache-Control':'no-store','X-Content-Type-Options':'nosniff'});
 if(req.method==='HEAD')res.end();else createReadStream(path).pipe(res);
}).listen(5177,'127.0.0.1',()=>console.log('Private staging management on http://127.0.0.1:5177'));
