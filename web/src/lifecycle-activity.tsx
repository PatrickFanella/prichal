import { useEffect, useRef, useState } from 'react';
import { lifecycleHistory, type History } from './lifecycle-history-contract';

export function LifecycleActivity({ deploymentId, operator = false }: { deploymentId: string; operator?: boolean }) {
  const [page, setPage] = useState<History | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const read = useRef<AbortController | null>(null), busyRef = useRef(false);
  const pageStatus = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => {
    setPage(null); setError(''); setBusy(false); busyRef.current = false;
    return () => { read.current?.abort(); read.current = null; busyRef.current = false; };
  }, [deploymentId, operator]);
  useEffect(() => { if (page) pageStatus.current?.focus(); }, [page]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  async function load(before?: number) {
    if (busyRef.current) return;
    busyRef.current = true; read.current?.abort();
    const controller = new AbortController(); read.current = controller;
    setBusy(true); setError(''); setPage(null);
    try {
      const response = await fetch(`/api/${operator ? 'operator/' : ''}deployments/${deploymentId}/activity${before ? `?before=${before}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (read.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !lifecycleHistory(value, operator, before)) throw Error('Unconfirmed history');
      setPage(value);
    } catch {
      if (read.current === controller && !controller.signal.aborted) { setPage(null); setError('Activity could not be confirmed. Refresh access before loading current history.'); }
    } finally {
      if (read.current === controller) { read.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); }
    }
  }
  return <details className="lifecycle-activity" aria-busy={busy}><summary>Service and account activity</summary>
    <button type="button" disabled={busy} onClick={() => { void load(); }}>Load latest activity</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {page && <><p role="status" ref={pageStatus} tabIndex={-1}>{page.items.length} service and account actions shown.</p>
      <ol>{page.items.map(item => <li key={item.id}>Generation {item.generation}: {item.kind} {item.desiredState}, {item.state}. Reason: {item.reason.replaceAll('_', ' ')}. {operator && <>Actor: {item.actor}.</>}</li>)}</ol>
      {page.items.length === 0 && <p>No service or account actions recorded.</p>}
      {page.hasMore && page.nextBefore !== null && <button type="button" disabled={busy} onClick={() => { void load(page.nextBefore!); }}>Load older activity</button>}
    </>}
  </details>;
}
