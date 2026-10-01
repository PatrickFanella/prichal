import { useEffect, useRef, useState } from 'react';
import { CollaborationHistory } from './collaboration-history';
import { SharedInstanceStatus } from './shared-instance-status';

type Role = 'administrator' | 'viewer';
type Organization = { organizationId: string; name: string; role: Role | 'owner' };
type OrganizationPage = { items: Organization[]; hasMore: boolean; nextAfter: string | null };
type Incoming = { invitations: { id: string; organizationId: string; role: Role; expiresAt: string }[]; transfers: { id: string; organizationId: string; expiresAt: string }[] };
type Shared = Organization & { instances: { id: string; name: string; state: string; kind: string }[]; hasMore: boolean };
type Members = { members: { identityId: string; role: Role; state: 'active' | 'revoked' }[]; membersTruncated: boolean; invitations: { id: string; targetId: string; role: Role; state: string; expiresAt: string }[]; audit: { id: string; actorId: string; targetId: string | null; role: Role | null; action: string; createdAt: string }[] };
type Intent =
  | { kind: 'invite'; organizationId: string; identityId: string; role: Role; key: string }
  | { kind: 'offer'; organizationId: string; identityId: string; key: string }
  | { kind: 'accept-invite'; id: string; organizationId: string; role: Role }
  | { kind: 'accept-offer'; id: string; organizationId: string }
  | { kind: 'member'; organizationId: string; identityId: string; role: Role | 'revoked' }
  | { kind: 'rename'; organizationId: string; name: string };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null; }
