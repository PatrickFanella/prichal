import { MemberInvitations } from './member-invitations';
import { useEffect, useRef, useState } from 'react';

type Instance = { id: string; name: string; state: string; kind: 'agent' | 'test' };
type Page = { items: Instance[]; hasMore: boolean; nextAfter: string | null };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function page(value: unknown): value is Page {
  return object(value) && Array.isArray(value.items) && value.items.length <= 50 && value.items.every(item => object(item) && typeof item.id === 'string' && uuid.test(item.id) && typeof item.name === 'string' && typeof item.state === 'string' && ['queued', 'configuring', 'checking', 'ready', 'failed'].includes(item.state) && (item.kind === 'agent' || item.kind === 'test')) && typeof value.hasMore === 'boolean' && (value.hasMore ? typeof value.nextAfter === 'string' && value.nextAfter.length > 0 && value.nextAfter.length <= 384 : value.nextAfter === null);
}
export function SharedInstanceStatus({ organizationId, csrfToken, manageInvitations = false }: { organizationId: string; csrfToken: string; manageInvitations?: boolean }) {
  const [records, setRecords] = useState<Page | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null), focusResult = useRef(false);
  useEffect(() => { if (records && focusResult.current) { status.current?.focus(); focusResult.current = false; } }, [records]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => {
    setRecords(null); setError(''); focusResult.current = false;
    void load(undefined, false);
    return () => { active.current?.abort(); active.current = null; };
  }, [organizationId]);
  async function load(after?: string, focus = true) {
    active.current?.abort(); const controller = new AbortController(); active.current = controller;
    setBusy(true); setError(''); setRecords(null);
    try {
      const response = await fetch(`/api/team/organizations/${organizationId}/instances/page${after ? `?after=${encodeURIComponent(after)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (active.current !== controller) return;
      if (!response.ok || !page(value)) { setRecords(null); throw new Error('Status unavailable'); }
      focusResult.current = focus; setRecords(value);
    } catch { if (active.current === controller) setError('Shared server status could not be confirmed. Refresh current organization access before loading again.'); }
    finally { if (active.current === controller) setBusy(false); }
  }
  return <section aria-label="Shared server status" aria-busy={busy}><h5>Shared server status</h5><p>Server state is recorded deployment metadata. Member invitation controls check their own current permissions.</p>
    <button type="button" disabled={busy} onClick={() => { void load(); }}>Load first server page</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {records && <><p role="status" ref={status} tabIndex={-1}>{records.items.length} shared server records shown.</p><ul>{records.items.map(item => <li key={item.id}>{item.name}: {item.state}. {item.kind === 'agent' ? 'Managed local PDS' : 'Deployment simulation'}.{manageInvitations && item.kind === 'agent' && item.state === 'ready' && <SharedMemberInvitations deploymentId={item.id} name={item.name} csrfToken={csrfToken}/>}</li>)}</ul>{records.items.length === 0 && <p>No shared server records.</p>}{records.hasMore && records.nextAfter && <button type="button" disabled={busy} onClick={() => { void load(records.nextAfter!); }}>Load more server records</button>}</>}
  </section>;
}

function SharedMemberInvitations({ deploymentId, name, csrfToken }: { deploymentId: string; name: string; csrfToken: string }) {
 const [open,setOpen] = useState(false);
 return <details onToggle={event=>setOpen(event.currentTarget.open)}><summary>Member invitations for {name}</summary>
 <p>Administrators need a separate local operator opt-in to manage invitations. It grants no access to a member's password or posting.</p>
 {open && <MemberInvitations deploymentId={deploymentId} csrfToken={csrfToken}/>}</details>;
}
