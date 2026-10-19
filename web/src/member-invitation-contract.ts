const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === "object" && !Array.isArray(value); }

export type MemberInvitation = {
  id: string; deploymentId: string; state: string; message: string; code?: string;
  expiresAt: string | null; accountUrl: string; did?: string; handle?: string;
  withdrawalRequested: boolean; createdAt: string;
};
export type MemberSummary = { ceiling: number; members: number; open: number; invitations: MemberInvitation[] };
const date = (value: unknown): value is string => typeof value === "string" && Number.isFinite(Date.parse(value));
export function validMemberInvitation(value: unknown, deploymentId: string): value is MemberInvitation {
  if (!object(value) || typeof value.id !== "string" || !uuid.test(value.id) || value.deploymentId !== deploymentId ||
    typeof value.state !== "string" || !["pending", "invitation_ready", "accepted", "expired", "withdrawing", "withdrawn", "ceiling_reached", "owner_required", "failed"].includes(String(value.state)) ||
    typeof value.message !== "string" || value.message.length > 1024 || typeof value.withdrawalRequested !== "boolean" || !date(value.createdAt) ||
    !(value.expiresAt === null || date(value.expiresAt)) || typeof value.accountUrl !== "string") return false;
  if (value.state === "invitation_ready") {
    if (value.withdrawalRequested || !date(value.expiresAt) || typeof value.code !== "string" || !/^[A-Za-z0-9._:-]{1,256}$/.test(value.code)) return false;
  } else if (value.code !== undefined) return false;
  if (["withdrawing", "withdrawn"].includes(String(value.state)) && !value.withdrawalRequested) return false;
  if (value.state === "accepted") {
    if (typeof value.did !== "string" || !/^did:plc:[a-z2-7]{24}$/.test(value.did) || typeof value.handle !== "string" || !/^[a-zA-Z0-9.-]{1,253}$/.test(value.handle)) return false;
  } else if (value.did !== undefined || value.handle !== undefined) return false;
  try {
    const url = new URL(value.accountUrl);
    return url.protocol === "https:" && url.hostname === `${deploymentId}.localhost` && url.port === String(36000 + parseInt(deploymentId.slice(0, 4), 16) % 10000) && url.pathname === "/account" && !url.username && !url.password && !url.search && !url.hash;
  } catch { return false; }
}
export function validMemberSummary(value: unknown, deploymentId: string): value is MemberSummary {
  return object(value) && Number.isSafeInteger(value.ceiling) && Number(value.ceiling) > 0 && Number(value.ceiling) <= 25 &&
    Number.isSafeInteger(value.members) && Number(value.members) >= 0 && Number.isSafeInteger(value.open) && Number(value.open) >= 0 &&
    Array.isArray(value.invitations) && value.invitations.length <= 100 && value.invitations.every(item => validMemberInvitation(item, deploymentId)) &&
    new Set(value.invitations.map(item => item.id)).size === value.invitations.length;
}
