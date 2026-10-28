import { useEffect, useRef, useState } from 'react';

type Event = { key: string; category: string; state: string; createdAt: string; message: string; nextStep: string };
type Page = { items: Event[]; hasMore: boolean; nextBefore: string | null };
const categories = ['deployment', 'enrollment', 'domain', 'service', 'account', 'export', 'exit'];
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function page(value: unknown): value is Page {
  return object(value) && Array.isArray(value.items) && value.items.length <= 50 && value.items.every(item => object(item) && typeof item.key === 'string' && /^(deployment|enrollment|domain|service|account|export|exit):[1-9][0-9]*$/.test(item.key) && typeof item.category === 'string' && categories.includes(item.category) && typeof item.state === 'string' && typeof item.createdAt === 'string' && Number.isFinite(Date.parse(item.createdAt)) && typeof item.message === 'string' && typeof item.nextStep === 'string') && typeof value.hasMore === 'boolean' && (value.hasMore ? typeof value.nextBefore === 'string' && value.nextBefore.length <= 256 : value.nextBefore === null);
}

export function OwnerJourney({ deploymentId }: { deploymentId: string }) {
  const [history, setHistory] = useState<Page | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null);
  const pageStatus = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null), loading = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (history) pageStatus.current?.focus(); }, [history]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { if (busy) loading.current?.focus(); }, [busy]);
  useEffect(() => { setHistory(null); setError(''); setBusy(false); return () => { active.current?.abort(); active.current = null; }; }, [deploymentId]);
  async function load(before?: string) {
    active.current?.abort();
    const controller = new AbortController(); active.current = controller;
    setBusy(true); setError(''); setHistory(null);
    try {
      const response = await fetch(`/api/deployments/${deploymentId}/journey${before ? `?before=${encodeURIComponent(before)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (!response.ok || !page(value)) throw new Error('Hosting history is temporarily unavailable.');
      if (active.current === controller) setHistory(value);
    } catch { if (active.current === controller) setError('Hosting history is temporarily unavailable. Try loading the latest history again.'); }
    finally { if (active.current === controller) setBusy(false); }
  }
  return <details><summary>Hosting history and next steps</summary>
    <p>This is recorded history. Open the current controls to check availability, dates and independent holds before acting.</p>
    <button type="button" disabled={busy} onClick={() => { void load(); }}>Load latest hosting history</button>
    {busy && <p role="status" ref={loading} tabIndex={-1}>Loading hosting history...</p>}
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {history && <><p role="status" ref={pageStatus} tabIndex={-1}>{history.items.length} recorded events shown.</p><ol>{history.items.map(item => <li key={item.key}>
      <p><time dateTime={item.createdAt}>{new Date(item.createdAt).toLocaleString()}</time>: {item.message}</p><p>{item.nextStep}</p>
    </li>)}</ol>{history.items.length === 0 && <p>No hosting events recorded.</p>}{history.hasMore && history.nextBefore && <button type="button" disabled={busy} onClick={() => { void load(history.nextBefore!); }}>Load older hosting history</button>}</>}
  </details>;
}
