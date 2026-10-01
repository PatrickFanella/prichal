import { useEffect, useRef, useState } from 'react';
import { claimChoice, claimIntent, claimReceipt, type ClaimChoice, type ClaimIntent, type ClaimReceipt } from './endpoint-claim-contract';

export function EndpointClaim({ organizationId, ownerId, csrfToken, choice, disabled, onPendingChange, onAccessDenied }: {
  organizationId: string; ownerId: string; csrfToken: string; choice: ClaimChoice | null; disabled: boolean;
  onPendingChange: (pending: boolean) => void; onAccessDenied: () => void;
}) {
  const storageKey = `pds-endpoint-claim:${ownerId}:${organizationId}`;
  const [retained] = useState(() => {
    try { const raw = sessionStorage.getItem(storageKey); if (!raw) return { intent: null, blocked: false }; const v: unknown = JSON.parse(raw); return claimIntent(v) ? { intent: v, blocked: false } : { intent: null, blocked: true }; }
    catch { return { intent: null, blocked: true }; }
  });
  const [intent, setIntent] = useState<ClaimIntent | null>(retained.intent), [blocked, setBlocked] = useState(retained.blocked);
  const [scoped, setScoped] = useState<ClaimChoice | null>(null), [value, setValue] = useState<ClaimReceipt | null>(null);
  const [loaded, setLoaded] = useState(false), [loading, setLoading] = useState(false), [busy, setBusy] = useState(false);
  const [review, setReview] = useState<'reserve' | 'clear' | null>(null), [understood, setUnderstood] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [now, setNow] = useState(Date.now());
  const read = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null), busyRef = useRef(false), focusResult = useRef(false);
  const panel = useRef<HTMLElement | null>(null);
  const focusAfterMutation = useRef(false);
  const title = useRef<HTMLHeadingElement | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null), launcher = useRef<HTMLButtonElement | null>(null);
  const target = intent ?? (choice ? { id: choice.id, hostname: choice.hostname } : null);
  const eligible = !!scoped && scoped.state === 'awaiting_ownership' && Date.parse(scoped.expiresAt) > now && !scoped.hostname.endsWith('.test') && !value;
  useEffect(() => { if (review === 'reserve' && !eligible && scoped?.state === 'awaiting_ownership' && Date.parse(scoped.expiresAt) <= now) setError('The saved proof expired before confirmation. Refresh current status before continuing.'); setReview(null); setUnderstood(false); }, [eligible, scoped?.id, scoped?.state, scoped?.expiresAt, value?.state]);
  useEffect(() => { onPendingChange(!!intent || blocked || busy); }, [intent, blocked, busy, onPendingChange]);
  useEffect(() => { if (intent || blocked || error || review) { const details = panel.current?.closest('details'); if (details) details.open = true; } if (review) title.current?.focus(); }, [intent, blocked, error, review]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { if ((loaded || notice) && focusResult.current && (value || notice)) { status.current?.closest('details')?.setAttribute('open', ''); status.current?.focus(); focusResult.current = false; } }, [loaded, value, notice]);
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer); }, []);
  useEffect(() => {
    busyRef.current = false; focusResult.current = false; focusAfterMutation.current = false; setBusy(false); setLoaded(false); setScoped(null); setValue(null); setReview(null); setUnderstood(false); setError(''); setNotice('');
    return () => { read.current?.abort(); read.current = null; mutation.current?.abort(); mutation.current = null; busyRef.current = false; focusResult.current = false; focusAfterMutation.current = false; };
  }, [organizationId, ownerId, csrfToken]);
  function clearIntent() {
    try { sessionStorage.removeItem(storageKey); setIntent(null); setBlocked(false); return true; }
    catch { setError('The browser retry could not be cleared. Server receipts remain preserved.'); return false; }
  }
  async function refresh(explicit = true) {
    if (disabled || busyRef.current || blocked || !target) return;
    read.current?.abort(); const controller = new AbortController(); read.current = controller;
    const shouldFocus = explicit || !!intent || focusAfterMutation.current; focusAfterMutation.current = false;
    setLoading(true); setLoaded(false); setScoped(null); setValue(null); setReview(null); setUnderstood(false); setError(''); setNotice('');
    const exact = target, path = `/api/organizations/${organizationId}/endpoint-choices/${exact.id}`;
    try {
      const responses = await Promise.all([fetch(path, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }), fetch(path + '/claim', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) })]);
      if (read.current !== controller || controller.signal.aborted) return;
      if ([401, 403, 404].includes(responses[0].status) || [401, 403].includes(responses[1].status)) { setError('Access to the retained claim choice could not be confirmed. Sign in with its original owner or contact support.'); onAccessDenied(); return; }
      const source: unknown = await responses[0].json();
      if (read.current !== controller || controller.signal.aborted) return;
      if (!responses[0].ok || !claimChoice(source, organizationId, exact)) throw Error('Invalid claim choice');
      let receipt: ClaimReceipt | null = null;
      if (responses[1].status !== 404) { const v: unknown = await responses[1].json(); if (read.current !== controller || controller.signal.aborted) return; if (!responses[1].ok || !claimReceipt(v, exact)) throw Error('Invalid claim receipt'); receipt = v; }
      setScoped(source); setValue(receipt); setLoaded(true); focusResult.current = shouldFocus;
      if (receipt) { if (intent) clearIntent(); setNotice('The exact claim receipt was loaded. Hosting activation remains unavailable.'); }
      else if (explicit || intent) setNotice('No claim receipt was found for this exact choice. Review its current state before retrying or closing the browser retry.');
    } catch { if (read.current === controller && !controller.signal.aborted) { setScoped(null); setValue(null); setLoaded(false); setError('Claim status could not be confirmed. Review controls are paused. Refresh this exact choice before retrying.'); } }
    finally { if (read.current === controller && !controller.signal.aborted) setLoading(false); }
  }
  useEffect(() => {
    read.current?.abort(); read.current = null; setReview(null); setUnderstood(false);
    if (disabled) { setScoped(null); setValue(null); setLoaded(false); setLoading(false); }
    else if (!busy) void refresh(false);
    return () => { read.current?.abort(); read.current = null; };
  }, [target?.id, target?.hostname, choice?.state, choice?.expiresAt, disabled, busy, csrfToken]);
  async function submit() {
    if (!target || !eligible || !loaded || disabled || blocked || busyRef.current || review !== 'reserve' || !understood) return;
    const exact = target;
    try { sessionStorage.setItem(storageKey, JSON.stringify(exact)); }
    catch { setError('Enable session storage before requesting a domain claim. No request was submitted.'); return; }
    busyRef.current = true; read.current?.abort(); read.current = null;
    const controller = new AbortController(); mutation.current = controller;
    setIntent(exact); setBusy(true); setLoaded(false); setValue(null); setError(''); setNotice(''); setReview(null); setUnderstood(false);
    try {
      const response = await fetch(`/api/organizations/${organizationId}/endpoint-choices/${exact.id}/claim`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const v: unknown = await response.json();
      if (mutation.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !claimReceipt(v, exact)) { setScoped(null); setError('The domain claim could not be confirmed. The exact choice is retained. Refresh status before reviewing a retry.'); if ([401, 403, 404].includes(response.status)) onAccessDenied(); return; }
      setValue(v); setLoaded(true); clearIntent(); focusResult.current = true; focusAfterMutation.current = true; setNotice('The claim receipt was confirmed. Routing, HTTPS and hosting remain unqualified.');
    } catch { if (mutation.current === controller && !controller.signal.aborted) { setScoped(null); setError('The domain claim could not be confirmed. The exact choice is retained. Refresh status before reviewing a retry.'); } }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  if (!choice && !intent && !blocked && !value && !error && !notice) return null;
  const receiptState = value?.state === 'reserved' && Date.parse(value.expiresAt) <= now ? 'expired' : value?.state;
  const canClose = (blocked && !disabled) || (!!intent && loaded && !eligible);
  return <section ref={panel} className="endpoint-claim" aria-label="Exclusive hostname claim" aria-busy={loading || busy}>
    <h5>Exclusive hostname claim</h5><p>A claim reserves this exact hosting hostname through this service until its proof expires. Routing, HTTPS and hosting activation remain unavailable.</p>
    {intent && <p>Unconfirmed retry retained for {intent.hostname}. Other address changes are paused until the exact result is checked. Reload never submits a claim automatically.</p>}
    {blocked && <p>The retained browser retry could not be validated. Check the saved address choice before reviewing its removal. Removing it changes no server receipt.</p>}
    {(target || value) && <button type="button" disabled={disabled || loading || busy || blocked || !target} onClick={() => { void refresh(); }}>Refresh exact claim status</button>}
    {!disabled && (value || notice) && <p role="status" ref={status} tabIndex={-1}>{value ? `${value.hostname}: ${receiptState}. Receipt expires ${new Date(value.expiresAt).toLocaleString()}. ` : ''}{notice} No hosting activation is authorized.</p>}
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {loaded && scoped && !eligible && !value && <p>{scoped.state === 'cancelled' ? 'This saved choice is cancelled.' : Date.parse(scoped.expiresAt) <= now ? 'This saved proof has expired.' : 'A local test namespace cannot obtain a public-domain reservation.'} No new claim can be requested for this choice.</p>}
    {!review && !disabled && (eligible && loaded || canClose) && <button ref={launcher} type="button" disabled={busy || loading} onClick={() => { setError(''); setUnderstood(false); setReview(eligible ? 'reserve' : 'clear'); }}>{eligible ? intent ? 'Review exact claim retry' : 'Review exclusive domain claim' : 'Review closing browser retry'}</button>}
    {review && !disabled && <div className="endpoint-choice-review">
      <h6 ref={title} tabIndex={-1}>{review === 'reserve' ? 'Review exclusive domain claim' : 'Review closing browser retry'}</h6>
      <p>{review === 'reserve' ? `Request a reservation for ${target?.hostname}. The service will recheck the exact TXT challenge and competing claims. This does not activate hosting.` : 'Remove only this browser retry after checking current scope and state. Any server choice or claim receipt remains preserved.'}</p>
      <label><input type="checkbox" checked={understood} disabled={busy || loading} onChange={event => setUnderstood(event.target.checked)} /> I reviewed this exact choice and understand that hosting remains unqualified.</label>
      <button type="button" disabled={busy || loading || !understood || !loaded && !blocked || review === 'reserve' && !eligible || review === 'clear' && !canClose} onClick={() => { if (review === 'reserve' && eligible) void submit(); else if (review === 'clear' && canClose && clearIntent()) { setReview(null); setUnderstood(false); setNotice('The browser retry was closed. Server receipts remain preserved.'); focusResult.current = true; } }}>{review === 'reserve' ? intent ? 'Retry exact domain claim' : 'Request exclusive domain claim' : 'Close browser retry'}</button>
      <button type="button" disabled={busy} onClick={() => { setReview(null); setUnderstood(false); window.setTimeout(() => launcher.current?.focus(), 0); }}>Close review</button>
    </div>}
  </section>;
}
