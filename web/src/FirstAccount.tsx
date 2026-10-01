import { HandleSetup } from './HandleSetup';
import { useEffect, useState } from 'react';

type Enrollment = {
  id: string; deploymentId: string; state: string; message: string; code?: string; expiresAt: string | null;
  accountUrl: string; did?: string; handle?: string; emailConfirmed: boolean;
};
type Request = { idempotencyKey: string; recover: boolean };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === 'object'; }
function valid(value: unknown, deploymentId: string): value is Enrollment {
  if (!object(value) || typeof value.id !== 'string' || !uuid.test(value.id) || value.deploymentId !== deploymentId ||
    typeof value.state !== 'string' || !['pending', 'invitation_ready', 'account_created', 'verified', 'expired', 'failed', 'superseded'].includes(value.state) ||
    !(value.did === undefined || typeof value.did === 'string') || !(value.handle === undefined || typeof value.handle === 'string') || typeof value.message !== 'string' || typeof value.accountUrl !== 'string' || typeof value.emailConfirmed !== 'boolean' ||
    !(value.expiresAt === null || (typeof value.expiresAt === 'string' && Number.isFinite(Date.parse(value.expiresAt)))) ||
    !(value.code === undefined || (typeof value.code === 'string' && /^[A-Za-z0-9._:-]{1,256}$/.test(value.code)))) return false;
  try {
    const url = new URL(value.accountUrl);
    return url.protocol === 'https:' && url.hostname === `${deploymentId}.localhost` && url.port === String(36000 + parseInt(deploymentId.slice(0, 4), 16) % 10000) && url.pathname === '/account' && !url.username && !url.password && !url.search && !url.hash;
  } catch { return false; }
}
export function FirstAccount({ deploymentId, csrfToken }: { deploymentId: string; csrfToken: string }) {
  const storageKey = `pds-enrollment-request:${deploymentId}`;
  const [operation, setOperation] = useState<Enrollment | null>(null);
  const [request, setRequest] = useState<Request | null>(() => {
    try { const value: unknown = JSON.parse(localStorage.getItem(storageKey) ?? 'null'); return object(value) && typeof value.idempotencyKey === 'string' && uuid.test(value.idempotencyKey) && typeof value.recover === 'boolean' ? { idempotencyKey: value.idempotencyKey, recover: value.recover } : null; } catch { return null; }
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loadError, setLoadError] = useState('');
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      try {
        const response = await fetch(`/api/deployments/${deploymentId}/enrollment`, { signal: controller.signal });
        if (response.status === 404) { setLoadError(''); return; }
        if (response.status === 401 || response.status === 403) { setOperation(null); setLoadError('Sign in again to view account status.'); return; }
        const value: unknown = await response.json();
        if (!response.ok || !valid(value, deploymentId)) throw new Error('Enrollment unavailable');
        setOperation(value); setLoadError('');
      } catch { if (!controller.signal.aborted) setLoadError('Account status is temporarily unavailable. Refresh to try again.'); }
    };
    void load();
    const interval = window.setInterval(() => { setNow(Date.now()); void load(); }, 3000);
    return () => { controller.abort(); window.clearInterval(interval); };
  }, [deploymentId]);
  async function prepare(recover: boolean) {
    setBusy(true); setError(''); setNotice('');
    const pending = request ?? { idempotencyKey: crypto.randomUUID(), recover };
    setRequest(pending);
    try { localStorage.setItem(storageKey, JSON.stringify(pending)); } catch { /* Keep the stable retry key in memory. */ }
    try {
      const response = await fetch(`/api/deployments/${deploymentId}/enrollment`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify(pending) });
      const value: unknown = await response.json();
      if (!response.ok || !valid(value, deploymentId)) { if (object(value) && typeof value.error === 'string') setError(value.error); else setError('Invitation could not be confirmed. Retry the same request.'); return; }
      setOperation(value); setRequest(null);
      try { localStorage.removeItem(storageKey); } catch { /* The operation is durable on the service. */ }
    } catch { setError('Invitation could not be confirmed. Retry the same request.'); }
    finally { setBusy(false); }
  }
  async function refresh() {
    if (!operation) return;
    setBusy(true); setError(''); setNotice('');
    try {
      const response = await fetch(`/api/enrollments/${operation.id}/refresh`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}' });
      const value: unknown = await response.json();
      if (!response.ok || !valid(value, deploymentId)) { setError(object(value) && typeof value.error === 'string' ? value.error : 'Account status refresh could not be confirmed.'); return; }
      setOperation(value); setNotice('Account status check requested.');
    } catch { setError('Account status refresh could not be confirmed.'); }
    finally { setBusy(false); }
  }
  const expired = operation?.state === 'invitation_ready' && (!operation.expiresAt || Date.parse(operation.expiresAt) <= now);
  const state = expired ? 'expired' : operation?.state;
  return <section className="first-account" aria-label="First AT account">
    <h4>First account</h4>
    <p>Create your account directly on your PDS. Your account password stays with the PDS.</p>
    <p>For this local test, use a .test handle with 3–18 characters before the suffix and a .invalid email address. Confirmation mail stays in the test inbox.</p>
    {operation && <p role="status">{state}: {expired ? 'This invitation expired. Request recovery to replace it.' : operation.message}</p>}
    {state === 'invitation_ready' && operation?.code && <>
      <p>One-use invitation: <code>{operation.code}</code></p>
      <p>Expires {operation.expiresAt && new Date(operation.expiresAt).toLocaleString()}.</p>
      <button type="button" onClick={() => { if (!navigator.clipboard) { setError('Copy unavailable. Select the invitation text.'); return; } void navigator.clipboard.writeText(operation.code ?? '').then(() => setNotice('Invitation copied.'), () => setError('Copy unavailable. Select the invitation text.')); }}>Copy invitation</button>
      <p><a href={operation.accountUrl} target="_blank" rel="noreferrer">Open account creation on your PDS</a></p>
    </>}
    {(state === 'account_created' || state === 'verified') && <p>{operation?.handle}. {operation?.emailConfirmed ? 'Email confirmed.' : 'Email confirmation pending.'}</p>}
    {(!operation || state === 'expired' || state === 'failed' || state === 'superseded' || request) && <button type="button" disabled={busy} onClick={() => { void prepare(!!operation); }}>{busy ? 'Preparing invitation…' : request ? 'Retry invitation request' : operation ? 'Recover invitation' : 'Prepare first-account invitation'}</button>}
    {operation && <button type="button" disabled={busy || state === 'pending' || state === 'superseded' || state === 'failed'} onClick={() => { void refresh(); }}>Check account status</button>}
    {state === 'verified' && <HandleSetup deploymentId={deploymentId} csrfToken={csrfToken} />}
    {notice && <p role="status">{notice}</p>}{(error || loadError) && <p role="alert">{error || loadError}</p>}
  </section>;
}
