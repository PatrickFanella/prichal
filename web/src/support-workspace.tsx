import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { LifecycleActivity } from './lifecycle-activity';
import { OperatorLifecycleRecovery } from './operator-lifecycle-recovery';
import { operatorIntent, operatorOperation, type OperatorIntent, type ReceiptState } from './operator-lifecycle-contract';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null && !Array.isArray(value); }
function errorMessage(value: unknown): string { return object(value) && typeof value.error === 'string' ? value.error : 'Support evidence is temporarily unavailable.'; }
export { LifecycleActivity } from './lifecycle-activity';
type Tenant = { instanceId: string; organizationId: string; name: string; state: string; kind: string; managedHostname: string; billing: unknown; hostingGrant: unknown; lifecycle: unknown; incidents: unknown; usage: unknown; domainProof: unknown; managementRecovery: unknown };
function tenant(value: unknown, id: string): value is Tenant { return object(value) && value.instanceId === id && typeof value.organizationId === 'string' && uuid.test(value.organizationId) && typeof value.name === 'string' && typeof value.state === 'string' && ['queued', 'configuring', 'checking', 'ready', 'failed'].includes(value.state) && (value.kind === 'agent' || value.kind === 'test') && typeof value.managedHostname === 'string'; }
function state(value: unknown): string { return object(value) && typeof value.state === 'string' ? value.state : 'Unconfigured'; }
export function SupportWorkspace({ csrfToken, ownerId }: { csrfToken: string; ownerId: string }) {
  const [role, setRole] = useState<'inspect' | 'operate' | null>(null), [reference, setReference] = useState(''), [kind, setKind] = useState('deployment');
  const [view, setView] = useState<Tenant | null>(null), [results, setResults] = useState<{ instanceId: string; name: string; state: string }[]>([]), [truncated, setTruncated] = useState(false);
  const [error, setError] = useState(''), [busy, setBusy] = useState(false), [actionKind, setActionKind] = useState('service'), [desired, setDesired] = useState('suspended'), [reason, setReason] = useState('maintenance'), [reviewed, setReviewed] = useState(false), [outcome, setOutcome] = useState('');
  const request = useRef<OperatorIntent | null>(null);
  const [blocked, setBlocked] = useState(false), [receiptState, setReceiptState] = useState<ReceiptState>('unknown');
  const inspection = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null), permission = useRef<AbortController | null>(null), busyRef = useRef(false);
  const result = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  const storageKey = `pds-support-intent:${ownerId}`;
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { if (outcome) result.current?.focus(); }, [outcome]);
  useEffect(() => { setReviewed(false); }, [receiptState, role]);
  async function loadRole() {
    permission.current?.abort(); const controller = new AbortController(); permission.current = controller; setRole(null);
    try {
      const response = await fetch('/api/operator/session', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (permission.current !== controller || controller.signal.aborted) return;
      if (response.ok && object(value) && (value.role === 'inspect' || value.role === 'operate')) setRole(value.role);
    } catch { /* Current operator authority stays unconfirmed. */ }
  }
  useEffect(() => {
    request.current = null; busyRef.current = false; setBusy(false); setRole(null); setBlocked(false); setReceiptState('unknown'); setView(null); setResults([]); setTruncated(false); setReviewed(false); setError(''); setOutcome('');
    try { const raw = sessionStorage.getItem(storageKey); if (raw) { const value: unknown = JSON.parse(raw); if (!operatorIntent(value)) throw Error('Invalid retained intent'); request.current = value; setActionKind(value.kind); setDesired(value.state); setReason(value.reason); setReference(value.instance); setKind('deployment'); } }
    catch { setBlocked(true); setError('Retained support metadata could not be validated. New actions are paused.'); }
    void loadRole();
    return () => { permission.current?.abort(); permission.current = null; inspection.current?.abort(); inspection.current = null; mutation.current?.abort(); mutation.current = null; busyRef.current = false; };
  }, [ownerId, storageKey, csrfToken]);
  function closeConfirmedRetry() {
    if (busyRef.current || receiptState !== 'terminal' || !request.current || blocked) return false;
    try { sessionStorage.removeItem(storageKey); }
    catch { return false; }
    request.current = null; setReceiptState('unknown'); setReviewed(false); setView(null); setResults([]); setError(''); setOutcome('The confirmed operator browser retry was closed. Server operation and history remain preserved. Inspect current state before starting another action.');
    return true;
  }
  async function inspect(id: string, target = 'deployment') {
    if (busyRef.current) return;
    if (request.current && (target !== 'deployment' || request.current.instance !== id)) { setError('Inspect the exact retained instance before continuing. Other requests remain paused.'); return; }
    const controller = new AbortController(); inspection.current?.abort(); inspection.current = controller; busyRef.current = true;
    setBusy(true); setError(''); setView(null); setResults([]); setTruncated(false); setReviewed(false);
    try {
      const response = await fetch(`/api/operator/${target === 'organization' ? 'organizations' : 'deployments'}/${id}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json(); if (inspection.current !== controller || controller.signal.aborted) return; if (!response.ok) throw new Error(errorMessage(value));
      if (target === 'deployment') { if (!tenant(value, id)) throw new Error('The tenant response could not be verified.'); setView(value); }
      else { if (!object(value) || !Array.isArray(value.items) || value.items.length > 100 || !value.items.every(item => object(item) && typeof item.instanceId === 'string' && uuid.test(item.instanceId) && typeof item.name === 'string' && typeof item.state === 'string') || typeof value.hasMore !== 'boolean') throw new Error('The organization response could not be verified.'); setResults(value.items as { instanceId: string; name: string; state: string }[]); setTruncated(value.hasMore); }
    } catch { if (inspection.current === controller && !controller.signal.aborted) { setView(null); setResults([]); setReviewed(false); setError('Support scope could not be confirmed. Refresh access and inspect this exact instance before continuing.'); } } finally { if (inspection.current === controller) { inspection.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  async function action(event: FormEvent) {
    event.preventDefault(); if (!view || view.kind !== 'agent' || view.state !== 'ready' || !reviewed || role !== 'operate' || blocked || busyRef.current || request.current && receiptState !== 'absent') return;
    const intent = request.current ?? { key: crypto.randomUUID(), instance: view.instanceId, kind: actionKind, state: desired, reason };
    if (!operatorIntent(intent) || intent.instance !== view.instanceId) { setError('This exact lifecycle request could not be validated.'); return; }
    try { sessionStorage.setItem(storageKey, JSON.stringify(intent)); }
    catch { setReviewed(false); setError('Enable session storage before recording an operator action. No request was submitted.'); return; }
    request.current = intent; busyRef.current = true; inspection.current?.abort(); inspection.current = null;
    const controller = new AbortController(); mutation.current = controller;
    setBusy(true); setReviewed(false); setReceiptState('unknown'); setError(''); setOutcome('');
    try {
      const response = await fetch(`/api/operator/deployments/${intent.instance}/lifecycle`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ idempotencyKey: intent.key, kind: intent.kind, state: intent.state, reason: intent.reason }), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (mutation.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !operatorOperation(value, intent)) throw Error('Unconfirmed operation');
      setOutcome(`Operation ${value.id}: ${value.state}. Check the exact receipt before closing this browser retry.`);
    } catch { if (mutation.current === controller && !controller.signal.aborted) { setView(null); setReviewed(false); setError('The operator action could not be confirmed. Its exact request is retained. Check its receipt and inspect the original instance before reviewing a retry.'); } }
    finally { if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) setBusy(false); } }
  }
  if (!role) return request.current || blocked ? <section className="support-workspace" aria-label="Retained operator access"><p role="alert">Current operator access could not be confirmed. Retained browser metadata remains preserved and no action will be submitted automatically.</p><button type="button" onClick={() => { void loadRole(); }}>Check operator access</button></section> : null;
  return <section className="support-workspace" aria-labelledby="support-heading"><h3 id="support-heading">Operator support</h3><p>Role: {role === 'operate' ? 'Inspection and lifecycle actions' : 'Inspection only'}. Inspection is audited. Customer sign-in and data are never impersonated.</p>
    <form onSubmit={event => { event.preventDefault(); void inspect(reference, kind); }}><label htmlFor="support-kind">Reference type</label><select id="support-kind" value={kind} onChange={event => setKind(event.target.value)} disabled={busy || request.current !== null}><option value="deployment">Instance</option><option value="organization">Organization</option></select><label htmlFor="support-reference">Exact reference UUID</label><input id="support-reference" value={reference} onChange={event => setReference(event.target.value.trim().toLowerCase())} maxLength={36} required disabled={busy || request.current !== null}/><button disabled={busy || !uuid.test(reference)}>Inspect</button></form>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}{outcome && <p role="status" ref={result} tabIndex={-1}>{outcome}</p>}
    <OperatorLifecycleRecovery key={`${ownerId}:${request.current?.key ?? 'invalid'}`} intent={request.current} blocked={blocked} disabled={busy} role={role} onState={setReceiptState} onRole={setRole} onUnconfirmed={() => { setView(null); setResults([]); setReviewed(false); }} onClose={closeConfirmedRetry}/>
    {results.length > 0 && <ul>{results.map(item => <li key={item.instanceId}>{item.name}: {item.state} <button type="button" disabled={busy} onClick={() => { void inspect(item.instanceId); }}>Inspect instance</button></li>)}</ul>}{truncated && <p>Only the first 100 instances are shown. Inspect an exact instance reference for other records.</p>}
    {view && <><h4>{view.name}</h4><p>Instance {view.instanceId}. Organization {view.organizationId}. {view.state}.</p><dl><div><dt>Hostname</dt><dd>{view.managedHostname}</dd></div><div><dt>Billing observation</dt><dd>{object(view.billing) && view.billing.source === 'local_fixture' ? 'Local fixture: ' : ''}{state(view.billing)}</dd></div><div><dt>Local hosting grant</dt><dd>{state(view.hostingGrant)}</dd></div></dl>
      {object(view.usage) && <p>Observed storage: {typeof view.usage.dataBytes === 'number' && typeof view.usage.blobBytes === 'number' ? ((view.usage.dataBytes + view.usage.blobBytes) / 1048576).toFixed(2) : 'Unknown'} MiB. Measured {typeof view.usage.measuredAt === 'string' ? new Date(view.usage.measuredAt).toLocaleString() : 'Unknown'}.</p>}
      <p>Domain proof: {state(view.domainProof)}. Management recovery: {typeof view.managementRecovery === 'string' ? view.managementRecovery : 'Unknown'}.</p>
      {Array.isArray(view.lifecycle) && <ul>{view.lifecycle.filter(object).map((item, index) => <li key={index}>{typeof item.kind === 'string' ? item.kind : 'Lifecycle'}: {typeof item.desiredState === 'string' ? item.desiredState : 'Unknown'}, {state(item)}.</li>)}</ul>}
      {Array.isArray(view.incidents) && <ul>{view.incidents.filter(object).map((item, index) => <li key={index}>{typeof item.kind === 'string' ? item.kind.replaceAll('_', ' ') : 'Observation'}: {item.active === true ? 'Active' : 'Resolved'}. Checked {typeof item.lastObservedAt === 'string' ? new Date(item.lastObservedAt).toLocaleString() : 'Unknown'}.</li>)}</ul>}
      <LifecycleActivity key={view.instanceId} deploymentId={view.instanceId} operator/>
      {role === 'operate' && !blocked && (request.current === null || receiptState === 'absent') && view.kind === 'agent' && view.state === 'ready' && <form onSubmit={event => { void action(event); }}><label htmlFor="support-action-kind">Lifecycle target</label><select id="support-action-kind" value={actionKind} disabled={busy || request.current !== null} onChange={event => { setActionKind(event.target.value); setReviewed(false); }}><option value="service">Service runtime</option><option value="account">Native account</option></select><label htmlFor="support-state">Action</label><select id="support-state" value={desired} disabled={busy || request.current !== null} onChange={event => { setDesired(event.target.value); setReviewed(false); }}><option value="suspended">Suspend</option><option value="running">Resume</option></select><label htmlFor="support-reason">Reason</label><select id="support-reason" value={reason} disabled={busy || request.current !== null} onChange={event => { setReason(event.target.value); setReviewed(false); }}><option value="maintenance">Maintenance</option><option value="safety_review">Safety review</option><option value="resource_pressure">Resource pressure</option></select><label><input type="checkbox" checked={reviewed} disabled={busy} onChange={event => setReviewed(event.target.checked)}/> I reviewed the target instance and current activity.</label><button disabled={busy || !reviewed}>{request.current ? 'Retry this request' : 'Record lifecycle action'}</button></form>}
    </>}
  </section>;
}
