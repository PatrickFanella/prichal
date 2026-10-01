import { useEffect, useRef, useState } from 'react';

type Role = 'administrator' | 'viewer';
export type Member = { identityId: string; role: Role; state: 'active' | 'revoked' };
type Invitation = { id: string; targetId: string; role: Role; state: 'pending' | 'accepted' | 'expired' | 'revoked'; expiresAt: string };
type Page<T> = { items: T[]; hasMore: boolean; nextAfter: string | null };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(v: unknown): v is Record<string, unknown> { return typeof v === 'object' && v !== null; }
function identity(v: unknown): v is string { return typeof v === 'string' && uuid.test(v); }
function role(v: unknown): v is Role { return v === 'viewer' || v === 'administrator'; }
function member(v: unknown): v is Member { return object(v) && identity(v.identityId) && role(v.role) && (v.state === 'active' || v.state === 'revoked'); }
function invitation(v: unknown): v is Invitation { return object(v) && identity(v.id) && identity(v.targetId) && role(v.role) && ['pending', 'accepted', 'expired', 'revoked'].includes(String(v.state)) && typeof v.expiresAt === 'string' && Number.isFinite(Date.parse(v.expiresAt)); }
function page<T>(v: unknown, valid: (item: unknown) => item is T): v is Page<T> {
  return object(v) && Array.isArray(v.items) && v.items.length <= 50 && v.items.every(valid) && typeof v.hasMore === 'boolean' && (v.hasMore ? typeof v.nextAfter === 'string' && v.nextAfter.length > 0 && v.nextAfter.length <= 384 : v.nextAfter === null);
}
function useReceipts<T>(organizationId: string, kind: 'members' | 'invitations', valid: (item: unknown) => item is T, beforeRead: () => void) {
  const [records, setRecords] = useState<Page<T> | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null), status = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (records) status.current?.focus(); }, [records]);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { setRecords(null); setError(''); setBusy(false); return () => { active.current?.abort(); active.current = null; }; }, [organizationId, kind]);
  async function load(after?: string) {
    beforeRead(); active.current?.abort(); const controller = new AbortController(); active.current = controller; setBusy(true); setError('');
    try {
      const response = await fetch(`/api/team/organizations/${organizationId}/${kind}/page${after ? `?after=${encodeURIComponent(after)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (active.current !== controller) return;
      if (!response.ok || !page(value, valid)) throw new Error('Receipt page unavailable');
      setRecords(value);
    } catch { if (active.current === controller) { setRecords(null); setError('This owner-only page could not be confirmed. Refresh current organization access before loading again.'); } }
    finally { if (active.current === controller) setBusy(false); }
  }
  return { records, error, busy, status, failure, load };
}
export function CollaborationMembers({ organizationId, disabled, working, emptyOrganization, beforeRead, onChange, onOffer }: { organizationId: string; disabled: boolean; working: boolean; emptyOrganization: boolean; beforeRead: () => void; onChange: (member: Member, role: Role | 'revoked') => void; onOffer: (member: Member) => void }) {
  const { records, error, busy, status, failure, load } = useReceipts(organizationId, 'members', member, beforeRead);
  return <section aria-label="Organization membership records" aria-busy={busy}><h5>Membership records</h5><p>Each page checks current ownership and shows current roles, including retained revocations. Loading a page does not confirm a role change.</p>
    <button type="button" disabled={busy || working} onClick={() => { void load(); }}>Load first membership page</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {records && <><p role="status" ref={status} tabIndex={-1}>{records.items.length} membership records shown.</p><ul>{records.items.map(item => <li key={item.identityId}><p><code>{item.identityId}</code>: {item.role}, {item.state}.</p>{item.state === 'active' && <><button type="button" disabled={busy || disabled} onClick={() => onChange(item, item.role === 'viewer' ? 'administrator' : 'viewer')}>Review role change</button><button type="button" disabled={busy || disabled} onClick={() => onChange(item, 'revoked')}>Review revocation</button>{item.role === 'administrator' && emptyOrganization && <button type="button" disabled={busy || disabled} onClick={() => onOffer(item)}>Review empty-organization ownership offer</button>}</>}</li>)}</ul>{records.items.length === 0 && <p>No membership records.</p>}{records.hasMore && records.nextAfter && <button type="button" disabled={busy || working} onClick={() => { void load(records.nextAfter!); }}>Load more membership records</button>}</>}
  </section>;
}
export function CollaborationInvitationReceipts({ organizationId, disabled, beforeRead }: { organizationId: string; disabled: boolean; beforeRead: () => void }) {
  const { records, error, busy, status, failure, load } = useReceipts(organizationId, 'invitations', invitation, beforeRead);
  return <details aria-label="Organization invitation receipts" aria-busy={busy}><summary>Invitation receipts</summary><p>Receipts are recorded history. Pending expiry is calculated on each read. Current membership determines access.</p>
    <button type="button" disabled={busy || disabled} onClick={() => { void load(); }}>Load first invitation receipt page</button>
    {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
    {records && <><p role="status" ref={status} tabIndex={-1}>{records.items.length} invitation receipts shown.</p><ul>{records.items.map(item => <li key={item.id}><code>{item.targetId}</code>: {item.role}, {item.state}. Expires <time dateTime={item.expiresAt}>{new Date(item.expiresAt).toLocaleString()}</time>.</li>)}</ul>{records.items.length === 0 && <p>No invitation receipts.</p>}{records.hasMore && records.nextAfter && <button type="button" disabled={busy || disabled} onClick={() => { void load(records.nextAfter!); }}>Load more invitation receipts</button>}</>}
  </details>;
}
