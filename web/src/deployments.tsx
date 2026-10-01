import { useEffect, useState } from 'react';
import { FirstAccount } from './FirstAccount';
import { HostingEntitlement } from './hosting-entitlement';
import type { FormEvent } from 'react';

type Event = { state: string; message: string; createdAt: string };
type Operation = { usage: {state: string; checkedAt: string | null; dataBytes: number; blobBytes: number; accounts: number; storageLimit: string}; service: { state: string; lifecycle: string; reason: string; account: string; accountReason: string; checkedAt: string | null; backup: string }; endpoint: string; managedHostname: string; kind: 'test' | 'agent'; id: string; name: string; state: string; message: string; events: Event[] };
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function localEndpoint(value: unknown, hostname: unknown): boolean {
  if (value === '') return true;
  if (typeof value !== 'string' || typeof hostname !== 'string') return false;
  try { const endpoint = new URL(value); return endpoint.protocol === 'https:' && endpoint.hostname === hostname && /^[a-f0-9-]{36}\.localhost$/.test(hostname) && Number(endpoint.port) >= 36000 && Number(endpoint.port) < 46000 && !endpoint.username && !endpoint.password && endpoint.pathname === '/' && !endpoint.search && !endpoint.hash; } catch { return false; }
}
function operation(value: unknown): value is Operation {
  return object(value) && typeof value.endpoint === 'string' && typeof value.managedHostname === 'string' && localEndpoint(value.endpoint, value.managedHostname) && (value.kind === 'test' || value.kind === 'agent') && typeof value.id === 'string' && typeof value.name === 'string' &&
    typeof value.state === 'string' && ['queued', 'configuring', 'checking', 'ready', 'failed'].includes(value.state) &&
    object(value.usage) && ['unknown','observed','unavailable','stale'].includes(String(value.usage.state)) && [value.usage.dataBytes,value.usage.blobBytes,value.usage.accounts].every(metric=>typeof metric==='number' && Number.isSafeInteger(metric) && metric>=0) && (value.usage.checkedAt===null || typeof value.usage.checkedAt==='string') && value.usage.storageLimit==='unconfigured' && object(value.service) && typeof value.service.state === 'string' && ['unknown', 'healthy', 'degraded', 'stale','suspended'].includes(value.service.state) && (value.service.checkedAt === null || typeof value.service.checkedAt === 'string') && value.service.backup === 'unconfigured' && ['running','suspended','pending','failed'].includes(String(value.service.lifecycle)) && typeof value.service.reason==='string' && ['running','suspended','pending','failed'].includes(String(value.service.account)) && typeof value.service.accountReason==='string' && typeof value.message === 'string' && Array.isArray(value.events) && value.events.every(event =>
      object(event) && typeof event.state === 'string' && typeof event.message === 'string' && typeof event.createdAt === 'string');
}
export function Deployments({ organizationId, csrfToken }: { organizationId: string; csrfToken: string }) {
  const [operations, setOperations] = useState<Operation[]>([]);
  const storageKey = `pds-deployment-request:${organizationId}`;
  const [pending] = useState(() => {
    try {
      const raw = localStorage.getItem(storageKey);
      const value: unknown = raw ? JSON.parse(raw) : null;
      if (object(value) && typeof value.name === 'string' && typeof value.key === 'string') return { name: value.name, key: value.key, kind: value.kind === 'agent' ? 'agent' as const : 'test' as const };
    } catch { /* A blocked storage area still permits an in-memory request. */ }
    return null;
  });
  const [kind, setKind] = useState<'test' | 'agent'>(pending?.kind ?? 'test');
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
    try { localStorage.setItem(storageKey, JSON.stringify({ name, key, kind })); } catch { /* Keep the same key in memory. */ }
    try {
      const response = await fetch(`/api/organizations/${organizationId}/deployments`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ kind, name, region: 'local', idempotencyKey: key }), signal: AbortSignal.timeout(5000) });
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
    <h4>Servers</h4><p>Local qualification. Public hosting is unavailable.</p>
    <HostingEntitlement organizationId={organizationId}/>
    {error && <p role="alert">{error}</p>}
    {!loaded && <p role="status">Loading deployments...</p>}
    <ul>{operations.map(item => <li key={item.id}><div><strong>{item.name}</strong><p>{item.kind === 'agent' ? 'Local PDS runtime' : 'Deployment simulation'}</p>{item.endpoint && (item.state === 'checking' || item.state === 'ready') && <p><a href={item.endpoint + '/xrpc/_health'} target="_blank" rel="noreferrer">Open local PDS health</a></p>}<p role="status">{item.state}: {item.message}</p>{item.kind === 'agent' && <p>Local service observation: {item.service?.state ?? 'unknown'}.{item.service?.checkedAt && <> Checked {new Date(item.service.checkedAt).toLocaleString()}.</>} Scheduled backups are not configured.</p>}{item.kind === 'agent' && <p>Storage observation: {item.usage.state}. {item.usage.checkedAt && <>{((item.usage.dataBytes + item.usage.blobBytes) / 1048576).toFixed(2)} MiB, {item.usage.accounts} account. Checked {new Date(item.usage.checkedAt).toLocaleString()}.</>} Total storage limits are not configured.</p>}{item.kind === 'agent' && item.service.account!=='running' && <p role="status">Account moderation: {item.service.account}. Reason: {item.service.accountReason.replaceAll('_',' ')}. Contact support for an update.</p>}{item.kind === 'agent' && item.service.lifecycle!=='running' && <p role="status">Service lifecycle: {item.service.lifecycle}. Reason: {item.service.reason.replaceAll('_',' ')}. Contact support for an update.</p>}{item.kind === 'agent' && item.state === 'ready' && item.service.lifecycle==='running' && item.service.account==='running' && <FirstAccount deploymentId={item.id} csrfToken={csrfToken} />}<details><summary>Progress history</summary><ol>{item.events.map((event, index) => <li key={index}>{event.state}: {event.message}</li>)}</ol></details></div></li>)}</ul>
    <form onSubmit={event => { void submit(event); }}>
      <label htmlFor={`kind-${organizationId}`}>Deployment mode</label>
      <select id={`kind-${organizationId}`} value={kind} disabled={busy || uncertain} onChange={event => setKind(event.target.value === 'agent' ? 'agent' : 'test')}><option value="test">Simulate deployment</option><option value="agent">Create a local PDS</option></select>
      <label htmlFor={`server-${organizationId}`}>Server name</label>
      <div className="form-row"><input id={`server-${organizationId}`} value={name} onChange={event => setName(event.target.value)} disabled={busy || uncertain} pattern="[a-z][a-z0-9-]*" maxLength={63} required placeholder="my-studio"/><button disabled={busy || !name} type="submit">{busy ? 'Submitting...' : uncertain ? 'Retry request' : kind === 'agent' ? 'Create local PDS' : 'Run deployment test'}</button></div>
    </form>
  </div>;
}
