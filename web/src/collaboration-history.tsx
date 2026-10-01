import { useEffect, useRef, useState } from 'react';

type Audit = { id: string; actorId: string; targetId: string | null; action: string; role: 'administrator' | 'viewer' | null; createdAt: string };
type Page = { items: Audit[]; hasMore: boolean; nextBefore: string | null };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const actions = ['invite', 'accept', 'expire', 'revoke', 'role', 'rename', 'transfer_offer', 'transfer_accept'];
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function identity(value: unknown): value is string { return typeof value === 'string' && uuid.test(value); }
function page(value: unknown): value is Page {
  return object(value) && Array.isArray(value.items) && value.items.length <= 50 && value.items.every(item => object(item) && typeof item.id === 'string' && /^[1-9][0-9]{0,18}$/.test(item.id) && identity(item.actorId) && (item.targetId === null || identity(item.targetId)) && typeof item.action === 'string' && actions.includes(item.action) && (item.role === null || item.role === 'administrator' || item.role === 'viewer') && typeof item.createdAt === 'string' && Number.isFinite(Date.parse(item.createdAt))) && typeof value.hasMore === 'boolean' && (value.hasMore ? typeof value.nextBefore === 'string' && value.nextBefore.length > 0 && value.nextBefore.length <= 128 : value.nextBefore === null);
}
export function CollaborationHistory({ organizationId }: { organizationId: string }) {
  const [history, setHistory] = useState<Page | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (history) status.current?.focus(); }, [history]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { setHistory(null); setError(''); setBusy(false); return () => { active.current?.abort(); active.current = null; }; }, [organizationId]);
  async function load(before?: string) {
    active.current?.abort(); const controller = new AbortController(); active.current = controller;
    setBusy(true); setError('');
    try {
      const response = await fetch(`/api/team/organizations/${organizationId}/audit${before ? `?before=${encodeURIComponent(before)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (active.current !== controller) return;
      if (!response.ok || !page(value)) { setHistory(null); throw new Error('History unavailable'); }
      setHistory(value);
    } catch { if (active.current === controller) setError('Organization access history could not be confirmed. Refresh current access and load the latest page again.'); }
    finally { if (active.current === controller) setBusy(false); }
  }
  return <details aria-busy={busy}><summary>Organization access history</summary><p>Recorded changes do not grant current access. Each page checks current organization ownership.</p>
    <button type="button" disabled={busy} onClick={() => { void load(); }}>Load latest access history</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {history && <><p role="status" ref={status} tabIndex={-1}>{history.items.length} access changes shown.</p><ol>{history.items.map(event => <li key={event.id}><time dateTime={event.createdAt}>{new Date(event.createdAt).toLocaleString()}</time>: {event.action.replaceAll('_', ' ')}. Actor <code>{event.actorId}</code>. {event.targetId && <>Target <code>{event.targetId}</code>.</>} {event.role && <>Role {event.role}.</>}</li>)}</ol>{history.items.length === 0 && <p>No access changes recorded.</p>}{history.hasMore && history.nextBefore && <button type="button" disabled={busy} onClick={() => { void load(history.nextBefore!); }}>Load older access history</button>}</>}
  </details>;
}
