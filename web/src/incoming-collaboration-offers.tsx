import { useEffect, useRef, useState } from 'react';

type Offer = { id: string; organizationId: string; role?: 'viewer' | 'administrator'; expiresAt: string };
type Page = { items: Offer[]; hasMore: boolean; nextAfter: string | null };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null; }
function identity(v: unknown): v is string { return typeof v === 'string' && uuid.test(v); }
function page(v: unknown, kind: 'invitations' | 'transfers'): v is Page {
  return object(v) && Array.isArray(v.items) && v.items.length <= 50 && v.items.every(item => object(item) && identity(item.id) && identity(item.organizationId) && typeof item.expiresAt === 'string' && Number.isFinite(Date.parse(item.expiresAt)) && (kind === 'invitations' ? item.role === 'viewer' || item.role === 'administrator' : item.role === undefined)) && typeof v.hasMore === 'boolean' && (v.hasMore ? typeof v.nextAfter === 'string' && v.nextAfter.length > 0 && v.nextAfter.length <= 320 : v.nextAfter === null);
}
export function IncomingCollaborationOffers({ kind, initial, working, mutationBlocked, beforeRead, onReview }: { kind: 'invitations' | 'transfers'; initial: Page; working: boolean; mutationBlocked: boolean; beforeRead: () => void; onReview: (offer: Offer) => void }) {
  const [records, setRecords] = useState<Page | null>(initial), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null), focusResult = useRef(false);
  useEffect(() => { if (records && focusResult.current) { status.current?.focus(); focusResult.current = false; } }, [records]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { active.current?.abort(); active.current = null; setRecords(initial); setError(''); setBusy(false); focusResult.current = false; }, [initial]);
  useEffect(() => () => { active.current?.abort(); active.current = null; }, []);
  const name = kind === 'invitations' ? 'invitations' : 'ownership offers';
  async function load(after?: string) {
    beforeRead(); active.current?.abort(); const controller = new AbortController(); active.current = controller; setBusy(true); setError('');
    try {
      const response = await fetch(`/api/team/incoming/${kind}/page${after ? `?after=${encodeURIComponent(after)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (active.current !== controller) return;
      if (!response.ok || !page(value, kind)) throw new Error('Offers unavailable');
      focusResult.current = true; setRecords(value);
    } catch { if (active.current === controller) { setRecords(null); setError('Incoming offers could not be confirmed. Refresh collaboration access before loading again.'); } }
    finally { if (active.current === controller) setBusy(false); }
  }
  return <section aria-label={`Incoming ${name}`} aria-busy={busy}><h4>Incoming {name}</h4><p>Each page checks your current verified identity and offer eligibility. Acceptance requires separate review and rechecks current permission, expiry and organization eligibility.</p>
    <button type="button" disabled={busy || working} onClick={() => { void load(); }}>Load first {name} page</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {records && <><p role="status" ref={status} tabIndex={-1}>{records.items.length} incoming {name} shown.</p><ul>{records.items.map(item => <li key={item.id}><p>Organization {item.organizationId}: {kind === 'invitations' ? item.role : 'empty-organization ownership'}. Expires <time dateTime={item.expiresAt}>{new Date(item.expiresAt).toLocaleString()}</time>.</p><button type="button" disabled={working || busy || mutationBlocked} onClick={() => onReview(item)}>{kind === 'invitations' ? 'Review invitation acceptance' : 'Review ownership acceptance'}</button></li>)}</ul>{records.items.length === 0 && <p>No pending {name}.</p>}{records.hasMore && records.nextAfter && <button type="button" disabled={busy || working} onClick={() => { void load(records.nextAfter!); }}>Load more {name}</button>}</>}
  </section>;
}
export function incomingOfferPage(value: unknown, kind: 'invitations' | 'transfers'): value is Page { return page(value, kind); }
