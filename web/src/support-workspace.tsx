import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function errorMessage(value: unknown): string { return object(value) && typeof value.error === 'string' ? value.error : 'Support evidence is temporarily unavailable.'; }
type HistoryItem = { id: string; kind: string; generation: number; state: string; desiredState: string; reason: string; actor: string };
type History = { items: HistoryItem[]; hasMore: boolean; nextBefore: number | null };
function history(value: unknown): value is History {
  return object(value) && Array.isArray(value.items) && value.items.every(item => object(item) && typeof item.id === 'string' && typeof item.kind === 'string' && typeof item.generation === 'number' && typeof item.state === 'string' && typeof item.desiredState === 'string' && typeof item.reason === 'string' && typeof item.actor === 'string') && typeof value.hasMore === 'boolean' && (value.nextBefore === null || typeof value.nextBefore === 'number');
}
export function LifecycleActivity({ deploymentId, operator = false }: { deploymentId: string; operator?: boolean }) {
  const [page, setPage] = useState<History | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  async function load(before?: number) {
    setBusy(true); setError('');
    try {
      const response = await fetch(`/api/${operator ? 'operator/' : ''}deployments/${deploymentId}/activity${before ? `?before=${before}` : ''}`, { cache: 'no-store', signal: AbortSignal.timeout(8000) });
      const value: unknown = await response.json(); if (!response.ok) throw new Error(errorMessage(value)); if (!history(value)) throw new Error('The activity response could not be verified.'); setPage(value);
    } catch (error) { setError(error instanceof Error ? error.message : 'Activity is unavailable.'); } finally { setBusy(false); }
  }
  return <details><summary>Service and account activity</summary><button type="button" disabled={busy} onClick={() => { void load(); }}>Load latest activity</button>{error && <p role="alert">{error}</p>}{page && <><ol>{page.items.map(item => <li key={item.id}>Generation {item.generation}: {item.kind} {item.desiredState}, {item.state}. Reason: {item.reason.replaceAll('_', ' ')}. {operator && <>Actor: {item.actor}.</>}</li>)}</ol>{page.items.length === 0 && <p>No service or account actions recorded.</p>}{page.hasMore && page.nextBefore !== null && <button type="button" disabled={busy} onClick={() => { void load(page.nextBefore!); }}>Load older activity</button>}</>}</details>;
}
type Tenant = { instanceId: string; organizationId: string; name: string; state: string; kind: string; managedHostname: string; billing: unknown; hostingGrant: unknown; lifecycle: unknown; incidents: unknown; usage: unknown; domainProof: unknown; managementRecovery: unknown };
function tenant(value: unknown): value is Tenant { return object(value) && typeof value.instanceId === 'string' && uuid.test(value.instanceId) && typeof value.organizationId === 'string' && typeof value.name === 'string' && typeof value.state === 'string' && typeof value.kind === 'string' && typeof value.managedHostname === 'string'; }
function state(value: unknown): string { return object(value) && typeof value.state === 'string' ? value.state : 'Unconfigured'; }
export function SupportWorkspace({ csrfToken, ownerId }: { csrfToken: string; ownerId: string }) {
  const [role, setRole] = useState<'inspect' | 'operate' | null>(null), [reference, setReference] = useState(''), [kind, setKind] = useState('deployment');
  const [view, setView] = useState<Tenant | null>(null), [results, setResults] = useState<{ instanceId: string; name: string; state: string }[]>([]), [truncated, setTruncated] = useState(false);
  const [error, setError] = useState(''), [busy, setBusy] = useState(false), [actionKind, setActionKind] = useState('service'), [desired, setDesired] = useState('suspended'), [reason, setReason] = useState('maintenance'), [reviewed, setReviewed] = useState(false), [outcome, setOutcome] = useState('');
  const request = useRef<{ key: string; instance: string; kind: string; state: string; reason: string } | null>(null);
  const storageKey = `pds-support-intent:${ownerId}`;
  useEffect(() => {
    const controller = new AbortController(); setRole(null); request.current = null;
    try { const raw = sessionStorage.getItem(storageKey); if (raw) { const value: unknown = JSON.parse(raw); if (!object(value) || typeof value.key !== 'string' || !uuid.test(value.key) || typeof value.instance !== 'string' || !uuid.test(value.instance) || (value.kind !== 'service' && value.kind !== 'account') || (value.state !== 'running' && value.state !== 'suspended') || (value.reason !== 'maintenance' && value.reason !== 'safety_review' && value.reason !== 'resource_pressure')) throw new Error('Retained action could not be verified.'); request.current = { key: value.key, instance: value.instance, kind: value.kind, state: value.state, reason: value.reason }; setActionKind(value.kind); setDesired(value.state); setReason(value.reason); setReference(value.instance); setOutcome('A retained lifecycle request is available. Inspect that instance and its activity before retrying.'); } } catch { setError('Retained support request is unavailable. Inspect history before starting another action.'); }
    void fetch('/api/operator/session', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) }).then(async response => { if (!response.ok) return; const value: unknown = await response.json(); if (!controller.signal.aborted && object(value) && (value.role === 'inspect' || value.role === 'operate')) setRole(value.role); }).catch(() => undefined);
    return () => controller.abort();
  }, [ownerId, storageKey]);
  async function inspect(id: string, target = 'deployment') {
    if (request.current && (target !== 'deployment' || request.current.instance !== id)) { setError('Review the retained instance request or explicitly start another action first.'); return; }
    setBusy(true); setError(''); setView(null); setResults([]); setTruncated(false); setReviewed(false);
    try {
      const response = await fetch(`/api/operator/${target === 'organization' ? 'organizations' : 'deployments'}/${id}`, { cache: 'no-store', signal: AbortSignal.timeout(8000) });
      const value: unknown = await response.json(); if (!response.ok) throw new Error(errorMessage(value));
      if (target === 'deployment') { if (!tenant(value)) throw new Error('The tenant response could not be verified.'); setView(value); }
      else { if (!object(value) || !Array.isArray(value.items) || !value.items.every(item => object(item) && typeof item.instanceId === 'string' && uuid.test(item.instanceId) && typeof item.name === 'string' && typeof item.state === 'string') || typeof value.hasMore !== 'boolean') throw new Error('The organization response could not be verified.'); setResults(value.items as { instanceId: string; name: string; state: string }[]); setTruncated(value.hasMore); }
    } catch (error) { setError(error instanceof Error ? error.message : 'Inspection failed.'); } finally { setBusy(false); }
  }
  async function action(event: FormEvent) {
    event.preventDefault(); if (!view || !reviewed || role !== 'operate') return;
    const intent = request.current ?? { key: crypto.randomUUID(), instance: view.instanceId, kind: actionKind, state: desired, reason }; request.current = intent;
    setBusy(true); setError('');
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(intent));
      const response = await fetch(`/api/operator/deployments/${intent.instance}/lifecycle`, { method: 'POST', headers: { Origin: window.location.origin, 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ idempotencyKey: intent.key, kind: intent.kind, state: intent.state, reason: intent.reason }), signal: AbortSignal.timeout(8000) });
      const value: unknown = await response.json(); if (!response.ok) throw new Error(errorMessage(value)); if (!object(value) || typeof value.id !== 'string' || typeof value.state !== 'string') throw new Error('The operation response could not be verified.'); setOutcome(`Operation ${value.id}: ${value.state}. Load activity to check its outcome.`);
    } catch (error) { setError(error instanceof Error ? error.message : 'Action result is unavailable. Retry this request.'); } finally { setBusy(false); }
  }
  if (!role) return null;
  return <section className="support-workspace" aria-labelledby="support-heading"><h3 id="support-heading">Operator support</h3><p>Role: {role === 'operate' ? 'Inspection and lifecycle actions' : 'Inspection only'}. Inspection is audited. Customer sign-in and data are never impersonated.</p>
    <form onSubmit={event => { event.preventDefault(); void inspect(reference, kind); }}><label htmlFor="support-kind">Reference type</label><select id="support-kind" value={kind} onChange={event => setKind(event.target.value)} disabled={busy}><option value="deployment">Instance</option><option value="organization">Organization</option></select><label htmlFor="support-reference">Exact reference UUID</label><input id="support-reference" value={reference} onChange={event => setReference(event.target.value.trim().toLowerCase())} maxLength={36} required disabled={busy}/><button disabled={busy || !uuid.test(reference)}>Inspect</button></form>
    {error && <p role="alert">{error}</p>}{!view && request.current && <button type="button" disabled={busy} onClick={() => { sessionStorage.removeItem(storageKey); request.current = null; setReviewed(false); setOutcome(''); }}>Start another action after reviewing history</button>}{results.length > 0 && <ul>{results.map(item => <li key={item.instanceId}>{item.name}: {item.state} <button type="button" disabled={busy} onClick={() => { void inspect(item.instanceId); }}>Inspect instance</button></li>)}</ul>}{truncated && <p>Only the first 100 instances are shown. Inspect an exact instance reference for other records.</p>}
    {view && <><h4>{view.name}</h4><p>Instance {view.instanceId}. Organization {view.organizationId}. {view.state}.</p><dl><div><dt>Hostname</dt><dd>{view.managedHostname}</dd></div><div><dt>Billing observation</dt><dd>{object(view.billing) && view.billing.source === 'local_fixture' ? 'Local fixture: ' : ''}{state(view.billing)}</dd></div><div><dt>Local hosting grant</dt><dd>{state(view.hostingGrant)}</dd></div></dl>
      {object(view.usage) && <p>Observed storage: {typeof view.usage.dataBytes === 'number' && typeof view.usage.blobBytes === 'number' ? ((view.usage.dataBytes + view.usage.blobBytes) / 1048576).toFixed(2) : 'Unknown'} MiB. Measured {typeof view.usage.measuredAt === 'string' ? new Date(view.usage.measuredAt).toLocaleString() : 'Unknown'}.</p>}
      <p>Domain proof: {state(view.domainProof)}. Management recovery: {typeof view.managementRecovery === 'string' ? view.managementRecovery : 'Unknown'}.</p>
      {Array.isArray(view.lifecycle) && <ul>{view.lifecycle.filter(object).map((item, index) => <li key={index}>{typeof item.kind === 'string' ? item.kind : 'Lifecycle'}: {typeof item.desiredState === 'string' ? item.desiredState : 'Unknown'}, {state(item)}.</li>)}</ul>}
      {Array.isArray(view.incidents) && <ul>{view.incidents.filter(object).map((item, index) => <li key={index}>{typeof item.kind === 'string' ? item.kind.replaceAll('_', ' ') : 'Observation'}: {item.active === true ? 'Active' : 'Resolved'}. Checked {typeof item.lastObservedAt === 'string' ? new Date(item.lastObservedAt).toLocaleString() : 'Unknown'}.</li>)}</ul>}
      <LifecycleActivity key={view.instanceId} deploymentId={view.instanceId} operator/>
      {role === 'operate' && view.kind === 'agent' && view.state === 'ready' && <form onSubmit={event => { void action(event); }}><label htmlFor="support-action-kind">Lifecycle target</label><select id="support-action-kind" value={actionKind} disabled={busy || request.current !== null} onChange={event => { setActionKind(event.target.value); setReviewed(false); }}><option value="service">Service runtime</option><option value="account">Native account</option></select><label htmlFor="support-state">Action</label><select id="support-state" value={desired} disabled={busy || request.current !== null} onChange={event => { setDesired(event.target.value); setReviewed(false); }}><option value="suspended">Suspend</option><option value="running">Resume</option></select><label htmlFor="support-reason">Reason</label><select id="support-reason" value={reason} disabled={busy || request.current !== null} onChange={event => { setReason(event.target.value); setReviewed(false); }}><option value="maintenance">Maintenance</option><option value="safety_review">Safety review</option><option value="resource_pressure">Resource pressure</option></select><label><input type="checkbox" checked={reviewed} disabled={busy} onChange={event => setReviewed(event.target.checked)}/> I reviewed the target instance and current activity.</label><button disabled={busy || !reviewed}>{request.current ? 'Retry this request' : 'Record lifecycle action'}</button>{request.current && <button type="button" disabled={busy} onClick={() => { sessionStorage.removeItem(storageKey); request.current = null; setReviewed(false); setOutcome(''); }}>Start another action</button>}{outcome && <p role="status">{outcome}</p>}</form>}
    </>}
  </section>;
}
