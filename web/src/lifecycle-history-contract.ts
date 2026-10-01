export type HistoryItem = { id: string; kind: 'service' | 'account'; generation: number; state: 'pending' | 'applied' | 'failed'; desiredState: 'running' | 'suspended'; reason: string; actor: string };
export type History = { items: HistoryItem[]; hasMore: boolean; nextBefore: number | null };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value); }
export function lifecycleHistory(value: unknown, operator: boolean, before?: number): value is History {
  if (!object(value) || !Array.isArray(value.items) || value.items.length > 50 || typeof value.hasMore !== 'boolean') return false;
  let prior = before ?? Infinity;
  const ids = new Set<string>();
  for (const item of value.items) {
    if (!object(item) || typeof item.id !== 'string' || !uuid.test(item.id) || ids.has(item.id) || !['service', 'account'].includes(item.kind as string) || !['pending', 'applied', 'failed'].includes(item.state as string) || !['running', 'suspended'].includes(item.desiredState as string) || !['maintenance', 'safety_review', 'resource_pressure'].includes(item.reason as string) || typeof item.generation !== 'number' || !Number.isSafeInteger(item.generation) || item.generation < 1 || item.generation >= prior || typeof item.actor !== 'string' || item.actor.length < 1 || item.actor.length > 256 || !operator && item.actor !== 'operator') return false;
    ids.add(item.id); prior = item.generation;
  }
  return value.hasMore ? value.items.length === 50 && value.nextBefore === prior : value.nextBefore === null;
}
