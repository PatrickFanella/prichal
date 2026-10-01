export type ClaimIntent = { id: string; hostname: string };
export type ClaimChoice = ClaimIntent & { organizationId: string; state: 'awaiting_ownership' | 'cancelled'; expiresAt: string; canActivate: false };
export type ClaimReceipt = { choiceId: string; hostname: string; state: 'reserved' | 'released' | 'expired'; reserved: boolean; expiresAt: string; canActivate: false };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(v: unknown): v is Record<string, unknown> { return v !== null && typeof v === 'object' && !Array.isArray(v); }
function hostname(v: unknown): v is string { return typeof v === 'string' && v.length >= 3 && v.length <= 244 && v.split('.').every(label => label.length >= 1 && label.length <= 63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(label)); }
export function claimIntent(v: unknown): v is ClaimIntent { return object(v) && typeof v.id === 'string' && uuid.test(v.id) && hostname(v.hostname); }
export function claimChoice(v: unknown, org: string, target: ClaimIntent): v is ClaimChoice {
  return object(v) && v.id === target.id && v.hostname === target.hostname && v.organizationId === org && ['awaiting_ownership', 'cancelled'].includes(v.state as string) && typeof v.expiresAt === 'string' && Number.isFinite(Date.parse(v.expiresAt)) && v.canActivate === false && claimIntent(v);
}
export function claimReceipt(v: unknown, target: ClaimIntent): v is ClaimReceipt {
  return object(v) && v.choiceId === target.id && v.hostname === target.hostname && ['reserved', 'released', 'expired'].includes(v.state as string) && v.reserved === (v.state === 'reserved') && typeof v.expiresAt === 'string' && Number.isFinite(Date.parse(v.expiresAt)) && v.canActivate === false;
}
