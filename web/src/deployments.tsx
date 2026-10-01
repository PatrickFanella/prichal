import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';

type Event = { state: string; message: string; createdAt: string };
type Operation = { id: string; name: string; state: string; message: string; events: Event[] };
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function operation(value: unknown): value is Operation {
  return object(value) && typeof value.id === 'string' && typeof value.name === 'string' &&
    typeof value.state === 'string' && ['queued', 'configuring', 'checking', 'ready', 'failed'].includes(value.state) &&
    typeof value.message === 'string' && Array.isArray(value.events) && value.events.every(event =>
      object(event) && typeof event.state === 'string' && typeof event.message === 'string' && typeof event.createdAt === 'string');
}
export function Deployments({ organizationId, csrfToken }: { organizationId: string; csrfToken: string }) {
  const [operations, setOperations] = useState<Operation[]>([]);
  const storageKey = `pds-deployment-request:${organizationId}`;
  const [pending] = useState(() => {
    try {
      const raw = localStorage.getItem(storageKey);
      const value: unknown = raw ? JSON.parse(raw) : null;
      if (object(value) && typeof value.name === 'string' && typeof value.key === 'string') return { name: value.name, key: value.key };
    } catch { /* A blocked storage area still permits an in-memory request. */ }
    return null;
  });
  const [name, setName] = useState(pending?.name ?? '');
  const [key, setKey] = useState(() => pending?.key ?? crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [uncertain, setUncertain] = useState(pending !== null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const response = await fetch(`/api/organizations/${organizationId}/deployments`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]) });
        const result: unknown = await response.json();
        if (!response.ok || !Array.isArray(result) || !result.every(operation)) throw new Error('Invalid deployment list');
        if (!controller.signal.aborted) { setOperations(result); setLoaded(true); }
      } catch { if (!controller.signal.aborted) setError('Deployment status is temporarily unavailable.'); }
    }
    void load(); const timer = window.setInterval(() => { void load(); }, 3000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [organizationId]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError('');
    try { localStorage.setItem(storageKey, JSON.stringify({ name, key })); } catch { /* Keep the same key in memory. */ }
    try {
      const response = await fetch(`/api/organizations/${organizationId}/deployments`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ name, region: 'local', idempotencyKey: key }), signal: AbortSignal.timeout(5000) });
      const result: unknown = await response.json();
      if (!response.ok) {
        if (response.status >= 400 && response.status < 500) {
          try { localStorage.removeItem(storageKey); } catch { /* No request was accepted. */ }
          setUncertain(false); setKey(crypto.randomUUID());
        }
        setError(object(result) && typeof result.error === 'string' ? result.error : 'The deployment request could not be accepted.'); return; }
      if (!operation(result)) throw new Error('Invalid operation');
      setOperations(current => [...current.filter(item => item.id !== result.id), result]);
      try { localStorage.removeItem(storageKey); } catch { /* The committed operation is also listed by the API. */ }
      setName(''); setKey(crypto.randomUUID()); setUncertain(false);
    } catch { setUncertain(true); setError('The request could not be confirmed. Retry to resume the same operation.'); }
    finally { setBusy(false); }
  }
  return <div className="deployments">
    <h4>Servers</h4><p>Local deployment test. This does not create a live PDS.</p>
    {error && <p role="alert">{error}</p>}
    {!loaded && <p role="status">Loading deployments...</p>}
    <ul>{operations.map(item => <li key={item.id}><div><strong>{item.name}</strong><p role="status">{item.state}: {item.message}</p><details><summary>Progress history</summary><ol>{item.events.map((event, index) => <li key={index}>{event.state}: {event.message}</li>)}</ol></details></div></li>)}</ul>
    <form onSubmit={event => { void submit(event); }}>
      <label htmlFor={`server-${organizationId}`}>Server name</label>
      <div className="form-row"><input id={`server-${organizationId}`} value={name} onChange={event => setName(event.target.value)} disabled={busy || uncertain} pattern="[a-z][a-z0-9-]*" maxLength={63} required placeholder="my-studio"/><button disabled={busy || !name} type="submit">{busy ? 'Submitting...' : uncertain ? 'Retry request' : 'Run deployment test'}</button></div>
    </form>
  </div>;
}
