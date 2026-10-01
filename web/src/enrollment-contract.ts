export type Enrollment = {
  id: string; deploymentId: string; state: string; message: string; code?: string; expiresAt: string | null;
  accountUrl: string; did?: string; handle?: string; emailConfirmed: boolean;
  withdrawalRequested: boolean; withdrawalAttempts: number;
};
export const enrollmentUUID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
export function enrollmentObject(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value); }
export function validEnrollment(value: unknown, deploymentId: string): value is Enrollment {
  if (!enrollmentObject(value) || typeof value.id !== 'string' || !enrollmentUUID.test(value.id) || value.deploymentId !== deploymentId ||
    typeof value.state !== 'string' || !['pending', 'invitation_ready', 'account_created', 'verified', 'expired', 'failed', 'superseded', 'withdrawing', 'withdrawn'].includes(value.state) ||
    typeof value.message !== 'string' || value.message.length > 1024 || typeof value.accountUrl !== 'string' || typeof value.emailConfirmed !== 'boolean' ||
    typeof value.withdrawalRequested !== 'boolean' || typeof value.withdrawalAttempts !== 'number' || !Number.isSafeInteger(value.withdrawalAttempts) || value.withdrawalAttempts < 0 || value.withdrawalAttempts > 3 ||
    value.withdrawalRequested !== (value.withdrawalAttempts > 0) ||
    !(value.expiresAt === null || (typeof value.expiresAt === 'string' && Number.isFinite(Date.parse(value.expiresAt)))) ||
    !(value.code === undefined || (typeof value.code === 'string' && /^[A-Za-z0-9._:-]{1,256}$/.test(value.code)))) return false;
  if (value.code !== undefined && value.state !== 'invitation_ready') return false;
  if (value.state === 'invitation_ready' && (value.code === undefined || value.expiresAt === null || value.withdrawalRequested)) return false;
  if (value.withdrawalRequested && ['pending', 'invitation_ready', 'expired'].includes(value.state)) return false;
  if (['withdrawing', 'withdrawn'].includes(value.state) && !value.withdrawalRequested) return false;
  if (['account_created', 'verified'].includes(value.state)) {
    if (typeof value.did !== 'string' || !/^did:plc:[a-z2-7]{24}$/.test(value.did) || typeof value.handle !== 'string' || value.handle.length < 1 || value.handle.length > 253 || value.expiresAt !== null || (value.state === 'verified' && !value.emailConfirmed)) return false;
  } else if (value.did !== undefined || value.handle !== undefined || value.emailConfirmed) return false;
  try {
    const url = new URL(value.accountUrl);
    return url.protocol === 'https:' && url.hostname === `${deploymentId}.localhost` && url.port === String(36000 + parseInt(deploymentId.slice(0, 4), 16) % 10000) && url.pathname === '/account' && !url.username && !url.password && !url.search && !url.hash;
  } catch { return false; }
}
