// Fixed local moderation helper. Native administrator credentials never leave the PDS.
const [action,state,ticket,ownedRef='']=process.argv.slice(1);
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
async function call(method,body){
 const response=await fetch('http://127.0.0.1:2583/xrpc/'+method,{method:body===undefined?'GET':'POST',headers:{Authorization:'Basic '+Buffer.from('admin:'+process.env.PDS_ADMIN_PASSWORD).toString('base64'),...(body===undefined?{}:{'Content-Type':'application/json'})},...(body===undefined?{}:{body:JSON.stringify(body)}),signal:AbortSignal.timeout(5000)});
 if(!response.ok)throw new Error('Native moderation unavailable');
 const text=await response.text();if(text.length>8192)throw new Error('Native moderation response exceeds bound');return JSON.parse(text);
}
async function main(){
 if(process.env.PDS_DEV_MODE!=='true'||process.env.PDS_DID_PLC_URL!=='http://pds-ass-protocol-fixtures:2582')throw new Error('Isolated local PDS required');
 if(action!=='native_status'&&action!=='apply')throw new Error('Fixed moderation action required');
 const {DatabaseSync}=require('node:sqlite'),database=new DatabaseSync('/pds/account.sqlite',{readOnly:true});const actors=database.prepare('SELECT did FROM actor LIMIT 2').all();database.close();
 if(actors.length!==1||!/^did:plc:[a-z2-7]{24}$/.test(actors[0].did))throw new Error('Single retained account required');
 const did=actors[0].did;
 const status=()=>call('com.atproto.admin.getSubjectStatus?did='+encodeURIComponent(did));
 let current=await status();
 const valid=value=>value.subject?.$type==='com.atproto.admin.defs#repoRef'&&value.subject.did===did&&typeof value.takedown?.applied==='boolean'&&typeof value.deactivated?.applied==='boolean';
 if(!valid(current))throw new Error('Invalid native subject status');
 if(action==='apply'){
  if(!uuid.test(ticket)||(ownedRef!==''&&!uuid.test(ownedRef))||!['running','suspended'].includes(state))throw new Error('Fixed moderation request required');
  if(current.takedown.applied&&current.takedown.ref!==ticket&&current.takedown.ref!==ownedRef)throw new Error('Unowned moderation must remain intact');
  const applied=state==='suspended';
  if(current.takedown.applied!==applied||(applied&&current.takedown.ref!==ticket))await call('com.atproto.admin.updateSubjectStatus',{subject:{$type:'com.atproto.admin.defs#repoRef',did},takedown:{applied,...(applied?{ref:ticket}:{})}});
  current=await status();
  if(!valid(current)||current.takedown.applied!==applied||(applied&&current.takedown.ref!==ticket))throw new Error('Moderation result unconfirmed');
 }
 return {did,state:current.takedown.applied?'suspended':'running',appliedRef:current.takedown.applied?(current.takedown.ref??''):'',deactivated:current.deactivated.applied};
}
main().then(result=>process.stdout.write(JSON.stringify(result)),()=>{process.stderr.write('Fixed native moderation failed');process.exitCode=1});
