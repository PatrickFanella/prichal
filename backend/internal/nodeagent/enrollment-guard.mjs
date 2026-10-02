// Admission policy contains no administrator credential and never receives signup bodies.
import { createServer } from 'node:http';
import { lstatSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
export function validEnrollmentLease(value, instance, now = Date.now()) {
  return value !== null && typeof value === 'object' && value.instanceId === instance && value.state === 'open' &&
    typeof value.ticket === 'string' && uuid.test(value.ticket) &&
    Number.isSafeInteger(value.issuedAt) && Number.isSafeInteger(value.expiresAt) && value.issuedAt <= now &&
    value.expiresAt > now && value.expiresAt > value.issuedAt && value.expiresAt - value.issuedAt <= 15 * 60 * 1000;
}
function privatePolicy(directory, file) {
  const path = join(directory, file);
  const info = lstatSync(path);
  if (!info.isFile() || info.mode & 0o077 || info.size > 4096) throw new Error('unsafe policy');
  return JSON.parse(readFileSync(path, 'utf8'));
}
export function enrollmentAdmission(instance, directory) {
  try { return validEnrollmentLease(privatePolicy(directory, 'lease.json'), instance); } catch { return false; }
}
export function emptyFirstAccountInventory(value) {
  return value !== null && typeof value === 'object' && Array.isArray(value.repos) && value.repos.length === 0;
}
// Member admission is open while at least one current invitation exists. Codes
// never reach the guard; the native single-use code still binds each signup.
export function validMemberAdmission(value, instance, now = Date.now()) {
  if (value === null || typeof value !== 'object' || value.instanceId !== instance || !Number.isSafeInteger(value.ceiling) ||
    value.ceiling < 1 || value.ceiling > 25 || !Array.isArray(value.invitations) || value.invitations.length > 25) return false;
  return value.invitations.some(entry => entry !== null && typeof entry === 'object' && typeof entry.ticket === 'string' && uuid.test(entry.ticket) &&
    Number.isSafeInteger(entry.issuedAt) && Number.isSafeInteger(entry.expiresAt) && entry.issuedAt <= now && entry.expiresAt > now &&
    entry.expiresAt - entry.issuedAt <= 72 * 60 * 60 * 1000);
}
export function memberAdmission(instance, directory) {
  try {
    const value = privatePolicy(directory, 'members.json');
    return validMemberAdmission(value, instance) ? value.ceiling : 0;
  } catch { return 0; }
}
// The owner account must exist, and owner plus members stays below the ceiling.
export function memberInventoryAdmits(value, ceiling) {
  return value !== null && typeof value === 'object' && Array.isArray(value.repos) && value.repos.length >= 1 && value.repos.length <= ceiling;
}
if (process.env.PDS_GUARD_INSTANCE) {
  const instance = process.env.PDS_GUARD_INSTANCE;
  if (!uuid.test(instance)) process.exit(1);
  createServer(async (request, response) => {
    response.setHeader('Cache-Control', 'no-store');
    let allowed = false;
    if (request.method === 'GET' && request.url === '/registration') {
      const first = enrollmentAdmission(instance, '/enrollment');
      const ceiling = first ? 0 : memberAdmission(instance, '/enrollment');
      if (first || ceiling) {
        try {
          const native = await fetch(`http://pds-ass-tenant-${instance}:2583/xrpc/com.atproto.sync.listRepos?limit=${first ? 1 : ceiling + 1}`, { signal: AbortSignal.timeout(2000) });
          const inventory = native.ok ? await native.json() : null;
          allowed = first ? emptyFirstAccountInventory(inventory) : memberInventoryAdmits(inventory, ceiling);
        } catch { allowed = false; }
      }
    }
    response.statusCode = allowed ? 204 : 403;
    response.end();
  }).listen(2727, '0.0.0.0');
}
