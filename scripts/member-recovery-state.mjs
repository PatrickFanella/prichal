// Fixed private host metadata for member admission and retained migration windows.
// These are checkpoint files, never a guard mount or public inventory response.
import assert from 'node:assert/strict';
import { constants, copyFileSync, lstatSync, mkdirSync, readFileSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function directory(path) {
  const info = lstatSync(path);
  assert.ok(isAbsolute(path) && info.isDirectory() && !(info.mode & 0o077), 'Private regular recovery directory required');
}
function read(path, maximum) {
  let info;
  try { info = lstatSync(path); } catch (error) { if (error.code === 'ENOENT') return null; throw error; }
  assert.ok(info.isFile() && !(info.mode & 0o077) && info.size <= maximum, 'Unsafe member recovery file');
  const bytes = readFileSync(path);
  let value; try { value = JSON.parse(bytes); } catch { throw Error('Invalid member recovery JSON'); }
  assert.ok(value && typeof value === 'object' && !Array.isArray(value), 'Invalid member recovery metadata');
  return value;
}
function metadata(invitation, exits, policy, tenant) {
  for (const value of [invitation, exits, policy].filter(Boolean)) assert.ok(value.instanceId === tenant, 'Foreign member recovery identity');
  if (invitation) assert.ok((invitation.entries === null || Array.isArray(invitation.entries)) && (invitation.entries?.length ?? 0) <= 256, 'Invalid member journal bound');
  if (exits) {
    assert.ok(invitation && exits.version === 1 && Array.isArray(exits.entries) && exits.entries.length <= 25, 'Retained exit requires member journal');
    for (const exit of exits.entries) assert.ok(exit && typeof exit === 'object' && invitation.entries?.some(member => member.ticket === exit.ticket && member.state === 'accepted' && member.did === exit.did), 'Exit lost its retained accepted identity');
  }
  if (policy) assert.ok(invitation && Number.isInteger(policy.ceiling) && policy.ceiling >= 1 && policy.ceiling <= 25 && Array.isArray(policy.invitations) && policy.invitations.length <= 25, 'Admission policy requires member journal');
}
// Discovery is a fixed allowlist. No caller supplies individual filenames.
export function memberRecoveryFiles(state, tenant) {
  assert.match(tenant, uuid, 'Fixed tenant UUID required'); directory(state);
  const invitationPath = join(state, `${tenant}-members.json`), exitPath = join(state, `${tenant}-member-exits.json`);
  const enrollment = join(state, `${tenant}-enrollment`);
  directory(enrollment);
  const policyPath = join(enrollment, 'members.json');
  const invitation = read(invitationPath, 256 << 10), exits = read(exitPath, 64 << 10), policy = read(policyPath, 4096);
  metadata(invitation, exits, policy, tenant);
  return [invitation && [invitationPath, 'member-invitations.json'], exits && [exitPath, 'member-exits.json'], policy && [policyPath, 'member-admission.json']].filter(Boolean);
}
// Rebuild only these sidecars in a fresh private candidate directory. Existing
// files are never replaced; the caller retains the failed candidate on error.
export function restoreMemberRecoveryFiles(checkpoint, candidate, tenant) {
  assert.match(tenant, uuid, 'Fixed tenant UUID required'); directory(checkpoint); directory(candidate);
  const invitation = read(join(checkpoint, 'member-invitations.json'), 256 << 10), exits = read(join(checkpoint, 'member-exits.json'), 64 << 10), policy = read(join(checkpoint, 'member-admission.json'), 4096);
  metadata(invitation, exits, policy, tenant);
  const enrollment = join(candidate, `${tenant}-enrollment`);
  const files = [invitation && ['member-invitations.json', join(candidate, `${tenant}-members.json`)], exits && ['member-exits.json', join(candidate, `${tenant}-member-exits.json`)], policy && ['member-admission.json', join(enrollment, 'members.json')]].filter(Boolean);
  for (const [, target] of files) {
    try { lstatSync(target); throw Error('Existing member recovery target refused'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  try { mkdirSync(enrollment, { mode: 0o700 }); } catch (error) { if (error.code !== 'EEXIST') throw error; } directory(enrollment);
  for (const [source, target] of files) {
    copyFileSync(join(checkpoint, source), target, constants.COPYFILE_EXCL);
    assert.ok(readFileSync(target).equals(readFileSync(join(checkpoint, source))), 'Restored member metadata differs');
  }
  return { files: files.length, guardPolicy: policy ? join(enrollment, 'members.json') : null };
}
