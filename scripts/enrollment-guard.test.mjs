import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, chmodSync, symlinkSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { validEnrollmentLease, enrollmentAdmission, emptyFirstAccountInventory, validMemberAdmission, memberAdmission, memberInventoryAdmits } from '../backend/internal/nodeagent/enrollment-guard.mjs';
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

test('First-account inventory denies populated and unavailable native state',()=>{
 assert.equal(emptyFirstAccountInventory({repos:[]}),true);
 for(const value of [null,{}, {repos:null},{repos:[{did:'did:plc:fixture'}]},{repos:'invalid'}]) assert.equal(emptyFirstAccountInventory(value),false);
});

test('Member admission needs a current tenant-bound invitation and never carries codes',()=>{
 const entry={ticket,issuedAt:1000,expiresAt:2000};
 const value={instanceId:id,ceiling:3,invitations:[entry]};
 assert.equal(validMemberAdmission(value,id,1500),true);
 assert.equal(validMemberAdmission({...value,invitations:[{...entry,expiresAt:1400},entry]},id,1500),true);
 for(const bad of [null,{...value,instanceId:ticket},{...value,ceiling:0},{...value,ceiling:26},{...value,ceiling:2.5},{...value,invitations:[]},{...value,invitations:'x'},
  {...value,invitations:[{...entry,ticket:'invalid'}]},{...value,invitations:[{...entry,expiresAt:1500}]},{...value,invitations:[{...entry,issuedAt:1600}]},
  {...value,invitations:[{...entry,expiresAt:1000+72*3600000+1}]},{...value,invitations:Array(26).fill(entry)}]) assert.equal(validMemberAdmission(bad,id,1500),false);
 const directory=mkdtempSync(join(tmpdir(),'pds-members-'));
 try {
  assert.equal(memberAdmission(id,directory),0);
  const path=join(directory,'members.json');
  writeFileSync(path,JSON.stringify({...value,invitations:[{ticket,issuedAt:Date.now()-1000,expiresAt:Date.now()+1000}]}),{mode:0o600});
  assert.equal(memberAdmission(id,directory),3);
  assert.equal(enrollmentAdmission(id,directory),false,'member policy must not open first-account admission');
  chmodSync(path,0o644); assert.equal(memberAdmission(id,directory),0);
  rmSync(path); symlinkSync('/missing',path); assert.equal(memberAdmission(id,directory),0);
 } finally { rmSync(directory,{recursive:true,force:true}); }
});

test('Member inventory requires the owner account and stays below the ceiling',()=>{
 const repos=n=>({repos:Array.from({length:n},(_,i)=>({did:'did:plc:'+i}))});
 assert.equal(memberInventoryAdmits(repos(1),3),true);
 assert.equal(memberInventoryAdmits(repos(3),3),true);
 for(const [value,ceiling] of [[repos(0),3],[repos(4),3],[null,3],[{},3],[{repos:'x'},3]]) assert.equal(memberInventoryAdmits(value,ceiling),false);
});
