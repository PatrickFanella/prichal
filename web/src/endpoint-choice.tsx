import { useEffect, useRef, useState } from 'react';
import { EndpointDNSObservation } from './endpoint-dns-observation';

type Plan = { hostname: string; applicationHosts: string[]; state: 'planning_only' | 'existing_native_requires_review' };
type Choice = { id: string; organizationId: string; hostname: string; applicationHosts: string[]; state: 'awaiting_ownership' | 'cancelled'; proofName: string; proofValue: string; createdAt: string; expiresAt: string; canActivate: false };
type Intent = { kind: 'save'; key: string; hostname: string; applicationHosts: string[] } | { kind: 'cancel'; key: string; id: string; hostname: string };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null; }
function host(v: unknown): v is string { return typeof v === 'string' && v.length >= 3 && v.length <= 244 && /^[a-z0-9.-]+$/.test(v); }
function apps(v: unknown): v is string[] { return Array.isArray(v) && v.length <= 8 && v.every(x => typeof x === 'string' && x.length <= 253 && /^[a-z0-9.-]+$/.test(x)); }
function intent(v: unknown): Intent | null {
  if (!object(v) || typeof v.key !== 'string' || !uuid.test(v.key) || !host(v.hostname)) return null;
  if (v.kind === 'save' && apps(v.applicationHosts)) return { kind: 'save', key: v.key, hostname: v.hostname, applicationHosts: v.applicationHosts };
  if (v.kind === 'cancel' && typeof v.id === 'string' && uuid.test(v.id)) return { kind: 'cancel', key: v.key, id: v.id, hostname: v.hostname };
  return null;
}
function choice(v: unknown, org: string): v is Choice {
  return object(v) && typeof v.id === 'string' && uuid.test(v.id) && v.organizationId === org && host(v.hostname) && apps(v.applicationHosts) && (v.state === 'awaiting_ownership' || v.state === 'cancelled') && v.proofName === `_pdsaas.${v.hostname}` && typeof v.proofValue === 'string' && /^pdsaas-domain-v1=[a-zA-Z0-9_-]{43}$/.test(v.proofValue) && typeof v.createdAt === 'string' && Number.isFinite(Date.parse(v.createdAt)) && typeof v.expiresAt === 'string' && Number.isFinite(Date.parse(v.expiresAt)) && v.canActivate === false;
}
function matches(c: Choice, retained: Intent): boolean {
  return retained.kind === 'cancel' ? c.id === retained.id && c.hostname === retained.hostname && c.state === 'cancelled' : c.hostname === retained.hostname && JSON.stringify(c.applicationHosts) === JSON.stringify(retained.applicationHosts);
}

