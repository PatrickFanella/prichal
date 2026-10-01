// Local operator export verification. The native admin token remains inside the PDS.
const assert=require('node:assert/strict');
const {createRequire}=require('node:module'),{createHash}=require('node:crypto');
const {DatabaseSync}=require('node:sqlite');
async function main(){
 assert.equal(process.env.PDS_DEV_MODE,'true');assert.equal(process.env.PDS_DID_PLC_URL,'http://pds-ass-protocol-fixtures:2582');
 const root=createRequire('/app/package.json'),upstream=createRequire(root.resolve('@atproto/pds'));
 const {verifyRepoCar,verifyRecords}=upstream('@atproto/repo'),{sha256RawToCid}=upstream('@atproto/common');
 const database=new DatabaseSync('/pds/account.sqlite',{readOnly:true}),actors=database.prepare('SELECT did FROM actor LIMIT 2').all();database.close();assert.equal(actors.length,1);const did=actors[0].did;assert.match(did,/^did:plc:[a-z2-7]{24}$/);
 const pds='http://127.0.0.1:2583';
 async function get(url){const response=await fetch(url,{signal:AbortSignal.timeout(15000),...(url.startsWith(pds+'/')?{headers:{Authorization:'Basic '+Buffer.from('admin:'+process.env.PDS_ADMIN_PASSWORD).toString('base64')}}:{})});assert.equal(response.status,200);return response}
 const identity=await (await get(process.env.PDS_DID_PLC_URL+'/'+did+'/data')).json();
 const car=Buffer.from(await (await get(pds+'/xrpc/com.atproto.sync.getRepo?did='+did)).arrayBuffer());assert.ok(car.length<=16*1024*1024);await verifyRepoCar(car,did,identity.verificationMethods.atproto);
 const records=JSON.parse(JSON.stringify(await verifyRecords(car,did,identity.verificationMethods.atproto)));assert.ok(records.length<=10000);
 const blobs=new Map();function walk(value,depth=0){assert.ok(depth<64);if(!value||typeof value!=='object')return;if(value.$type==='blob'){const cid=value.ref?.$link??value.ref?.['/'];assert.equal(typeof cid,'string');assert.ok(Number.isSafeInteger(value.size)&&value.size>=0&&value.size<=5*1024*1024);blobs.set(cid,value);return}for(const child of Object.values(value))walk(child,depth+1)}
 for(const record of records)walk(record.record);
 const media=[];for(const [cid,blob]of blobs){const bytes=Buffer.from(await (await get(pds+'/xrpc/com.atproto.sync.getBlob?did='+did+'&cid='+cid)).arrayBuffer());assert.equal(bytes.length,blob.size);const hash=createHash('sha256').update(bytes).digest();assert.equal(sha256RawToCid(hash).toString(),cid);media.push({did,cid,bytes:bytes.length,mimeType:blob.mimeType,sha256:hash.toString('hex')})}
 return {repositories:1,signedRecords:records.length,referencedBlobs:media.sort((a,b)=>a.cid.localeCompare(b.cid))};
}
main().then(result=>console.log(JSON.stringify(result)),()=>{console.error('Local operator held export verification failed');process.exitCode=1});
