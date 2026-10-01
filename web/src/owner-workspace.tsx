import { useCallback, useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { EndpointPreflight } from './endpoint-preflight';
import { Deployments } from './deployments';
import { SupportWorkspace } from './support-workspace';
import { CLIAccess } from './cli-access';
import { CollaborationWorkspace } from './collaboration-workspace';

type Session = { ownerId: string; displayName: string; csrfToken: string; expiresAt: string };
type Organization = { id: string; name: string };
type Workspace = { kind: 'checking' } | { kind: 'signed-out' } | { kind: 'unavailable' } |
  { kind: 'signed-in'; session: Session; organizations: Organization[] };
function isObject(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function isSession(value: unknown): value is Session {
  return isObject(value) && typeof value.ownerId === 'string' && typeof value.displayName === 'string' &&
    typeof value.csrfToken === 'string' && value.csrfToken.length === 43 && typeof value.expiresAt === 'string' &&
    Number.isFinite(Date.parse(value.expiresAt));
}
function isOrganization(value: unknown): value is Organization {
  return isObject(value) && typeof value.id === 'string' && typeof value.name === 'string';
}
function message(value: unknown): string {
  return isObject(value) && typeof value.error === 'string' ? value.error : 'Your workspace is temporarily unavailable. Try again.';
}
export function OwnerWorkspace() {
  const [workspace, setWorkspace] = useState<Workspace>({ kind: 'checking' });
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const options = { cache: 'no-store', signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(5000)]) : AbortSignal.timeout(5000) } satisfies RequestInit;
      const response = await fetch('/api/session', options);
      if (signal?.aborted) return;
      if (response.status === 401) { setWorkspace({ kind: 'signed-out' }); return; }
      const session: unknown = await response.json();
      if (!response.ok || !isSession(session)) throw new Error('Invalid session response');
      const organizationResponse = await fetch('/api/organizations', options);
      const organizations: unknown = await organizationResponse.json();
      if (!organizationResponse.ok || !Array.isArray(organizations) || !organizations.every(isOrganization)) throw new Error('Invalid organization response');
      if (!signal?.aborted) setWorkspace({ kind: 'signed-in', session, organizations });
    } catch { if (!signal?.aborted) setWorkspace({ kind: 'unavailable' }); }
  }, []);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  async function createOrganization(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (workspace.kind !== 'signed-in') return;
    setBusy(true); setError('');
    try {
      const response = await fetch('/api/organizations', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': workspace.session.csrfToken }, body: JSON.stringify({ name }), signal: AbortSignal.timeout(5000) });
      const result: unknown = await response.json();
      if (response.status === 401) { setWorkspace({ kind: 'signed-out' }); return; }
      if (!response.ok) { setError(message(result)); return; }
      if (!isOrganization(result)) throw new Error('Invalid organization response');
      setName(''); await load();
    } catch { setError('The request could not be confirmed. Refresh your organizations before trying again.'); }
    finally { setBusy(false); }
  }
  async function signOut(all: boolean) {
    if (workspace.kind !== 'signed-in') return;
    setBusy(true); setError('');
    try {
      const response = await fetch(all ? '/api/auth/revoke-all' : '/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': workspace.session.csrfToken }, signal: AbortSignal.timeout(5000) });
      if (response.ok || response.status === 401) setWorkspace({ kind: 'signed-out' });
      else setError(message(await response.json()));
    } catch { setError('Sign-out could not be confirmed. Try again.'); }
    finally { setBusy(false); }
  }
  if (workspace.kind === 'checking') return <section className="owner-card" aria-label="Owner workspace"><p role="status">Checking your sign-in…</p></section>;
  if (workspace.kind === 'signed-out') return <section className="owner-card" aria-labelledby="owner-title"><h2 id="owner-title">Start with your organization</h2><p>Sign in to create a workspace for your servers. Your hosting sign-in is separate from your PDS account.</p><a className="button" href="/api/auth/login">Sign in</a></section>;
  if (workspace.kind === 'unavailable') return <section className="owner-card" aria-label="Owner workspace"><p role="alert">Your workspace is temporarily unavailable.</p><button onClick={() => { void load(); }}>Try again</button></section>;
  return <section className="owner-card" aria-labelledby="owner-title">
    <div className="owner-heading"><div><p className="eyebrow">YOUR WORKSPACE</p><h2 id="owner-title">Welcome, {workspace.session.displayName}</h2></div><button className="text-button" disabled={busy} onClick={() => { void signOut(false); }}>Sign out</button></div>
    {error && <p className="form-error" role="alert">{error}</p>}
    {workspace.organizations.length > 0 ? <ul className="organizations">{workspace.organizations.map(organization => <li key={organization.id}><span className="org-symbol" aria-hidden="true">o.</span><div><h3>{organization.name}</h3><p>You own this organization</p><Deployments organizationId={organization.id} csrfToken={workspace.session.csrfToken}/><EndpointPreflight key={`${workspace.session.ownerId}:${organization.id}`} ownerId={workspace.session.ownerId} organizationId={organization.id} csrfToken={workspace.session.csrfToken}/></div><span className="owner-badge">Owner</span></li>)}</ul> : <p>Create your first organization to begin. Only you can access its workspace.</p>}
    <form onSubmit={event => { void createOrganization(event); }}>
      <label htmlFor="organization-name">Organization name</label>
      <div className="form-row"><input id="organization-name" name="organizationName" value={name} onChange={event => setName(event.target.value)} maxLength={100} required disabled={busy} placeholder="Your studio or community"/><button type="submit" disabled={busy || !name.trim()}>{busy ? 'Working…' : 'Create organization'}</button></div>
    </form>
    <CollaborationWorkspace key={workspace.session.ownerId} ownerId={workspace.session.ownerId} csrfToken={workspace.session.csrfToken} onChanged={() => { void load(); }}/>
    <CLIAccess csrfToken={workspace.session.csrfToken}/>
    <SupportWorkspace csrfToken={workspace.session.csrfToken} ownerId={workspace.session.ownerId}/>
    <div className="session-actions"><button className="text-button" disabled={busy} onClick={() => { void load(); }}>Refresh organizations</button><button className="text-button" disabled={busy} onClick={() => { void signOut(true); }}>Sign out all devices</button></div>
  </section>;
}
