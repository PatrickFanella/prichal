// Fixed read-only usage helper. No administrator credential, account identity or host path leaves the PDS.
const fs=require('node:fs');
async function main(){
 if(process.env.PDS_DEV_MODE!=='true')throw new Error('Local usage qualification required');
 let visited=0;
 function walk(path,depth=0){if(depth>12)throw new Error('Usage traversal depth exceeded');let bytes=0,files=0;for(const name of fs.readdirSync(path)){if(++visited>100000)throw new Error('Usage traversal bound exceeded');const child=path+'/'+name;let stat;try{stat=fs.lstatSync(child)}catch(error){if(error.code==='ENOENT')continue;throw error}if(stat.isSymbolicLink())throw new Error('Usage traversal symlink rejected');if(stat.isDirectory()){const nested=walk(child,depth+1);bytes+=nested.bytes;files+=nested.files}else if(stat.isFile()){bytes+=stat.size;files++}else throw new Error('Usage traversal file type rejected');if(!Number.isSafeInteger(bytes))throw new Error('Usage bytes overflow');}return {bytes,files}}
 const data=walk('/pds'),blobs=walk('/blobs');
 const {DatabaseSync}=require('node:sqlite'),db=new DatabaseSync('/pds/account.sqlite',{readOnly:true});const accounts=db.prepare('SELECT count(*) AS count FROM actor').get().count;db.close();
 const disk=fs.statfsSync('/pds');
 const value={dataBytes:data.bytes,blobBytes:blobs.bytes,dataFiles:data.files,blobFiles:blobs.files,accounts,filesystemBytes:disk.blocks*disk.bsize,filesystemAvailableBytes:disk.bavail*disk.bsize,observedAtMillis:Date.now()};
 if(!Object.values(value).every(Number.isSafeInteger))throw new Error('Usage metric overflow');
 process.stdout.write(JSON.stringify(value));
}
main().catch(()=>{process.stderr.write('Native usage measurement unavailable');process.exitCode=1});
