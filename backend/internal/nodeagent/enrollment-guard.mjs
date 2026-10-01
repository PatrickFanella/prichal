// Admission policy contains no administrator credential and never receives signup bodies.
import { createServer } from 'node:http';
import { lstatSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
export function validEnrollmentLease(value, instance, now = Date.now()) {
  return value !== null && typeof value === 'object' && value.instanceId === instance && value.state === 'open' &&
    typeof value.ticket === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value.ticket) &&
    Number.isSafeInteger(value.issuedAt) && Number.isSafeInteger(value.expiresAt) && value.issuedAt <= now &&
    value.expiresAt > now && value.expiresAt > value.issuedAt && value.expiresAt - value.issuedAt <= 15 * 60 * 1000;
}
export function enrollmentAdmission(instance, directory) {
  try {
    const path = join(directory, 'lease.json');
    const info = lstatSync(path);
    return info.isFile() && !(info.mode & 0o077) && info.size <= 4096 && validEnrollmentLease(JSON.parse(readFileSync(path, 'utf8')), instance);
  } catch { return false; }
}
export function emptyFirstAccountInventory(value) {
  return value !== null && typeof value === 'object' && Array.isArray(value.repos) && value.repos.length === 0;
}
if (process.env.PDS_GUARD_INSTANCE) {
  const instance = process.env.PDS_GUARD_INSTANCE;
  if (!/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(instance)) process.exit(1);
  createServer(async (request, response) => {
    response.setHeader('Cache-Control', 'no-store');
    let allowed = request.method === 'GET' && request.url === '/registration' && enrollmentAdmission(instance, '/enrollment');
    if (allowed) {
      try {
        const native = await fetch(`http://pds-ass-tenant-${instance}:2583/xrpc/com.atproto.sync.listRepos?limit=1`, { signal: AbortSignal.timeout(2000) });
        allowed = native.ok && emptyFirstAccountInventory(await native.json());
      } catch { allowed = false; }
    }
    response.statusCode = allowed ? 204 : 403;
    response.end();
  }).listen(2727, '0.0.0.0');
}
