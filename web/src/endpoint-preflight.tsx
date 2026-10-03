import { useEffect, useRef, useState } from 'react';
import { EndpointChoice } from './endpoint-choice';

type Result = { hostname: string; applicationHosts: string[]; state: 'planning_only' | 'existing_native_requires_review'; canActivate: false; choiceSaved: false; remainingGates: string[]; message: string };
const gates: Record<string, string> = { domain_ownership: 'Prove domain ownership', routing_dns: 'Confirm supported DNS routing', certificate: 'Qualify HTTPS certificate', persisted_choice: 'Save the approved address before enrollment', supported_activation: 'Activate through supported host provisioning', existing_native_review: 'Review migration or replacement of existing native hosting' };
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null; }
function result(v: unknown): v is Result {
  return object(v) && typeof v.hostname === 'string' && v.hostname.length <= 253 && Array.isArray(v.applicationHosts) && v.applicationHosts.length <= 8 && v.applicationHosts.every(x => typeof x === 'string') && (v.state === 'planning_only' || v.state === 'existing_native_requires_review') && v.canActivate === false && v.choiceSaved === false && Array.isArray(v.remainingGates) && v.remainingGates.length <= 6 && v.remainingGates.every(x => typeof x === 'string' && Object.hasOwn(gates, x)) && typeof v.message === 'string';
}
export function EndpointPreflight({ organizationId, ownerId, csrfToken }: { organizationId: string; ownerId: string; csrfToken: string }) {
  const [hostname, setHostname] = useState(''), [apps, setApps] = useState(''), [checked, setChecked] = useState<Result | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => () => { active.current?.abort(); active.current = null; }, []);
  useEffect(() => { if (checked) status.current?.focus(); }, [checked]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  async function check() {
    active.current?.abort(); const controller = new AbortController(); active.current = controller; setBusy(true); setChecked(null); setError('');
    try {
      const response = await fetch(`/api/organizations/${organizationId}/endpoint-preflight`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify({ hostname, applicationHosts: apps.split(/\s+/).filter(Boolean) }), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (active.current !== controller) return;
      if (!response.ok || !result(value)) { setError(object(value) && typeof value.error === 'string' ? value.error : 'The address plan could not be checked. Refresh current organization access and try again.'); return; }
      setChecked(value);
    } catch { if (active.current === controller) setError('The address plan could not be checked. Refresh current organization access and try again.'); }
    finally { if (active.current === controller) setBusy(false); }
  }
  function changed(update: () => void) { update(); setChecked(null); setError(''); }
  return <details className="endpoint-preflight" aria-busy={busy}><summary>Plan a custom server address</summary><p>Your handle is your username. A server address (PDS address) is where your account's home server is reached. This check saves no choice and makes no hosting change.</p><p>Use a separate registrable domain from app/OAuth pages, including our management site. Sibling subdomains are not sufficient. List all app hosts you use or plan to use; leave the list empty if there are none.</p>
    <form onSubmit={event => { event.preventDefault(); void check(); }}><label htmlFor={`endpoint-host-${organizationId}`}>Proposed server hostname</label><input id={`endpoint-host-${organizationId}`} value={hostname} onChange={event => changed(() => setHostname(event.target.value))} maxLength={253} disabled={busy} required placeholder="pds.your-hosting-domain.net"/>
      <label htmlFor={`endpoint-apps-${organizationId}`}>Application/OAuth hostnames, separated by spaces</label><textarea id={`endpoint-apps-${organizationId}`} value={apps} onChange={event => changed(() => setApps(event.target.value))} maxLength={2040} disabled={busy} rows={2}/><button type="submit" disabled={busy || !hostname}>{busy ? 'Checking address plan…' : 'Check address plan'}</button></form>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {checked && <><p role="status" ref={status} tabIndex={-1}>{checked.hostname}: {checked.message}</p><p>Address is not saved or activated. Domain ownership, DNS and TLS are unverified.</p><ul>{checked.remainingGates.map(gate => <li key={gate}>{gates[gate]}</li>)}</ul></>}
    <EndpointChoice organizationId={organizationId} ownerId={ownerId} csrfToken={csrfToken} plan={checked}/>
  </details>;
}
