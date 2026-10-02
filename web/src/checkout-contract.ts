export type CheckoutIntent = { key: string; priceId: string };
export type CheckoutPlan = { priceId: string; fixture: true; canPrepare: boolean };
export type CheckoutReceipt = { id: string; organizationId: string; idempotencyKey: string; priceId: string; state: 'prepared' | 'queued' | 'customer_ready' | 'session_ready' | 'manual_review'; createdAt: string; canPay: false };
export type CheckoutNavigation = { status: 'open' | 'complete' | 'expired'; expiresAt: string; url?: string };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const price = /^price_[A-Za-z0-9]{1,100}$/;
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null && !Array.isArray(v); }
function date(v: unknown): v is string { return typeof v === 'string' && v.length <= 40 && Number.isFinite(Date.parse(v)); }
export function checkoutIntent(v: unknown): v is CheckoutIntent {
 return object(v) && Object.keys(v).length === 2 && typeof v.key === 'string' && uuid.test(v.key) && typeof v.priceId === 'string' && price.test(v.priceId);
}
export function checkoutPlan(v: unknown): v is CheckoutPlan {
 return object(v) && Object.keys(v).length === 3 && typeof v.priceId === 'string' && price.test(v.priceId) && v.fixture === true && typeof v.canPrepare === 'boolean';
}
export function checkoutReceipt(v: unknown, org: string, intent: CheckoutIntent): v is CheckoutReceipt {
 return object(v) && Object.keys(v).length === 7 && typeof v.id === 'string' && uuid.test(v.id) && v.organizationId === org && v.idempotencyKey === intent.key && v.priceId === intent.priceId && ['prepared','queued','customer_ready','session_ready','manual_review'].includes(String(v.state)) && date(v.createdAt) && v.canPay === false;
}
export function checkoutNavigation(v: unknown): v is CheckoutNavigation {
 if (!object(v) || !date(v.expiresAt)) return false;
 if (v.status === 'complete' || v.status === 'expired') return Object.keys(v).length === 2 && v.url === undefined;
 if (v.status !== 'open' || Object.keys(v).length !== 3 || typeof v.url !== 'string' || Date.parse(v.expiresAt) <= Date.now() || Date.parse(v.expiresAt) > Date.now() + 24 * 3600000 + 60000) return false;
 try {
  const u = new URL(v.url);
  return u.protocol === 'http:' && !v.url.includes('?') && !v.url.includes('#') && (u.hostname === '[::1]' || /^127\.\d+\.\d+\.\d+$/.test(u.hostname)) && !u.username && !u.password && !u.search && !u.hash && u.href === v.url && /^\/checkout\/cs_test_[A-Za-z0-9]{1,100}$/.test(u.pathname);
 } catch { return false; }
}
