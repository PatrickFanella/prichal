import { HandleSetup } from './HandleSetup';
import { useEffect, useRef, useState } from 'react';

import { enrollmentObject as object, enrollmentUUID as uuid, validEnrollment as valid, type Enrollment } from './enrollment-contract';
type Request = { idempotencyKey: string; recover: boolean };
export function FirstAccount({ deploymentId, csrfToken }: { deploymentId: string; csrfToken: string }) {
  const storageKey = `pds-enrollment-request:${deploymentId}`;
  const [admission, setAdmission] = useState<{ open: boolean; updatedAt: string } | null>(null);
  const [policyChecked, setPolicyChecked] = useState(false);
  const policyRead = useRef(0);
  const [operation, setOperation] = useState<Enrollment | null>(null);
  const [request, setRequest] = useState<Request | null>(() => {
    try { const value: unknown = JSON.parse(localStorage.getItem(storageKey) ?? 'null'); return object(value) && typeof value.idempotencyKey === 'string' && uuid.test(value.idempotencyKey) && typeof value.recover === 'boolean' ? { idempotencyKey: value.idempotencyKey, recover: value.recover } : null; } catch { return null; }
  });
  const withdrawalStorageKey = `pds-enrollment-withdrawal:${deploymentId}`;
  const [withdrawalId, setWithdrawalId] = useState(() => {
    try { const id = sessionStorage.getItem(withdrawalStorageKey); return id && uuid.test(id) ? id : ''; } catch { return ''; }
  });
  const [review, setReview] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const failure = useRef<HTMLParagraphElement | null>(null);
  const reviewTitle = useRef<HTMLHeadingElement | null>(null);
  const result = useRef<HTMLParagraphElement | null>(null);
  const reviewLauncher = useRef<HTMLButtonElement | null>(null);
  const mutation = useRef<AbortController | null>(null);
  const busyRef = useRef(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    busyRef.current = false; setBusy(false);
    return () => { mutation.current?.abort(); mutation.current = null; busyRef.current = false; policyRead.current++; };
  }, [deploymentId, csrfToken]);
  function clearWithdrawal() {
    try { sessionStorage.removeItem(withdrawalStorageKey); } catch { /* Confirmed status is retained by the service. */ }
    setWithdrawalId('');
  }
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loadError, setLoadError] = useState('');
  const [now, setNow] = useState(Date.now());
  useEffect(() => { if (error || loadError) failure.current?.focus(); }, [error, loadError]);
  useEffect(() => { if (review) reviewTitle.current?.focus(); }, [review]);
  useEffect(() => { setUnderstood(false); setReview(false); setNotice(''); }, [operation?.id, operation?.state, operation?.withdrawalAttempts]);
  useEffect(() => {
    const controller = new AbortController();
    let loading = false;
    const load = async () => {
      if (loading || busyRef.current || controller.signal.aborted) return;
      loading = true;
      const sequence = ++policyRead.current;
      try {
        const [policyReadResult, enrollmentReadResult] = await Promise.allSettled([
          fetch(`/api/deployments/${deploymentId}/enrollment/admission`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }),
          fetch(withdrawalId ? `/api/enrollments/${withdrawalId}` : `/api/deployments/${deploymentId}/enrollment`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }),
        ]);
        if (controller.signal.aborted || policyRead.current !== sequence) return;
        try {
          if (policyReadResult.status !== 'fulfilled') throw new Error('Enrollment policy unavailable');
          const policyResponse = policyReadResult.value;
          const policy: unknown = await policyResponse.json();
          if (controller.signal.aborted || policyRead.current !== sequence) return;
          if (!policyResponse.ok || !object(policy) || typeof policy.open !== 'boolean' || typeof policy.updatedAt !== 'string' || !Number.isFinite(Date.parse(policy.updatedAt))) throw new Error('Enrollment policy unavailable');
          setAdmission({ open: policy.open, updatedAt: policy.updatedAt });
        } catch { if (!controller.signal.aborted && policyRead.current === sequence) setAdmission(null); }
        finally { if (!controller.signal.aborted && policyRead.current === sequence) setPolicyChecked(true); }
        try {
          if (enrollmentReadResult.status !== 'fulfilled') throw new Error('Enrollment unavailable');
          const response = enrollmentReadResult.value;
          if (response.status === 404) { setOperation(null); setLoaded(!withdrawalId); setLoadError(withdrawalId ? 'The retained withdrawal request could not be found. Sign in with its original owner or contact support.' : ''); return; }
          if (response.status === 401 || response.status === 403) { setOperation(null); setLoaded(false); setLoadError('Sign in again to view account status.'); return; }
          const value: unknown = await response.json();
          if (controller.signal.aborted || policyRead.current !== sequence) return;
          if (!response.ok || !valid(value, deploymentId) || (withdrawalId && value.id !== withdrawalId)) throw new Error('Enrollment unavailable');
          setOperation(value); setLoaded(true); setLoadError('');
          if (withdrawalId && value.id === withdrawalId && (value.withdrawalRequested || ['account_created', 'verified', 'withdrawn', 'superseded'].includes(value.state))) { clearWithdrawal(); setError(''); }
        } catch { if (!controller.signal.aborted && policyRead.current === sequence) { setOperation(null); setLoaded(false); setLoadError('Account status is temporarily unavailable. Invitation controls are paused until status returns.'); } }
      } finally { loading = false; }
    };
    void load();
    const interval = window.setInterval(() => { setNow(Date.now()); void load(); }, 3000);
    return () => { controller.abort(); window.clearInterval(interval); };
  }, [deploymentId, withdrawalId]);
  async function prepare(recover: boolean) {
    if (busyRef.current || !loaded || withdrawalId || (operation?.withdrawalRequested && operation.state !== 'withdrawn') || (!request && admission?.open !== true)) return;
    busyRef.current = true; policyRead.current++;
    const controller = new AbortController(); mutation.current = controller;
    setBusy(true); setError(''); setNotice('');
    const pending = request ?? { idempotencyKey: crypto.randomUUID(), recover };
    setRequest(pending);
    try { localStorage.setItem(storageKey, JSON.stringify(pending)); } catch { /* Keep the stable retry key in memory. */ }
    try {
      const response = await fetch(`/api/deployments/${deploymentId}/enrollment`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify(pending), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok || !valid(value, deploymentId)) { if (object(value) && typeof value.error === 'string') setError(value.error); else setError('Invitation could not be confirmed. Retry the same request.'); return; }
      setOperation(value); setRequest(null);
      try { localStorage.removeItem(storageKey); } catch { /* The operation is durable on the service. */ }
    } catch { if (!controller.signal.aborted) setError('Invitation could not be confirmed. Retry the same request.'); }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  async function refresh() {
    if (!operation || busyRef.current || !loaded || withdrawalId || ['pending', 'superseded', 'failed', 'withdrawing', 'withdrawn'].includes(operation.state)) return;
    busyRef.current = true; policyRead.current++;
    const controller = new AbortController(); mutation.current = controller;
    setBusy(true); setError(''); setNotice('');
    try {
      const response = await fetch(`/api/enrollments/${operation.id}/refresh`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok || !valid(value, deploymentId) || value.id !== operation.id) { setError(object(value) && typeof value.error === 'string' ? value.error : 'Account status refresh could not be confirmed.'); return; }
      setOperation(value); setNotice('Account status check requested.');
    } catch { if (!controller.signal.aborted) setError('Account status refresh could not be confirmed.'); }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  async function withdraw() {
    if (!operation || busyRef.current || !loaded || !review || !understood || (withdrawalId && withdrawalId !== operation.id)) return;
    if (!['invitation_ready', 'expired', 'failed'].includes(operation.state) || operation.withdrawalAttempts >= 3) return;
    const target = withdrawalId || operation.id;
    try { sessionStorage.setItem(withdrawalStorageKey, target); }
    catch { setError('This browser cannot retain a withdrawal request. Enable session storage before proceeding.'); return; }
    busyRef.current = true; policyRead.current++;
    const controller = new AbortController(); mutation.current = controller;
    const { code: _code, ...withoutCode } = operation;
    setWithdrawalId(target); setOperation(withoutCode); setLoaded(false);
    setBusy(true); setError(''); setNotice(''); setReview(false); setUnderstood(false);
    try {
      const response = await fetch(`/api/enrollments/${target}/withdraw`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok || !valid(value, deploymentId) || value.id !== target || !(value.withdrawalRequested || ['account_created', 'verified', 'withdrawn', 'superseded'].includes(value.state))) {
        if (response.status === 401 || response.status === 403 || response.status === 404) { setOperation(null); setLoaded(false); }
        setError('Invitation withdrawal could not be confirmed. The same request is retained. Refresh status before reviewing a retry.'); return;
      }
      setOperation(value); setLoaded(true); clearWithdrawal();
      setNotice(value.state === 'withdrawing' ? 'Withdrawal is recorded. Waiting for native confirmation. An account already created will be preserved.' : value.state === 'withdrawn' ? 'Native invitation withdrawal is confirmed.' : 'Account status was preserved. Review the current result.');
      window.setTimeout(() => { if (!controller.signal.aborted) result.current?.focus(); }, 0);
    } catch { if (!controller.signal.aborted) setError('Invitation withdrawal could not be confirmed. The same request is retained. Refresh status before reviewing a retry.'); }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  const expired = operation?.state === 'invitation_ready' && (!operation.expiresAt || Date.parse(operation.expiresAt) <= now);
  const state = expired ? 'expired' : operation?.state;
  const canWithdraw = !!operation && ['invitation_ready', 'expired', 'failed'].includes(operation.state ?? '') && operation.withdrawalAttempts < 3;
  const canRecover = !operation || (!operation.withdrawalRequested && ['expired', 'failed', 'superseded'].includes(state ?? '')) || state === 'withdrawn';
  return <section className="first-account" aria-label="First AT account">
    <h4>First account</h4>
    {admission?.open === false && <p role="status">New first-account requests are paused. Retained requests and account status remain available. Already-issued invitations remain valid until expiry.</p>}
    {!policyChecked && <p>Checking first-account availability…</p>}
    {policyChecked && admission === null && <p>New first-account availability could not be confirmed. Retained requests and account status remain available.</p>}
    <p>Create your account directly on your PDS. Your account password stays with the PDS.</p>
    <p>For this local test, use a .test handle with 3–18 characters before the suffix and a .invalid email address. Confirmation mail stays in the test inbox.</p>
    {operation && <p role="status" ref={result} tabIndex={-1}>{withdrawalId ? 'withdrawal unconfirmed' : state}: {withdrawalId ? 'The retained request is awaiting status confirmation.' : expired ? 'This invitation expired. Request recovery to replace it.' : operation.message}</p>}
    {state === 'invitation_ready' && !withdrawalId && loaded && operation?.code && <>
      <p>One-use invitation: <code>{operation.code}</code></p>
      <p>Expires {operation.expiresAt && new Date(operation.expiresAt).toLocaleString()}.</p>
      <button type="button" onClick={() => { if (!navigator.clipboard) { setError('Copy unavailable. Select the invitation text.'); return; } void navigator.clipboard.writeText(operation.code ?? '').then(() => setNotice('Invitation copied.'), () => setError('Copy unavailable. Select the invitation text.')); }}>Copy invitation</button>
      <p><a href={operation.accountUrl} target="_blank" rel="noreferrer">Open account creation on your PDS</a></p>
    </>}
    {(state === 'account_created' || state === 'verified') && <p>{operation?.handle}. {operation?.emailConfirmed ? 'Email confirmed.' : 'Email confirmation pending.'}</p>}
    {!withdrawalId && (canRecover || request) && <button type="button" disabled={busy || !loaded || (!request && admission?.open !== true)} onClick={() => { void prepare(!!operation); }}>{busy ? 'Preparing invitation…' : request ? 'Retry invitation request' : operation ? 'Recover invitation' : 'Prepare first-account invitation'}</button>}
    {operation && <button type="button" disabled={busy || !loaded || !!withdrawalId || ['pending', 'superseded', 'failed', 'withdrawing', 'withdrawn'].includes(state ?? '')} onClick={() => { void refresh(); }}>Check account status</button>}
    {withdrawalId && <p role="status">An unconfirmed withdrawal request is retained in this browser tab. Invitation copying, account creation and replacement are paused. Any retry uses the same invitation ticket.</p>}
    {state === 'withdrawing' && <p>Waiting for native confirmation. An account that completed signup will be preserved.</p>}
    {state === 'failed' && operation?.withdrawalRequested && <p>Withdrawal is unconfirmed. Invitation replacement is blocked. {operation.withdrawalAttempts >= 3 ? 'The local retry limit was reached. Contact support.' : 'Review a withdrawal retry with the same ticket or contact support.'}</p>}
    {canWithdraw && loaded && !review && <button ref={reviewLauncher} type="button" disabled={busy || !!request} onClick={() => { setError(''); setUnderstood(false); setReview(true); }}>{withdrawalId || operation?.withdrawalRequested ? 'Review withdrawal retry' : 'Review invitation withdrawal'}</button>}
    {review && operation && canWithdraw && loaded && <div className="enrollment-withdrawal-review" role="group" aria-labelledby={`withdrawal-review-${deploymentId}`}>
      <h5 id={`withdrawal-review-${deploymentId}`} ref={reviewTitle} tabIndex={-1}>Review invitation withdrawal</h5>
      <p>This closes the retained invitation. The server may briefly restart while signup is drained. A completed account and its data remain preserved. Completion requires a confirmed native result.</p>
      <label><input type="checkbox" checked={understood} onChange={event => setUnderstood(event.target.checked)} disabled={busy} /> I understand that an account already created will be preserved.</label>
      <button type="button" disabled={busy || !understood} onClick={() => { void withdraw(); }}>{withdrawalId || operation.withdrawalRequested ? 'Retry invitation withdrawal' : 'Request invitation withdrawal'}</button>
      <button type="button" disabled={busy} onClick={() => { setReview(false); setUnderstood(false); window.setTimeout(() => reviewLauncher.current?.focus(), 0); }}>Close review</button>
    </div>}
    {state === 'verified' && <HandleSetup deploymentId={deploymentId} csrfToken={csrfToken} />}
    {notice && <p role="status">{notice}</p>}{(error || loadError) && <p role="alert" ref={failure} tabIndex={-1}>{loadError || error}</p>}
  </section>;
}