function identity(value: unknown): value is string { return typeof value === 'string' && uuid.test(value); }
function role(value: unknown): value is Role { return value === 'administrator' || value === 'viewer'; }
function date(value: unknown): value is string { return typeof value === 'string' && Number.isFinite(Date.parse(value)); }
function organization(value: unknown): value is Organization { return object(value) && identity(value.organizationId) && typeof value.name === 'string' && (value.role === 'owner' || role(value.role)); }
function organizationPage(value: unknown): value is OrganizationPage {
  return object(value) && Array.isArray(value.items) && value.items.length <= 50 && value.items.every(organization) && typeof value.hasMore === 'boolean' && (value.hasMore ? typeof value.nextAfter === 'string' && value.nextAfter.length > 0 && value.nextAfter.length <= 256 : value.nextAfter === null);
}
function incoming(value: unknown): value is Incoming {
  return object(value) && Array.isArray(value.invitations) && value.invitations.length <= 30 && value.invitations.every(item => object(item) && identity(item.id) && identity(item.organizationId) && role(item.role) && date(item.expiresAt)) && Array.isArray(value.transfers) && value.transfers.length <= 30 && value.transfers.every(item => object(item) && identity(item.id) && identity(item.organizationId) && date(item.expiresAt));
}
function shared(value: unknown): value is Shared { return object(value) && Array.isArray(value.instances) && value.instances.length <= 100 && value.instances.every(item => object(item) && identity(item.id) && typeof item.name === 'string' && typeof item.state === 'string' && typeof item.kind === 'string') && typeof value.hasMore === 'boolean' && organization(value); }
function members(value: unknown): value is Members {
  return object(value) && Array.isArray(value.members) && value.members.length <= 100 && value.members.every(item => object(item) && identity(item.identityId) && role(item.role) && (item.state === 'active' || item.state === 'revoked')) && typeof value.membersTruncated === 'boolean' && Array.isArray(value.invitations) && value.invitations.length <= 100 && value.invitations.every(item => object(item) && identity(item.id) && identity(item.targetId) && role(item.role) && ['pending', 'accepted', 'expired', 'revoked'].includes(String(item.state)) && date(item.expiresAt)) && Array.isArray(value.audit) && value.audit.length <= 100 && value.audit.every(item => object(item) && typeof item.id === 'string' && /^[1-9][0-9]*$/.test(item.id) && identity(item.actorId) && (item.targetId === null || identity(item.targetId)) && (item.role === null || role(item.role)) && typeof item.action === 'string' && date(item.createdAt));
}
function intent(value: unknown): Intent | null {
  if (!object(value)) return null;
  if (value.kind === 'accept-invite' && identity(value.id) && identity(value.organizationId) && role(value.role)) return { kind: value.kind, id: value.id, organizationId: value.organizationId, role: value.role };
  if (value.kind === 'accept-offer' && identity(value.id) && identity(value.organizationId)) return { kind: value.kind, id: value.id, organizationId: value.organizationId };
  if (!identity(value.organizationId)) return null;
  if (value.kind === 'rename' && typeof value.name === 'string' && value.name.trim().length > 0 && [...value.name].length <= 100) return { kind: value.kind, organizationId: value.organizationId, name: value.name };
  if (!identity(value.identityId)) return null;
  if (value.kind === 'member' && (role(value.role) || value.role === 'revoked')) return { kind: value.kind, organizationId: value.organizationId, identityId: value.identityId, role: value.role };
  if (!identity(value.key)) return null;
  if (value.kind === 'invite' && role(value.role)) return { kind: value.kind, organizationId: value.organizationId, identityId: value.identityId, role: value.role, key: value.key };
  if (value.kind === 'offer') return { kind: value.kind, organizationId: value.organizationId, identityId: value.identityId, key: value.key };
  return null;
}
function description(value: Intent): string {
  switch (value.kind) {
    case 'invite': return `Invite management identity ${value.identityId} as ${value.role} to organization ${value.organizationId}.`;
    case 'offer': return `Offer ownership of empty organization ${value.organizationId} to management identity ${value.identityId}. Ownership changes only after their separate acceptance.`;
    case 'accept-offer': return `Accept management ownership of empty organization ${value.organizationId} in offer ${value.id}. The former owner will lose access.`;
    case 'accept-invite': return `Accept the ${value.role} management role in organization ${value.organizationId}, invitation ${value.id}.`;
    case 'member': return value.role === 'revoked' ? `Revoke management identity ${value.identityId} in organization ${value.organizationId}.` : `Change management identity ${value.identityId} to ${value.role} in organization ${value.organizationId}.`;
    case 'rename': return `Rename organization ${value.organizationId} to “${value.name}”.`;
  }
}
function request(value: Intent): { path: string; method: 'POST' | 'PATCH'; body: object } {
  switch (value.kind) {
    case 'invite': return { path: `/api/team/organizations/${value.organizationId}/invitations`, method: 'POST', body: { identityId: value.identityId, role: value.role, idempotencyKey: value.key } };
    case 'offer': return { path: `/api/team/organizations/${value.organizationId}/transfers`, method: 'POST', body: { identityId: value.identityId, idempotencyKey: value.key, confirmEmptyOrganizationHandover: true } };
    case 'accept-offer': return { path: `/api/team/transfers/${value.id}/accept`, method: 'POST', body: { confirmEmptyOrganizationHandover: true } };
    case 'accept-invite': return { path: `/api/team/invitations/${value.id}/accept`, method: 'POST', body: {} };
    case 'member': return { path: `/api/team/organizations/${value.organizationId}/members/${value.identityId}`, method: 'PATCH', body: { role: value.role } };
    case 'rename': return { path: `/api/team/organizations/${value.organizationId}`, method: 'PATCH', body: { name: value.name } };
  }
}

