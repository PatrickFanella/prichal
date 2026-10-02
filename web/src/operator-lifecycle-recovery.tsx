import { useEffect, useRef, useState } from 'react';
import { operatorReceipt, type OperatorIntent, type OperatorReceipt, type ReceiptState } from './operator-lifecycle-contract';

export function OperatorLifecycleRecovery({ intent, blocked, disabled, role, onState, onRole, onUnconfirmed, onClose }: {
  intent: OperatorIntent | null; blocked: boolean; disabled: boolean; role: 'inspect' | 'operate';
  onState: (state: ReceiptState) => void; onRole: (role: 'inspect' | 'operate') => void; onUnconfirmed: () => void; onClose: () => boolean;
}) {
  const [value, setValue] = useState<OperatorReceipt | null>(null), [loaded, setLoaded] = useState(false), [loading, setLoading] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [review, setReview] = useState(false), [understood, setUnderstood] = useState(false);
  const read = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null), title = useRef<HTMLHeadingElement | null>(null), launcher = useRef<HTMLButtonElement | null>(null);
  useEffect(() => { if (loaded && (value || notice)) status.current?.focus(); }, [loaded, value, notice]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { if (review) title.current?.focus(); }, [review]);
  useEffect(() => { setReview(false); setUnderstood(false); }, [value?.state, intent?.key, role]);
  async function refresh() {
    if (!intent || disabled || blocked) return;
    read.current?.abort(); const controller = new AbortController(); read.current = controller;
    setLoading(true); setLoaded(false); setValue(null); setError(''); setNotice(''); setReview(false); setUnderstood(false); onState('unknown');
    try {
      const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]);
      const [permission, response] = await Promise.all([fetch('/api/operator/session', { cache: 'no-store', signal }), fetch(`/api/operator/deployments/${intent.instance}/lifecycle-requests/${intent.key}`, { cache: 'no-store', signal })]);
      const p: unknown = await permission.json();
      if (read.current !== controller || controller.signal.aborted) return;
      if (!permission.ok || typeof p !== 'object' || !p || Array.isArray(p) || !('role' in p) || (p.role !== 'inspect' && p.role !== 'operate')) throw Error('Operator access unconfirmed');
      onRole(p.role);
      if (response.status === 404) { setLoaded(true); setNotice('No receipt was confirmed for this original request. Inspect the exact instance before reviewing an exact retry. New requests remain paused.'); onState('absent'); return; }
      const receipt: unknown = await response.json();
      if (read.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !operatorReceipt(receipt, intent)) throw Error('Receipt unconfirmed');
      setValue(receipt); setLoaded(true); onState(receipt.state === 'pending' ? 'pending' : 'terminal');
    } catch { if (read.current === controller && !controller.signal.aborted) { setValue(null); setLoaded(false); setError('The exact operator receipt could not be confirmed. Inspect current access and refresh this request before continuing.'); onState('unknown'); onUnconfirmed(); } }
    finally { if (read.current === controller && !controller.signal.aborted) setLoading(false); }
  }
  useEffect(() => {
    read.current?.abort(); read.current = null; setValue(null); setLoaded(false); setLoading(false); setReview(false); setUnderstood(false); onState('unknown');
    if (!disabled && !blocked && intent) void refresh();
    return () => { read.current?.abort(); read.current = null; };
  }, [intent?.key, intent?.instance, disabled, blocked, role]);
  if (!intent && !blocked) return null;
  return <section className="operator-lifecycle-recovery" aria-label="Retained operator request" aria-busy={loading}>
    <h4>Retained operator request</h4>
    {intent && <p>Request {intent.key} for instance {intent.instance}: {intent.kind} {intent.state}, {intent.reason.replaceAll('_', ' ')}. Reload checks its receipt and never submits it automatically.</p>}
    {blocked && <p role="alert">Retained metadata could not be validated. New lifecycle requests are paused. Inspect instance history and contact support for browser-state recovery; no server action has been cleared.</p>}
    {intent && <button type="button" disabled={disabled || loading || blocked} onClick={() => { void refresh(); }}>Check exact operator receipt</button>}
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {loaded && (value || notice) && <p role="status" ref={status} tabIndex={-1}>{value ? `Operation ${value.id}, generation ${value.generation}: ${value.state}. Updated ${new Date(value.updatedAt).toLocaleString()}. ${value.state === 'pending' ? 'Keep this request while its outcome is pending.' : 'Inspect current history before closing the browser retry.'}` : notice}</p>}
    {loaded && value && value.state !== 'pending' && !review && <button type="button" ref={launcher} disabled={disabled || loading} onClick={() => { setReview(true); setUnderstood(false); }}>Review closing operator browser retry</button>}
    {review && loaded && value && value.state !== 'pending' && !disabled && <div className="endpoint-choice-review">
      <h5 ref={title} tabIndex={-1}>Review closing operator browser retry</h5><p>Remove only this browser metadata for the confirmed {value.state} receipt. The server operation and its history remain preserved. No native action is cancelled or reversed.</p>
      <label><input type="checkbox" checked={understood} disabled={loading} onChange={event => setUnderstood(event.target.checked)} /> I reviewed this exact receipt and understand that only browser metadata will be removed.</label>
      <button type="button" disabled={loading || !understood} onClick={() => { if (understood && onClose()) { setReview(false); setUnderstood(false); } else setError('Browser metadata could not be cleared. The server receipt remains preserved.'); }}>Close confirmed operator browser retry</button>
      <button type="button" onClick={() => { setReview(false); setUnderstood(false); window.setTimeout(() => launcher.current?.focus(), 0); }}>Close review</button>
    </div>}
  </section>;
}
