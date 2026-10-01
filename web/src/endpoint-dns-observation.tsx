import { useEffect, useRef, useState } from 'react';

type State = 'challenge_match' | 'pending_dns' | 'answer_too_large' | 'unavailable' | 'local_namespace' | 'cancelled' | 'expired';
type Observation = { choiceId: string; hostname: string; state: State; matched: boolean; claimReserved: false; canActivate: false; checkedAt: string; message: string };
const states: State[] = ['challenge_match', 'pending_dns', 'answer_too_large', 'unavailable', 'local_namespace', 'cancelled', 'expired'];
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null; }
function observation(v: unknown, id: string, hostname: string): v is Observation {
  return object(v) && v.choiceId === id && v.hostname === hostname && states.includes(v.state as State) && v.matched === (v.state === 'challenge_match') && v.claimReserved === false && v.canActivate === false && typeof v.checkedAt === 'string' && Number.isFinite(Date.parse(v.checkedAt)) && typeof v.message === 'string' && v.message.length <= 512;
}

export function EndpointDNSObservation({ organizationId, choiceId, hostname, csrfToken, disabled, onAccessDenied }: { organizationId: string; choiceId: string; hostname: string; csrfToken: string; disabled: boolean; onAccessDenied: () => void }) {
  const [value, setValue] = useState<Observation | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (value) status.current?.focus(); }, [value]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => {
    active.current?.abort(); active.current = null; setValue(null); setError(''); setBusy(false);
    return () => { active.current?.abort(); active.current = null; };
  }, [organizationId, choiceId, hostname, disabled]);
  async function check() {
    if (busy || disabled) return;
    const controller = new AbortController(); active.current = controller; setValue(null); setError(''); setBusy(true);
    try {
      const response = await fetch(`/api/organizations/${organizationId}/endpoint-choices/${choiceId}/dns-observation`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: '{}', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const result: unknown = await response.json();
      if (active.current !== controller || controller.signal.aborted) return;
      if (response.status === 401 || response.status === 403 || response.status === 404) { onAccessDenied(); return; }
      if (!response.ok || !observation(result, choiceId, hostname)) { setError('The DNS observation could not be confirmed. Refresh the current saved choice and retry.'); return; }
      setValue(result);
    } catch { if (active.current === controller && !controller.signal.aborted) setError('The DNS observation could not be confirmed. Refresh the current saved choice and retry.'); }
    finally { if (active.current === controller && !controller.signal.aborted) setBusy(false); }
  }
  return <div className="endpoint-dns-observation" aria-busy={busy}>
    <button type="button" disabled={disabled || busy} onClick={() => { void check(); }}>{busy ? 'Checking saved TXT challenge…' : 'Check saved TXT challenge'}</button>
    <p>This read checks the exact saved challenge. It reserves no domain and records no ownership or activation. Local test names send no DNS query.</p>
    {!disabled && error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {!disabled && value && <p role="status" ref={status} tabIndex={-1}>{value.message} Checked {new Date(value.checkedAt).toLocaleString()}. No exclusive claim or activation was recorded.</p>}
  </div>;
}