export function CollaborationWorkspace({ ownerId, csrfToken, onChanged }: { ownerId: string; csrfToken: string; onChanged: () => void }) {
  const storageKey = `pds-collaboration-intent:${ownerId}`;
  const [pending, setPending] = useState<Intent | null>(null), [reviewed, setReviewed] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [blocked, setBlocked] = useState(false);
  const [available, setAvailable] = useState(false), [organizations, setOrganizations] = useState<OrganizationPage>({ items: [], hasMore: false, nextAfter: null }), [offers, setOffers] = useState<Incoming>({ invitations: [], transfers: [] });
  const [view, setView] = useState<Shared | null>(null), [management, setManagement] = useState<Members | null>(null), [target, setTarget] = useState(''), [inviteRole, setInviteRole] = useState<Role>('viewer'), [newName, setNewName] = useState('');
  const [busy, setBusy] = useState(false), [listBusy, setListBusy] = useState(false), [listError, setListError] = useState('');
  const listRead = useRef<AbortController | null>(null), listStatus = useRef<HTMLParagraphElement | null>(null), listFailure = useRef<HTMLParagraphElement | null>(null), focusList = useRef(false);
  useEffect(() => { if (focusList.current) { listStatus.current?.focus(); focusList.current = false; } }, [organizations]);
  useEffect(() => { if (listError) listFailure.current?.focus(); }, [listError]);
  const read = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null);
  const reviewHeading = useRef<HTMLHeadingElement | null>(null), receiptNotice = useRef<HTMLParagraphElement | null>(null);
  useEffect(() => { if (available && (pending || blocked)) reviewHeading.current?.focus(); }, [available, pending, blocked]);
  useEffect(() => { if (available && notice && !pending && !blocked) receiptNotice.current?.focus(); }, [available, notice, pending, blocked]);
  async function refresh() {
    read.current?.abort(); listRead.current?.abort(); setListBusy(false); setListError(''); const controller = new AbortController(); read.current = controller;
    try {
      const get = (path: string) => fetch(path, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const responses = await Promise.all([get('/api/team/organizations/page'), get('/api/team/incoming')]);
      if (controller.signal.aborted) return;
      if (responses.some(response => response.status === 404)) { setAvailable(false); setView(null); setManagement(null); return; }
      const [list, incomingOffers]: unknown[] = await Promise.all(responses.map(response => response.json()));
      if (controller.signal.aborted) return;
      if (!responses.every(response => response.ok) || !organizationPage(list) || !incoming(incomingOffers)) throw new Error('Invalid collaboration response');
      setAvailable(true); setOrganizations(list); setOffers(incomingOffers); setView(null); setManagement(null);
    } catch { if (!controller.signal.aborted) setError('Organization collaboration is temporarily unavailable.'); }
  }
  useEffect(() => {
    try { const raw = sessionStorage.getItem(storageKey); if (raw) { const restored = intent(JSON.parse(raw)); if (!restored) throw new Error('Invalid retained action'); setPending(restored); } }
    catch { setBlocked(true); setNotice('A retained action could not be verified. Check current access before starting another action.'); }
    void refresh();
    return () => { read.current?.abort(); listRead.current?.abort(); mutation.current?.abort(); };
  }, [storageKey]);
  async function loadOrganizations(after?: string) {
    read.current?.abort(); listRead.current?.abort();
    const controller = new AbortController(); listRead.current = controller;
    setListBusy(true); setListError(''); setView(null); setManagement(null); setReviewed(false);
    try {
      const response = await fetch(`/api/team/organizations/page${after ? `?after=${encodeURIComponent(after)}` : ''}`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (listRead.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !organizationPage(value)) { setOrganizations({ items: [], hasMore: false, nextAfter: null }); throw new Error('Invalid organization page'); }
      focusList.current = true; setOrganizations(value);
    } catch { if (listRead.current === controller && !controller.signal.aborted) setListError('The organization list could not be confirmed. Refresh current access and load the first page again.'); }
    finally { if (listRead.current === controller && !controller.signal.aborted) setListBusy(false); }
  }
  async function inspect(item: Organization) {
    read.current?.abort(); listRead.current?.abort(); setListBusy(false); const controller = new AbortController(); read.current = controller; setView(null); setManagement(null); setError(''); setReviewed(false);
    try {
      const paths = [`/api/team/organizations/${item.organizationId}`, ...(item.role === 'owner' ? [`/api/team/organizations/${item.organizationId}/members`] : [])];
      const responses = await Promise.all(paths.map(path => fetch(path, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) })));
      const values: unknown[] = await Promise.all(responses.map(response => response.json()));
      if (controller.signal.aborted) return;
      if (!responses.every(response => response.ok) || !shared(values[0]) || (item.role === 'owner' && !members(values[1]))) throw new Error('Invalid collaboration response');
      setView(values[0]); setManagement(item.role === 'owner' ? values[1] as Members : null); setNewName(values[0].name); setTarget('');
    } catch { if (!controller.signal.aborted) setError('This organization view could not be confirmed. Refresh current access.'); }
  }
  function stage(value: Intent) {
    if (pending || blocked || busy) return;
    try { sessionStorage.setItem(storageKey, JSON.stringify(value)); setPending(value); setReviewed(false); setError(''); setNotice(''); }
    catch { setError('Enable session storage before reviewing a collaboration action.'); }
  }
  function clear() {
    try { sessionStorage.removeItem(storageKey); setPending(null); setBlocked(false); setReviewed(false); setError(''); }
    catch { setError('The retained request could not be cleared.'); }
  }
  async function submit() {
    if (!pending || !reviewed || busy) return;
    const retained = pending; const controller = new AbortController(); mutation.current = controller;
    setBusy(true); setError('');
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(retained));
      const action = request(retained), response = await fetch(action.path, { method: action.method, headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }, body: JSON.stringify(action.body), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok) { setError(object(value) && typeof value.error === 'string' ? value.error : 'The action could not be confirmed. Review current access before retrying.'); return; }
      const receipt = object(value) && (retained.kind === 'member' || retained.kind === 'rename' ? value.recorded === true : retained.kind === 'invite' ? identity(value.id) && value.targetId === retained.identityId && value.organizationId === retained.organizationId && value.role === retained.role : retained.kind === 'accept-invite' ? value.id === retained.id && value.targetId === ownerId && value.organizationId === retained.organizationId && value.role === retained.role : identity(value.id) && (retained.kind === 'accept-offer' ? value.id === retained.id : true));
      if (!receipt) throw new Error('Invalid action receipt');
      sessionStorage.removeItem(storageKey); setPending(null); setReviewed(false); setNotice('The action was recorded. Current access and membership determine your permissions.');
      await refresh(); if (!controller.signal.aborted) onChanged();
    } catch { if (!controller.signal.aborted) setError('The action could not be confirmed. Review current access; retry retains the same request.'); }
    finally { if (!controller.signal.aborted) { setBusy(false); setReviewed(false); } }
  }
  if (!available) return null;
  return <section className="collaboration-workspace" aria-labelledby="collaboration-title"><h3 id="collaboration-title">Local organization collaboration</h3>
    <p>Hosting roles do not grant native account passwords, signing or publishing authority. Billing, exports and hosting exit stay under their existing owner controls.</p>
    <details><summary>Your management identity reference</summary><p>Ask colleagues to sign in and share this reference when inviting them. Email and display names do not establish identity.</p><code>{ownerId}</code></details>
    {error && <p role="alert">{error}</p>}{notice && <p role="status" ref={receiptNotice} tabIndex={-1}>{notice}</p>}
    {(pending || blocked) && <section className="collaboration-review" aria-labelledby="collaboration-review-title"><h4 id="collaboration-review-title" ref={reviewHeading} tabIndex={-1}>Review retained action</h4><p>{pending ? description(pending) : 'A retained request could not be verified. Check current membership and invitation history before clearing it.'}</p><label><input type="checkbox" checked={reviewed} disabled={busy} onChange={event => setReviewed(event.target.checked)}/>I checked this identity, role and action. {pending?.kind === 'accept-offer' && 'I accept management ownership of this empty organization.'}</label><button type="button" disabled={busy || !reviewed || pending === null} onClick={() => { void submit(); }}>Confirm or retry this action</button><button type="button" disabled={busy || !reviewed} onClick={clear}>Clear request after checking current access</button></section>}
    <button type="button" disabled={busy} onClick={() => { void refresh(); }}>Refresh collaboration access</button>
    <h4>Incoming invitations</h4><p>Only offers for your verified identity appear. Inviting and accepting a management role does not merge native accounts.</p>
    <ul>{offers.invitations.map(item => <li key={item.id}><p>Organization {item.organizationId}: {item.role}. Expires <time dateTime={item.expiresAt}>{new Date(item.expiresAt).toLocaleString()}</time>.</p><button type="button" disabled={busy || (pending !== null || blocked)} onClick={() => stage({ kind: 'accept-invite', id: item.id, organizationId: item.organizationId, role: item.role })}>Review invitation acceptance</button></li>)}</ul>
    {offers.invitations.length === 0 && <p>No pending invitations.</p>}
    <ul>{offers.transfers.map(item => <li key={item.id}><p>Ownership offer for empty organization {item.organizationId}. Expires <time dateTime={item.expiresAt}>{new Date(item.expiresAt).toLocaleString()}</time>.</p><button type="button" disabled={busy || (pending !== null || blocked)} onClick={() => stage({ kind: 'accept-offer', id: item.id, organizationId: item.organizationId })}>Review ownership acceptance</button></li>)}</ul>
    <h4>Permitted organizations</h4><p>Each page checks your current access. Incoming offers remain limited to 30 of each type; contact support if an offer is outside that list.</p>
    <p role="status" ref={listStatus} tabIndex={-1}>{organizations.items.length} permitted organizations shown.</p>
    {listError && <p role="alert" ref={listFailure} tabIndex={-1}>{listError}</p>}
    <button type="button" disabled={busy || listBusy} onClick={() => { void loadOrganizations(); }}>Load first organization page</button>
    <ul>{organizations.items.map(item => <li key={item.organizationId}>{item.name}: {item.role}. <button type="button" disabled={busy} onClick={() => { void inspect(item); }}>Inspect shared status</button></li>)}</ul>
    {organizations.hasMore && organizations.nextAfter && <button type="button" disabled={busy || listBusy} onClick={() => { void loadOrganizations(organizations.nextAfter!); }}>Load more organizations</button>}
    {view && <section aria-labelledby="collaboration-organization-title"><h4 id="collaboration-organization-title">{view.name}</h4><p>Current role: {view.role}.</p><SharedInstanceStatus key={view.organizationId} organizationId={view.organizationId}/>
      {view.role !== 'viewer' && <div><label htmlFor="collaboration-name">New organization name</label><input id="collaboration-name" value={newName} maxLength={100} disabled={busy || (pending !== null || blocked)} onChange={event => setNewName(event.target.value)}/><button type="button" disabled={busy || (pending !== null || blocked) || !newName.trim()} onClick={() => stage({ kind: 'rename', organizationId: view.organizationId, name: newName })}>Review rename</button></div>}
      {view.role === 'owner' && management && <><h5>Invite a colleague</h5><label htmlFor="collaboration-identity">Their management identity reference</label><input id="collaboration-identity" value={target} maxLength={36} disabled={busy || (pending !== null || blocked)} onChange={event => setTarget(event.target.value.trim().toLowerCase())}/><label htmlFor="collaboration-role">Hosting role</label><select id="collaboration-role" value={inviteRole} disabled={busy || (pending !== null || blocked)} onChange={event => setInviteRole(event.target.value as Role)}><option value="viewer">Viewer: shared status</option><option value="administrator">Administrator: shared status and organization rename</option></select><p>They must have signed in to this management service. Invitations expire after 24 hours.</p><button type="button" disabled={busy || (pending !== null || blocked) || !identity(target) || target === ownerId} onClick={() => stage({ kind: 'invite', organizationId: view.organizationId, identityId: target, role: inviteRole, key: crypto.randomUUID() })}>Review invitation</button>
        <h5>Members and access history</h5><ul>{management.members.map(member => <li key={member.identityId}><p><code>{member.identityId}</code>: {member.role}, {member.state}.</p>{member.state === 'active' && <><button type="button" disabled={busy || (pending !== null || blocked)} onClick={() => stage({ kind: 'member', organizationId: view.organizationId, identityId: member.identityId, role: member.role === 'viewer' ? 'administrator' : 'viewer' })}>Review role change</button><button type="button" disabled={busy || (pending !== null || blocked)} onClick={() => stage({ kind: 'member', organizationId: view.organizationId, identityId: member.identityId, role: 'revoked' })}>Review revocation</button>{member.role === 'administrator' && view.instances.length === 0 && !view.hasMore && <button type="button" disabled={busy || (pending !== null || blocked)} onClick={() => stage({ kind: 'offer', organizationId: view.organizationId, identityId: member.identityId, key: crypto.randomUUID() })}>Review empty-organization ownership offer</button>}</>}</li>)}</ul>{management.membersTruncated && <p>Retained membership history is truncated; current active members are included.</p>}<details><summary>Latest 100 invitation receipts</summary><p>Receipts are recorded history. Current membership determines access.</p><ul>{management.invitations.map(item => <li key={item.id}><code>{item.targetId}</code>: {item.role}, {item.state}. Expires <time dateTime={item.expiresAt}>{new Date(item.expiresAt).toLocaleString()}</time>.</li>)}</ul></details><CollaborationHistory key={view.organizationId} organizationId={view.organizationId}/><p>Ownership offers require a separate recipient acceptance. Populated hosting and billing handover need support review.</p></>}
    </section>}
  </section>;
}
