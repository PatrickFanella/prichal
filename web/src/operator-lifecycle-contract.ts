export type OperatorIntent = { key: string; instance: string; kind: 'service' | 'account'; state: 'running' | 'suspended'; reason: 'maintenance' | 'safety_review' | 'resource_pressure' };
export type OperatorOperation = { id: string; deploymentId: string; generation: number; kind: OperatorIntent['kind']; desiredState: OperatorIntent['state']; priorState: 'running' | 'suspended' | 'unknown'; reason: OperatorIntent['reason']; state: 'pending' | 'applied' | 'failed'; updatedAt: string };
export type OperatorReceipt = OperatorOperation & { idempotencyKey: string };
export type ReceiptState = 'unknown' | 'absent' | 'pending' | 'terminal';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value); }
export function operatorIntent(value: unknown): value is OperatorIntent {
  return object(value) && Object.keys(value).length === 5 && typeof value.key === 'string' && uuid.test(value.key) && typeof value.instance === 'string' && uuid.test(value.instance) && ['service', 'account'].includes(value.kind as string) && ['running', 'suspended'].includes(value.state as string) && ['maintenance', 'safety_review', 'resource_pressure'].includes(value.reason as string);
}
export function operatorOperation(value: unknown, intent: OperatorIntent): value is OperatorOperation {
  return object(value) && typeof value.id === 'string' && uuid.test(value.id) && value.deploymentId === intent.instance && value.kind === intent.kind && value.desiredState === intent.state && value.reason === intent.reason && typeof value.generation === 'number' && Number.isSafeInteger(value.generation) && value.generation > 0 && ['running', 'suspended', 'unknown'].includes(value.priorState as string) && ['pending', 'applied', 'failed'].includes(value.state as string) && typeof value.updatedAt === 'string' && Number.isFinite(Date.parse(value.updatedAt));
}
export function operatorReceipt(value: unknown, intent: OperatorIntent): value is OperatorReceipt {
  return object(value) && value.idempotencyKey === intent.key && operatorOperation(value, intent);
}
