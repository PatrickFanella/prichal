import assert from 'node:assert/strict';
import { mkdirSync, lstatSync, chmodSync, openSync, writeFileSync, fsyncSync, closeSync, readFileSync, linkSync, unlinkSync } from 'node:fs';
import { randomBytes, randomUUID } from 'node:crypto';
mkdirSync('.local',{recursive:true,mode:0o700});
assert.ok(lstatSync('.local').isDirectory(),'Local runtime must be a regular directory');
chmodSync('.local',0o700);
const path='.local/enrollment.key';
let exists=true;
try { lstatSync(path); } catch(error) { if(error.code!=='ENOENT') throw error;exists=false; }
if(!exists) {
 const temporary='.local/.enrollment-key-'+randomUUID()+'.tmp';
 const descriptor=openSync(temporary,'wx',0o600);
 try {
  try { writeFileSync(descriptor,randomBytes(32));fsyncSync(descriptor); } finally { closeSync(descriptor); }
  try { linkSync(temporary,path); } catch(error) { if(error.code!=='EEXIST') throw error; }
  const folder=openSync('.local','r');try { fsyncSync(folder); } finally { closeSync(folder); }
 } finally { unlinkSync(temporary); }
}
const info=lstatSync(path);
assert.ok(info.isFile() && !(info.mode&0o077) && info.size===32 && readFileSync(path).length===32,'Private enrollment key required');
