// Fixed native portable export. No SQLite, password, email or private signing key leaves this helper.
const assert=require('node:assert/strict');
const {createRequire}=require('node:module'),{createHash}=require('node:crypto');
const {DatabaseSync}=require('node:sqlite');
const MAX_CAR=8*1024*1024,MAX_MEDIA=8*1024*1024;
async function boundedBody(response,limit){assert.equal(response.status,200);assert.ok(response.body);const chunks=[];let bytes=0;for await(const chunk of response.body){bytes+=chunk.length;assert.ok(bytes<=limit,'Portable export exceeds local bound');chunks.push(chunk)}return Buffer.concat(chunks)}
function references(records){const blobs=new Map();function walk(value,depth=0){assert.ok(depth<64);if(!value||typeof value!=='object')return;if(value.$type==='blob'){const cid=value.ref?.$link??value.ref?.['/'];assert.match(cid??'',/^b[a-z2-7]{20,120}$/);assert.ok(Number.isSafeInteger(value.size)&&value.size>=0&&value.size<=5*1024*1024&&typeof value.mimeType==='string'&&value.mimeType.length<=256);if(blobs.has(cid)){assert.equal(blobs.get(cid).size,value.size);assert.equal(blobs.get(cid).mimeType,value.mimeType)}blobs.set(cid,value);assert.ok(blobs.size<=1000);return}for(const child of Object.values(value))walk(child,depth+1)}for(const record of records)walk(record.record);assert.ok([...blobs.values()].reduce((total,blob)=>total+blob.size,0)<=MAX_MEDIA);return blobs}
async function main(){
 const expected=process.argv[2];assert.match(expected??'',/^did:plc:[a-z2-7]{24}$/);assert.equal(process.env.PDS_DEV_MODE,'true');assert.equal(process.env.PDS_DID_PLC_URL,'http://pds-ass-protocol-fixtures:2582');
 const database=new DatabaseSync('/pds/account.sqlite',{readOnly:true}),actors=database.prepare('SELECT did FROM actor LIMIT 2').all();database.close();assert.equal(actors.length,1);assert.equal(actors[0].did,expected);
 const root=createRequire('/app/package.json'),upstream=createRequire(root.resolve('@atproto/pds')),{verifyRepoCar,verifyRecords}=upstream('@atproto/repo'),{sha256RawToCid}=upstream('@atproto/common');
 const pds='http://127.0.0.1:2583';async function get(url){return fetch(url,{signal:AbortSignal.timeout(15000),...(url.startsWith(pds+'/')?{headers:{Authorization:'Basic '+Buffer.from('admin:'+process.env.PDS_ADMIN_PASSWORD).toString('base64')}}:{})})}
 const identity=JSON.parse((await boundedBody(await get(process.env.PDS_DID_PLC_URL+'/'+expected+'/data'),65536)).toString());const signingKey=identity.verificationMethods.atproto;
 const car=await boundedBody(await get(pds+'/xrpc/com.atproto.sync.getRepo?did='+expected),MAX_CAR);await verifyRepoCar(car,expected,signingKey);const records=JSON.parse(JSON.stringify(await verifyRecords(car,expected,signingKey)));assert.ok(records.length<=10000);
 const blobs=[];for(const [cid,blob]of references(records)){const bytes=await boundedBody(await get(pds+'/xrpc/com.atproto.sync.getBlob?did='+expected+'&cid='+cid),Math.min(blob.size+1,5*1024*1024));assert.equal(bytes.length,blob.size);assert.equal(sha256RawToCid(createHash('sha256').update(bytes).digest()).toString(),cid);blobs.push({cid,mimeType:blob.mimeType,data:bytes.toString('base64')})}
 return {did:expected,signingKey,car:car.toString('base64'),blobs:blobs.sort((a,b)=>a.cid.localeCompare(b.cid)),signedRecords:records.length,capturedAt:new Date().toISOString()};
}
module.exports={boundedBody,references};
if(process.argv[1]==='native_export')main().then(value=>console.log(JSON.stringify(value)),()=>{console.error('Native portable export could not be verified');process.exitCode=1});