export function EndpointChoice({ organizationId, ownerId, csrfToken, plan }: { organizationId: string; ownerId: string; csrfToken: string; plan: Plan | null }) {
  const storageKey = `pds-endpoint-intent:${ownerId}:${organizationId}`;
  const [current, setCurrent] = useState<Choice | null>(null), [known, setKnown] = useState(false), [loading, setLoading] = useState(false), [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<Intent | null>(null), [blocked, setBlocked] = useState(false), [reviewed, setReviewed] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const read = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null), reviewHeading = useRef<HTMLHeadingElement | null>(null), result = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (pending || blocked) { const panel = reviewHeading.current?.closest('details'); if (panel) panel.open = true; reviewHeading.current?.focus(); } }, [pending, blocked]);
  useEffect(() => { if (notice) result.current?.focus(); }, [notice]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { setReviewed(false); }, [plan]);
  async function refresh(explicit = true) {
    read.current?.abort(); const controller = new AbortController(); read.current = controller;
    setLoading(true); setKnown(false); setCurrent(null); setReviewed(false); setError(''); setNotice('');
    try {
      const response = await fetch(`/api/organizations/${organizationId}/endpoint-choice`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (read.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !object(value) || !(value.choice === null || choice(value.choice, organizationId) && value.choice.state === 'awaiting_ownership')) throw new Error('Invalid current choice');
      setCurrent(value.choice as Choice | null); setKnown(true);
      if (explicit) setNotice(value.choice === null ? 'No pending address choice was found.' : 'The current unverified address choice was loaded.');
    } catch { if (read.current === controller && !controller.signal.aborted) setError('The current address choice could not be confirmed. Refresh organization access and retry this read.'); }
    finally { if (read.current === controller && !controller.signal.aborted) setLoading(false); }
  }
  useEffect(() => {
    try { const raw = sessionStorage.getItem(storageKey); if (raw) { const saved = intent(JSON.parse(raw)); if (!saved) throw new Error('Invalid intent'); setPending(saved); } }
    catch { setBlocked(true); }
    void refresh(false);
    return () => { read.current?.abort(); read.current = null; mutation.current?.abort(); mutation.current = null; };
  }, [storageKey]);
  function stage(value: Intent) {
    if (!known || loading || busy || pending || blocked) return;
    try { sessionStorage.setItem(storageKey, JSON.stringify(value)); setPending(value); setReviewed(false); setError(''); setNotice(''); }
    catch { setError('Enable session storage before reviewing an address request.'); }
  }
  function clear() {
    if (!known || !reviewed || busy || loading) return;
    try { sessionStorage.removeItem(storageKey); setPending(null); setBlocked(false); setReviewed(false); setError(''); setNotice('The browser request was cleared. Any saved server-side choice remains until separately cancelled.'); }
    catch { setError('The retained browser request could not be cleared.'); }
  }
  async function submit() {
    if (!pending || !reviewed || busy || loading || !known) return;
    const retained = pending, controller = new AbortController(); mutation.current = controller; read.current?.abort(); read.current = null; setBusy(true); setError(''); setNotice('');
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(retained));
      const path = `/api/organizations/${organizationId}/endpoint-choices${retained.kind === 'cancel' ? `/${retained.id}/cancel` : ''}`;
      const body = retained.kind === 'save' ? { hostname: retained.hostname, applicationHosts: retained.applicationHosts, idempotencyKey: retained.key, confirmed: true } : { idempotencyKey: retained.key, confirmed: true };
      const response = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify(body), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (mutation.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !choice(value, organizationId) || !matches(value, retained)) { setError(object(value) && typeof value.error === 'string' ? value.error : 'The address request could not be confirmed. Refresh current state and review the retained request before retrying.'); setKnown(false); setCurrent(null); return; }
      setCurrent(value.state === 'awaiting_ownership' ? value : null); setKnown(true);
      sessionStorage.removeItem(storageKey); setPending(null);
      setNotice(value.state === 'cancelled' ? 'The choice is cancelled and its receipt is retained. No hosting request was submitted.' : 'The unverified address choice was saved. Default native hosting is paused for this organization until qualification or cancellation.');
    } catch { if (mutation.current === controller && !controller.signal.aborted) { setKnown(false); setCurrent(null); setError('The address request could not be confirmed. Refresh current state; retry keeps this exact request.'); } }
    finally { if (mutation.current === controller && !controller.signal.aborted) { setBusy(false); setReviewed(false); } }
  }
  return <section className="endpoint-choice" aria-labelledby={`endpoint-choice-title-${organizationId}`} aria-busy={busy || loading}>
    <h4 id={`endpoint-choice-title-${organizationId}`}>Saved address choice</h4><p>Saving prepares an unverified address for new hosting. It does not reserve the domain, verify DNS or activate a server.</p>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}{notice && <p role="status" ref={result} tabIndex={-1}>{notice}</p>}
    <button type="button" disabled={busy || loading} onClick={() => { void refresh(); }}>{loading ? 'Loading address choice…' : 'Refresh saved address choice'}</button>
    {current && <div className="endpoint-choice-current"><p><strong>{current.hostname}</strong> is awaiting ownership verification. New default native hosting and ownership handover are paused.</p><p>Publish this TXT record only on the domain you control. Proof expires {new Date(current.expiresAt).toLocaleString()}. An expired proof still needs explicit cancellation before default hosting.</p><dl><dt>TXT name</dt><dd><code>{current.proofName}</code></dd><dt>TXT value</dt><dd><code>{current.proofValue}</code></dd></dl><p>Exclusive ownership claims, supported DNS routing, HTTPS and activation remain unavailable.</p><EndpointDNSObservation key={current.id} organizationId={organizationId} choiceId={current.id} hostname={current.hostname} csrfToken={csrfToken} disabled={busy || loading || !!pending || blocked || !known} onAccessDenied={() => { setCurrent(null); setKnown(false); setReviewed(false); setNotice(''); setError('Current access to this address choice could not be confirmed. Refresh organization access before continuing.'); }}/><button type="button" disabled={busy || loading || !!pending || blocked} onClick={() => stage({ kind: 'cancel', key: crypto.randomUUID(), id: current.id, hostname: current.hostname })}>Review cancellation of this choice</button></div>}
    {known && !current && <p>No pending address choice was found.</p>}
    {known && !current && plan?.state === 'planning_only' && plan.hostname.length <= 244 && <button type="button" disabled={busy || loading || !!pending || blocked} onClick={() => stage({ kind: 'save', key: crypto.randomUUID(), hostname: plan.hostname, applicationHosts: plan.applicationHosts })}>Review saving {plan.hostname}</button>}
    {(pending || blocked) && <section className="endpoint-choice-review" aria-labelledby={`endpoint-choice-review-${organizationId}`}><h5 id={`endpoint-choice-review-${organizationId}`} tabIndex={-1} ref={reviewHeading}>Review retained address request</h5>
      <p>{pending ? pending.kind === 'save' ? `Save ${pending.hostname} for new hosting. Declared app/OAuth hosts: ${pending.applicationHosts.join(', ') || 'none'}.` : `Cancel saved choice ${pending.id} for ${pending.hostname}.` : 'The retained request could not be validated. Check current server-side state before clearing the browser request.'}</p>
      <p>Saving pauses new default native hosting and handover. Cancellation retains the receipt and submits no deployment. Clearing a browser request does not undo a saved choice.</p>
      <label><input type="checkbox" checked={reviewed} disabled={busy || loading || !known} onChange={event => setReviewed(event.target.checked)}/>I checked the current choice and this exact address request.</label>
      <button type="button" disabled={busy || loading || !known || !reviewed || pending === null} onClick={() => { void submit(); }}>Confirm or retry address request</button>
      <button type="button" disabled={busy || loading || !known || !reviewed} onClick={clear}>Clear browser request after checking current state</button>
    </section>}
  </section>;
}
