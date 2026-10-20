const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const object = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);
const date = (v: unknown): v is string => typeof v === "string" && Number.isFinite(Date.parse(v));
const states = ["requested", "migration_window", "deactivated", "review"];
export type RemovalTarget = { deploymentId: string; invitationId: string; did: string };
export type RemovalReceipt = RemovalTarget & {
  id: string; idempotencyKey: string; state: string; message: string; requestedBy: string;
  createdAt: string; hostRequestedAt: string | null; deactivateAfter: string | null; completedAt: string | null;
  events: { state: string; message: string; createdAt: string; actorId: string | null }[];
};
export type RemovalStatus = RemovalReceipt | (RemovalTarget & { state: "not_requested" });
export function validRemoval(v: unknown, target: RemovalTarget): v is RemovalStatus {
  if (!object(v) || !uuid.test(target.deploymentId) || !uuid.test(target.invitationId) || !/^did:plc:[a-z2-7]{24}$/.test(target.did) ||
      v.deploymentId !== target.deploymentId || v.invitationId !== target.invitationId || v.did !== target.did) return false;
  if (v.state === "not_requested") return Object.keys(v).length === 4;
  if (Object.keys(v).some(k => !["id", "idempotencyKey", "state", "message", "requestedBy", "createdAt", "hostRequestedAt", "deactivateAfter", "completedAt", "events", "deploymentId", "invitationId", "did"].includes(k)) ||
      typeof v.id !== "string" || !uuid.test(v.id) || typeof v.idempotencyKey !== "string" || !uuid.test(v.idempotencyKey) || typeof v.requestedBy !== "string" || !uuid.test(v.requestedBy) ||
      typeof v.state !== "string" || !states.includes(v.state) || typeof v.message !== "string" || v.message.length > 1024 || !date(v.createdAt) || Date.parse(v.createdAt) > Date.now() + 60000) return false;
  const window = date(v.hostRequestedAt) && date(v.deactivateAfter) && Date.parse(v.deactivateAfter) - Date.parse(v.hostRequestedAt) === 30 * 86400000 && Date.parse(v.hostRequestedAt) <= Date.now() + 60000;
  if (!(window || (v.hostRequestedAt === null && v.deactivateAfter === null)) || (v.state === "requested" && v.hostRequestedAt !== null) || (["migration_window", "deactivated"].includes(v.state) && !window)) return false;
  if (v.state === "deactivated" ? !date(v.completedAt) || !date(v.deactivateAfter) || Date.parse(v.deactivateAfter) > Date.now() || Date.parse(v.completedAt) < Date.parse(v.deactivateAfter) || Date.parse(v.completedAt) > Date.now() + 60000 : v.completedAt !== null) return false;
  return Array.isArray(v.events) && v.events.length > 0 && v.events.length <= 100 && v.events.every(e => object(e) && typeof e.state === "string" && states.includes(e.state) && typeof e.message === "string" && e.message.length <= 1024 && date(e.createdAt) && (e.actorId === null || (typeof e.actorId === "string" && uuid.test(e.actorId))));
}
