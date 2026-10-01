import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, chmodSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { validEnrollmentLease, enrollmentAdmission } from '../backend/internal/nodeagent/enrollment-guard.mjs';
const id='aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa';
const ticket='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb';
test('Enrollment admission is tenant-bound and fails closed without private, current policy',()=>{
 const value={instanceId:id,ticket,state:'open',issuedAt:1000,expiresAt:2000};
 assert.equal(validEnrollmentLease(value,id,1500),true);
 for(const bad of [null,{...value,instanceId:ticket},{...value,ticket:'invalid'},{...value,state:'closed'},{...value,issuedAt:1600},{...value,expiresAt:1500},{...value,expiresAt:1000+900001},{...value,expiresAt:Infinity}]) assert.equal(validEnrollmentLease(bad,id,1500),false);
 const directory=mkdtempSync(join(tmpdir(),'pds-enrollment-'));
 try {
  assert.equal(enrollmentAdmission(id,directory),false);
  const path=join(directory,'lease.json');
  writeFileSync(path,JSON.stringify({...value,issuedAt:Date.now()-1000,expiresAt:Date.now()+1000}),{mode:0o600});
  assert.equal(enrollmentAdmission(id,directory),true);
  chmodSync(path,0o644); assert.equal(enrollmentAdmission(id,directory),false);
  rmSync(path); symlinkSync('/missing',path); assert.equal(enrollmentAdmission(id,directory),false);
 } finally { rmSync(directory,{recursive:true,force:true}); }
});
