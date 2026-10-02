import { useEffect, useState } from 'react';
import { OwnerWorkspace } from './owner-workspace';

type Status = { kind: 'checking' } | { kind: 'ready'; schemaVersion: number } | { kind: 'unavailable' };
function isReadiness(value: unknown): value is { ready: boolean; database: string; schemaVersion: number } {
  if (typeof value !== 'object' || value === null) return false;
  return 'ready' in value && typeof value.ready === 'boolean' &&
    'database' in value && typeof value.database === 'string' &&
    'schemaVersion' in value && typeof value.schemaVersion === 'number';
}
export function Workspace() {
  const [status, setStatus] = useState<Status>({ kind: 'checking' });
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const response = await fetch('/api/readiness', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(4000)]) });
        const body: unknown = await response.json();
        if (!controller.signal.aborted) setStatus(response.ok && isReadiness(body) && body.ready && body.database === 'ready'
          ? { kind: 'ready', schemaVersion: body.schemaVersion } : { kind: 'unavailable' });
      } catch {
        if (!controller.signal.aborted) setStatus({ kind: 'unavailable' });
      }
      if (!controller.signal.aborted) timer = setTimeout(() => { void poll(); }, 3000);
    }
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);
  const label = status.kind === 'ready' ? 'Service available' : status.kind === 'checking' ? 'Checking service' : 'Service unavailable';
  return <div className="workspace">
    <header><a className="brand" href="/" aria-label="PDS hosting home"><span className="brand-mark">p.</span> PDS hosting</a><span className="environment">Local workspace</span></header>
    <main>
      <p className="eyebrow">YOUR OWN CORNER OF THE NETWORK</p>
      <h1>Your data.<br/>Your address.</h1>
      <p className="intro">A home for your identity, posts, and media. Managed hosting for creators and organizations.</p>
      <OwnerWorkspace/>
      <section className="service-card" aria-labelledby="service-title">
        <div className="card-heading"><h2 id="service-title">Workspace status</h2><span className={`indicator ${status.kind}`} aria-hidden="true"/></div>
        <p className="status" role="status" aria-live="polite">{label}</p>
        <p className="status-detail">{status.kind === 'ready' ? 'The management service and database are responding.' : status.kind === 'checking' ? 'Connecting to the management service…' : 'Service readiness could not be confirmed. This page will check again automatically.'}</p>
        <dl><div><dt>Environment</dt><dd>Local development</dd></div><div><dt>Database</dt><dd>{status.kind === 'ready' ? 'Connected' : status.kind === 'checking' ? 'Checking' : 'Unverified'}</dd></div><div><dt>Schema</dt><dd>{status.kind === 'ready' ? `Version ${status.schemaVersion}` : 'Unverified'}</dd></div></dl>
      </section>
      <aside className="next"><span>UP NEXT</span><p>Public ingress and account enrollment are the next steps. Public hosting is not available yet.</p></aside>
    </main>
    <footer>Standalone PDS hosting<span>Local foundation · Private alpha in progress</span></footer>
  </div>;
}
