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
  const cancellationStorageKey = `pds-enrollment-cancellation:${deploymentId}`;
  const [cancellationId, setCancellationId] = useState(() => {
    try { const id = sessionStorage.getItem(cancellationStorageKey); return id && uuid.test(id) ? id : ''; } catch { return ''; }
  });
  const [cancellationReview, setCancellationReview] = useState(false);
  const cancellationTitle = useRef<HTMLHeadingElement | null>(null);
  const cancellationLauncher = useRef<HTMLButtonElement | null>(null);
  const [review, setReview] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const failure = useRef<HTMLParagraphElement | null>(null);
  const reviewTitle = useRef<HTMLHeadingElement | null>(null);
  const result = useRef<HTMLParagraphElement | null>(null);
  const focusResult = useRef(false);
  const reviewLauncher = useRef<HTMLButtonElement | null>(null);
  const mutation = useRef<AbortController | null>(null);
  const busyRef = useRef(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    busyRef.current = false; setBusy(false);
    return () => { mutation.current?.abort(); mutation.current = null; busyRef.current = false; focusResult.current = false; policyRead.current++; };
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
  useEffect(() => { if (cancellationReview) cancellationTitle.current?.focus(); }, [cancellationReview]);
  useEffect(() => { if (review) reviewTitle.current?.focus(); }, [review]);
  useEffect(() => { if (focusResult.current && loaded && operation) { result.current?.focus(); focusResult.current = false; } }, [operation, loaded, cancellationId]);
  useEffect(() => { setUnderstood(false); setReview(false); setCancellationReview(false); setNotice(''); }, [operation?.id, operation?.state, operation?.withdrawalAttempts, operation?.canCancel]);
  useEffect(() => {
    const controller = new AbortController();
    let loading = false;
    const load = async () => {
      if (loading || busyRef.current || controller.signal.aborted) return;
      if (withdrawalId && cancellationId) { setOperation(null); setLoaded(false); setLoadError('Conflicting retained enrollment actions require support review. No action will be retried automatically.'); return; }
      loading = true;
      const sequence = ++policyRead.current;
      try {
        const [policyReadResult, enrollmentReadResult] = await Promise.allSettled([
          fetch(`/api/deployments/${deploymentId}/enrollment/admission`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }),
          fetch((withdrawalId || cancellationId) ? `/api/enrollments/${withdrawalId || cancellationId}` : `/api/deployments/${deploymentId}/enrollment`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }),
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
          if (response.status === 404) { setOperation(null); setLoaded(!withdrawalId && !cancellationId); setLoadError((withdrawalId || cancellationId) ? 'The retained enrollment action could not be found. Sign in with its original owner or contact support.' : ''); return; }
          if (response.status === 401 || response.status === 403) { setOperation(null); setLoaded(false); setLoadError('Sign in again to view account status.'); return; }
          const value: unknown = await response.json();
          if (controller.signal.aborted || policyRead.current !== sequence) return;
          if (!response.ok || !valid(value, deploymentId) || ((withdrawalId || cancellationId) && value.id !== (withdrawalId || cancellationId))) throw new Error('Enrollment unavailable');
          setOperation(value); setLoaded(true); setLoadError('');
          if (cancellationId && value.id === cancellationId && !value.canCancel) {
            try { sessionStorage.removeItem(cancellationStorageKey); } catch { /* The server retains the result. */ }
            setCancellationId(''); setError('');
            setNotice(value.state === 'cancelled' ? 'Queued cancellation is confirmed. Native preparation did not start for this request.' : 'Queued cancellation is no longer available. Review the current enrollment result.');
            focusResult.current = true;
          }
          if (withdrawalId && value.id === withdrawalId && (value.withdrawalRequested || ['account_created', 'verified', 'withdrawn', 'superseded'].includes(value.state))) { clearWithdrawal(); setError(''); }
        } catch { if (!controller.signal.aborted && policyRead.current === sequence) { setOperation(null); setLoaded(false); setLoadError('Account status is temporarily unavailable. Invitation controls are paused until status returns.'); } }
      } finally { loading = false; }
    };
    void load();
    const interval = window.setInterval(() => { setNow(Date.now()); void load(); }, 3000);
    return () => { controller.abort(); window.clearInterval(interval); };
  }, [deploymentId, withdrawalId, cancellationId]);
  async function prepare(recover: boolean) {
    if (busyRef.current || !loaded || withdrawalId || cancellationId || (operation?.withdrawalRequested && operation.state !== 'withdrawn') || (!request && admission?.open !== true)) return;
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
    if (!operation || busyRef.current || !loaded || withdrawalId || cancellationId || ['pending', 'superseded', 'failed', 'withdrawing', 'withdrawn', 'cancelled'].includes(operation.state)) return;
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
    if (!operation || cancellationId || busyRef.current || !loaded || !review || !understood || (withdrawalId && withdrawalId !== operation.id)) return;
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
  async function cancelQueued() {
    if (!operation?.canCancel || busyRef.current || !loaded || !cancellationReview || !understood || withdrawalId || (cancellationId && cancellationId !== operation.id)) return;
    const target = cancellationId || operation.id;
    try { sessionStorage.setItem(cancellationStorageKey, target); }
    catch { setError('This browser cannot retain a cancellation request. Enable session storage before proceeding.'); return; }
    busyRef.current = true; policyRead.current++;
    const controller = new AbortController(); mutation.current = controller;
    setCancellationId(target); setLoaded(false); setBusy(true); setError(''); setNotice(''); setCancellationReview(false); setUnderstood(false);
    try {
      const response = await fetch(`/api/enrollments/${target}/cancel`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok || !valid(value, deploymentId) || value.id !== target || value.state !== 'cancelled') {
        if ([401, 403, 404].includes(response.status)) { setOperation(null); setLoaded(false); }
        setError('Queued cancellation could not be confirmed. The exact request is retained. Wait for status before reviewing a retry.'); return;
      }
      try { sessionStorage.removeItem(cancellationStorageKey); } catch { /* The server retains the confirmed result. */ }
      setCancellationId(''); setOperation(value); setLoaded(true);
      setNotice('Queued cancellation is confirmed. Native preparation did not start for this request.');
      window.setTimeout(() => { if (!controller.signal.aborted) result.current?.focus(); }, 0);
    } catch { if (!controller.signal.aborted) setError('Queued cancellation could not be confirmed. The exact request is retained. Wait for status before reviewing a retry.'); }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  const expired = operation?.state === 'invitation_ready' && (!operation.expiresAt || Date.parse(operation.expiresAt) <= now);
  const state = expired ? 'expired' : operation?.state;
  const canWithdraw = !!operation && ['invitation_ready', 'expired', 'failed'].includes(operation.state ?? '') && operation.withdrawalAttempts < 3;
  const canRecover = !operation || (!operation.withdrawalRequested && ['expired', 'failed', 'superseded', 'cancelled'].includes(state ?? '')) || state === 'withdrawn';
  const completedAccount = loaded && !loadError && !withdrawalId && !cancellationId && operation?.did && valid(operation, deploymentId) && (state === 'account_created' || state === 'verified');
  return <section className="first-account" aria-label="First AT account">
    <h4>First account</h4>
    {admission?.open === false && <p role="status">New first-account requests are paused. Retained requests and account status remain available. Already-issued invitations remain valid until expiry.</p>}
    {!policyChecked && <p>Checking first-account availability…</p>}
    {policyChecked && admission === null && <p>New first-account availability could not be confirmed. Retained requests and account status remain available.</p>}
    <p>Create your account directly on your server. Your account password stays with that server.</p>
    <p>For this local test, use a .test handle with 3–18 characters before the suffix and a .invalid email address. Confirmation mail stays in the test inbox.</p>
    {operation && <p role="status" ref={result} tabIndex={-1}>{withdrawalId ? 'withdrawal unconfirmed' : cancellationId ? 'cancellation unconfirmed' : state}: {(withdrawalId || cancellationId) ? 'The retained request is awaiting status confirmation.' : expired ? 'This invitation expired. Request recovery to replace it.' : operation.message}</p>}
    {state === 'invitation_ready' && !withdrawalId && !cancellationId && loaded && operation?.code && <>
      <p>One-use invitation: <code>{operation.code}</code></p>
      <p>Expires {operation.expiresAt && new Date(operation.expiresAt).toLocaleString()}.</p>
      <button type="button" onClick={() => { if (!navigator.clipboard) { setError('Copy unavailable. Select the invitation text.'); return; } void navigator.clipboard.writeText(operation.code ?? '').then(() => setNotice('Invitation copied.'), () => setError('Copy unavailable. Select the invitation text.')); }}>Copy invitation</button>
      <p><a href={operation.accountUrl} target="_blank" rel="noreferrer">Open account creation on your server</a></p>
    </>}
    {(state === 'account_created' || state === 'verified') && <p>{operation?.handle}. {operation?.emailConfirmed ? 'Email confirmed.' : 'Email confirmation pending.'}</p>}
    {completedAccount && <div className="native-account-security">
      <h5>Email sign-in codes</h5>
      <p>Status unknown. Prichal does not read your native account session or change this setting.</p>
      <p>In Bluesky, open Settings → Privacy and security → Two-factor authentication. Confirm your account email, then enable email sign-in codes there. Enter your password and codes only in your client or on your server’s sign-in page.</p>
      <p>Keep access to that mailbox. Codes protect new password sign-ins; existing sessions and app passwords are separate. Review app passwords in your client. Check that your chosen client asks for the code before relying on it.</p>
      <p>If you lose mailbox access, keep any signed-in client open and recover the mailbox with your email provider. Contact hosting support if that fails. Prichal has no automatic mailbox-recovery or factor-disable flow; never send support your password or a sign-in code.</p>
    </div>}
    {completedAccount && operation && <div className="public-account-records">
      <h5>Public records and media</h5>
      <p>Records and media published by this account are public.</p>
      {operation.handle?.endsWith('.test') ? <p>Local test accounts cannot be opened in a public browser.</p> : <>
        <p><a href={`https://pdsls.dev/at/${operation.did}`} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">View public records in PDSls (opens in a new tab)</a></p>
        <p>PDSls is an independent service. You can browse without signing in. If it is unavailable, return here to keep managing your server.</p>
      </>}
    </div>}
    {!withdrawalId && !cancellationId && loaded && (canRecover || request) && <button type="button" disabled={busy || (!request && admission?.open !== true)} onClick={() => { void prepare(!!operation); }}>{busy ? 'Preparing invitation…' : request ? 'Retry invitation request' : operation ? 'Recover invitation' : 'Prepare first-account invitation'}</button>}
    {operation && <button type="button" disabled={busy || !loaded || !!withdrawalId || !!cancellationId || ['pending', 'superseded', 'failed', 'withdrawing', 'withdrawn', 'cancelled'].includes(state ?? '')} onClick={() => { void refresh(); }}>Check account status</button>}
    {withdrawalId && <p role="status">An unconfirmed withdrawal request is retained in this browser tab. Invitation copying, account creation and replacement are paused. Any retry uses the same invitation ticket.</p>}
    {state === 'withdrawing' && <p>Waiting for native confirmation. An account that completed signup will be preserved.</p>}
    {state === 'failed' && operation?.withdrawalRequested && <p>Withdrawal is unconfirmed. Invitation replacement is blocked. {operation.withdrawalAttempts >= 3 ? 'The local retry limit was reached. Contact support.' : 'Review a withdrawal retry with the same ticket or contact support.'}</p>}
    {canWithdraw && !cancellationId && loaded && !review && <button ref={reviewLauncher} type="button" disabled={busy || !!request} onClick={() => { setError(''); setUnderstood(false); setCancellationReview(false); setReview(true); }}>{withdrawalId || operation?.withdrawalRequested ? 'Review withdrawal retry' : 'Review invitation withdrawal'}</button>}
    {review && !cancellationId && operation && canWithdraw && loaded && <div className="enrollment-withdrawal-review" role="group" aria-labelledby={`withdrawal-review-${deploymentId}`}>
      <h5 id={`withdrawal-review-${deploymentId}`} ref={reviewTitle} tabIndex={-1}>Review invitation withdrawal</h5>
      <p>This closes the retained invitation. The server may briefly restart while signup is drained. A completed account and its data remain preserved. Completion requires a confirmed native result.</p>
      <label><input type="checkbox" checked={understood} onChange={event => setUnderstood(event.target.checked)} disabled={busy} /> I understand that an account already created will be preserved.</label>
      <button type="button" disabled={busy || !understood} onClick={() => { void withdraw(); }}>{withdrawalId || operation.withdrawalRequested ? 'Retry invitation withdrawal' : 'Request invitation withdrawal'}</button>
      <button type="button" disabled={busy} onClick={() => { setReview(false); setUnderstood(false); window.setTimeout(() => reviewLauncher.current?.focus(), 0); }}>Close review</button>
    </div>}
    {cancellationId && <p role="status">An unconfirmed queued cancellation is retained in this browser tab. Invitation use, replacement and status refresh are paused. A fresh scoped read is required before retrying the same request.</p>}
    {state === 'pending' && loaded && !operation?.canCancel && !cancellationId && <p>Native preparation has started or its prior dispatch history is uncertain. Queued cancellation is unavailable. Wait for status or contact support.</p>}
    {operation?.canCancel && loaded && !withdrawalId && !cancellationReview && <button ref={cancellationLauncher} type="button" disabled={busy || !!request} onClick={() => { setError(''); setUnderstood(false); setReview(false); setCancellationReview(true); }}>{cancellationId ? 'Review queued cancellation retry' : 'Review queued cancellation'}</button>}
    {cancellationReview && operation?.canCancel && loaded && !withdrawalId && <div className="enrollment-withdrawal-review" role="group" aria-labelledby={`cancellation-review-${deploymentId}`}>
      <h5 id={`cancellation-review-${deploymentId}`} ref={cancellationTitle} tabIndex={-1}>Review queued cancellation</h5>
      <p>This cancels this request only if native preparation has not started. If preparation starts first, cancellation is refused and the current result remains available. No account or server data is deleted.</p>
      <label><input type="checkbox" checked={understood} onChange={event => setUnderstood(event.target.checked)} disabled={busy} /> I understand that cancellation may be refused if preparation starts first.</label>
      <button type="button" disabled={busy || !understood} onClick={() => { void cancelQueued(); }}>{cancellationId ? 'Retry queued cancellation' : 'Cancel queued enrollment'}</button>
      <button type="button" disabled={busy} onClick={() => { setCancellationReview(false); setUnderstood(false); window.setTimeout(() => cancellationLauncher.current?.focus(), 0); }}>Close review</button>
    </div>}
    {state === 'verified' && <HandleSetup deploymentId={deploymentId} csrfToken={csrfToken} />}
    {notice && <p role="status">{notice}</p>}{(error || loadError) && <p role="alert" ref={failure} tabIndex={-1}>{loadError || error}</p>}
  </section>;
}
