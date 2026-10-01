import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';

type Credential = { id: string; createdAt: string; expiresAt: string };
export function CLIAccess({ csrfToken }: { csrfToken: string }) {
  const [code, setCode] = useState('');
  const [reviewed, setReviewed] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [unavailable, setUnavailable] = useState(false);
  async function load(signal?: AbortSignal) {
    try {
      const response = await fetch('/api/cli/credentials', { cache: 'no-store', signal: signal ?? AbortSignal.timeout(5000) });
      const result: unknown = await response.json();
      if (!response.ok || !Array.isArray(result) || !result.every(v => v && typeof v.id === 'string' && typeof v.expiresAt === 'string' && typeof v.createdAt === 'string')) throw new Error();
      if (!signal?.aborted) { setCredentials(result as Credential[]); setUnavailable(false); }
    } catch { if (!signal?.aborted) setUnavailable(true); }
  }
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, []);
  async function request(path: string, body?: unknown) {
    const response = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: body ? JSON.stringify(body) : null, signal: AbortSignal.timeout(5000) });
    const result: unknown = await response.json();
    if (!response.ok) throw new Error(typeof result === 'object' && result !== null && 'error' in result && typeof result.error === 'string' ? result.error : 'Request could not be confirmed.');
    return result;
  }
  async function review(event: FormEvent) {
    event.preventDefault(); setBusy(true); setMessage('');
    try { await request('/api/cli/review', { userCode: code }); setReviewed(code.trim().toUpperCase()); }
    catch (error) { setMessage(error instanceof Error ? error.message : 'Review could not be confirmed.'); }
    finally { setBusy(false); }
  }
  async function decide(approved: boolean) {
    setBusy(true); setMessage('');
    try { await request('/api/cli/approve', { userCode: reviewed, approved }); setCode(''); setReviewed(''); setMessage(approved ? 'Approved. Return to your CLI to finish sign-in.' : 'Request denied.'); await load(); }
    catch (error) { setMessage(error instanceof Error ? error.message : 'Approval could not be confirmed.'); }
    finally { setBusy(false); }
  }
  async function revoke(id: string) {
    setBusy(true); setMessage('');
    try { await request(`/api/cli/credentials/${id}/revoke`); await load(); setMessage('CLI access revoked.'); }
    catch (error) { setMessage(error instanceof Error ? error.message : 'Revocation could not be confirmed.'); }
    finally { setBusy(false); }
  }
  return <section id="cli-access" aria-labelledby="cli-title">
    <h3 id="cli-title">Connect your CLI</h3>
    <p>Start sign-in in your CLI, then enter its code here. Approve only a request you started yourself. Never approve a code someone sent you.</p>
    {reviewed ? <div>
      <p>Confirm that <strong>{reviewed}</strong> matches your CLI.</p>
      <p>This allows the CLI to read your organizations and servers and create deployments for 24 hours.</p>
      <div className="form-row"><button disabled={busy} onClick={() => { void decide(true); }}>Approve CLI access</button><button disabled={busy} onClick={() => { void decide(false); }}>Deny request</button><button disabled={busy} onClick={() => { setReviewed(''); }}>Back</button></div>
    </div> : <form onSubmit={event => { void review(event); }}>
      <label htmlFor="cli-code">Code from your CLI</label>
      <div className="form-row"><input id="cli-code" value={code} onChange={event => { setCode(event.target.value); }} maxLength={9} pattern="[A-Za-z2-7]{4}-[A-Za-z2-7]{4}" placeholder="ABCD-EFGH" autoComplete="off" required disabled={busy}/><button disabled={busy}>Review request</button></div>
    </form>}
    {message && <p role="status">{message}</p>}
    <h4>Active CLI access</h4>
    {unavailable ? <p role="alert">CLI access could not be loaded.</p> : credentials.length ? <ul>{credentials.map(c => <li key={c.id}><span>Approved {new Date(c.createdAt).toLocaleString()}; expires {new Date(c.expiresAt).toLocaleString()}.</span> <button disabled={busy} onClick={() => { void revoke(c.id); }}>Revoke access</button></li>)}</ul> : <p>No active CLI credentials.</p>}
    <button className="text-button" disabled={busy} onClick={() => { void load(); }}>Refresh CLI access</button>
  </section>;
}
