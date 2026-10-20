import { useEffect, useRef, useState } from "react";
import { enrollmentObject as object, enrollmentUUID as uuid } from "./enrollment-contract";
import { validMemberInvitation, validMemberSummary, type MemberSummary } from "./member-invitation-contract";

export function MemberInvitations(props: { deploymentId: string; csrfToken: string }) {
  return <MemberWorkspace key={`${props.deploymentId}:${props.csrfToken}`} {...props} />;
}
function MemberWorkspace({ deploymentId, csrfToken }: { deploymentId: string; csrfToken: string }) {
  const requestKey = `pds-member-request:${deploymentId}`;
  const withdrawalKey = `pds-member-withdrawal:${deploymentId}`;
  function retained(key: string) { try { const id = sessionStorage.getItem(key); return id && uuid.test(id) ? id : ""; } catch { return ""; } }
  const [request, setRequest] = useState(() => retained(requestKey));
  const [withdrawal, setWithdrawal] = useState(() => retained(withdrawalKey));
  const [summary, setSummary] = useState<MemberSummary | null>(null);
  const [admission, setAdmission] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [review, setReview] = useState("");
  const [understood, setUnderstood] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tick, setTick] = useState(0);
  const [now, setNow] = useState(Date.now());
  const busyRef = useRef(false);
  const sequence = useRef(0);
  const mutation = useRef<AbortController | null>(null);
  const failure = useRef<HTMLParagraphElement>(null);
  const reviewTitle = useRef<HTMLHeadingElement>(null);
  const launchers = useRef(new Map<string, HTMLButtonElement>());
  const path = `/api/deployments/${deploymentId}/members/invitations`;
  useEffect(() => () => { sequence.current++; mutation.current?.abort(); }, []);
  useEffect(() => { if (error || loadError) failure.current?.focus(); }, [error, loadError]);
  useEffect(() => { if (review) reviewTitle.current?.focus(); }, [review]);
  useEffect(() => {
    const controller = new AbortController(); let loading = false;
    async function load() {
      if (loading || busyRef.current) return;
      loading = true; const read = ++sequence.current;
      try {
        const response = await fetch(path, { cache: "no-store", signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
        const value: unknown = await response.json();
        if (controller.signal.aborted || read !== sequence.current) return;
        if (!response.ok || !validMemberSummary(value, deploymentId)) throw Error([401, 403].includes(response.status) ? "Sign in again to view member invitations." : "Member status is unavailable. Invitation controls are paused until status returns.");
        // A retained withdrawal can fall outside the newest 100 invitations.
        if (withdrawal && !value.invitations.some(item => item.id === withdrawal)) {
          const detail = await fetch(`/api/member-invitations/${withdrawal}`, { cache: "no-store", signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
          const item: unknown = await detail.json();
          if (controller.signal.aborted || read !== sequence.current) return;
          if (!detail.ok || !validMemberInvitation(item, deploymentId) || item.id !== withdrawal) throw Error("The retained withdrawal could not be found. Sign in with its original owner or contact support.");
          value.invitations.push(item);
        }
        if (withdrawal && value.invitations.some(item => item.id === withdrawal && item.state !== "invitation_ready")) {
          try { sessionStorage.removeItem(withdrawalKey); } catch { /* Durable status remains on the server. */ }
          setWithdrawal("");
        }
        setSummary(value); setLoadError("");
        if (review && !value.invitations.some(item => item.id === review && item.state === "invitation_ready" && !item.withdrawalRequested && item.expiresAt && Date.parse(item.expiresAt) > Date.now())) { setReview(""); setUnderstood(false); }
        try {
          const policyResponse = await fetch(`/api/deployments/${deploymentId}/members/admission`, { cache: "no-store", signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
          if (controller.signal.aborted || read !== sequence.current) return;
          if ([401, 403].includes(policyResponse.status)) { setSummary(null); setAdmission(false); setLoadError("Sign in again to view member invitations."); setReview(""); setUnderstood(false); return; }
          const policy: unknown = await policyResponse.json();
          if (controller.signal.aborted || read !== sequence.current) return;
          setAdmission(policyResponse.ok && object(policy) && policy.open === true);
        } catch { if (!controller.signal.aborted && read === sequence.current) { setAdmission(false); } }
      } catch (cause) {
        if (!controller.signal.aborted && read === sequence.current) { setSummary(null); setAdmission(false); setReview(""); setUnderstood(false); setLoadError(cause instanceof Error ? cause.message : "Member status is unavailable."); }
      } finally { loading = false; }
    }
    void load(); const timer = window.setInterval(() => { setNow(Date.now()); void load(); }, 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [deploymentId, path, withdrawal, tick, review, withdrawalKey]);
  async function act(action: "issue" | "refresh" | "withdraw", id = "") {
    if (busyRef.current || !summary || (withdrawal && (action !== "withdraw" || id !== withdrawal))) return;
    const item = summary.invitations.find(value => value.id === id);
    if (action !== "issue" && (!item || item.state !== "invitation_ready" || item.withdrawalRequested || !item.expiresAt || Date.parse(item.expiresAt) <= Date.now())) return;
    if (action === "withdraw" && (review !== id || !understood)) return;
    if (action === "issue" && !request && !admission) return;
    const key = request || crypto.randomUUID();
    // Retain only request identifiers, never invitation codes or member credentials.
    try {
      if (action === "issue") { sessionStorage.setItem(requestKey, key); setRequest(key); }
      if (action === "withdraw") { sessionStorage.setItem(withdrawalKey, id); setWithdrawal(id); }
    } catch { setError("Enable session storage before requesting or withdrawing an invitation so retries can retain the same request."); return; }
    busyRef.current = true; sequence.current++; setBusy(true); setError(""); setNotice(""); setAdmission(false); setReview(""); setUnderstood(false);
    if (action === "withdraw") setSummary(null);
    const controller = new AbortController(); mutation.current = controller;
    try {
      const response = await fetch(action === "issue" ? path : `/api/member-invitations/${id}/${action}`, {
        method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
        body: JSON.stringify(action === "issue" ? { idempotencyKey: key } : {}), signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]),
      });
      const value: unknown = await response.json();
      if (controller.signal.aborted) return;
      if (!response.ok || !validMemberInvitation(value, deploymentId) || (action !== "issue" && value.id !== id)) {
        throw Error(object(value) && typeof value.error === "string" && value.error.length <= 1024 ? value.error : "The invitation action could not be confirmed. Refresh status before retrying the same request.");
      }
      if (action === "issue") { sessionStorage.removeItem(requestKey); setRequest(""); }
      setNotice(action === "issue" ? "Member invitation request recorded." : action === "withdraw" ? "Withdrawal requested. An account created first will be preserved." : "Native invitation status check requested.");
    } catch (cause) {
      if (!controller.signal.aborted) { setSummary(null); setError(cause instanceof Error ? cause.message : "The invitation action could not be confirmed. Retry the same request."); }
    } finally {
      if (mutation.current === controller) { mutation.current = null; busyRef.current = false; if (!controller.signal.aborted) { setBusy(false); setTick(value => value + 1); } }
    }
  }
  return <section className="member-invitations" aria-label="Member accounts">
    <h4>Member accounts</h4>
    <p>Invite people to create their own accounts on this server. Members enter their email and password directly on the PDS. Their network records are public.</p>
    <p>These invitations create AT accounts. Management access is invited separately.</p>
    {summary && <p>{summary.members} accepted members · {summary.open} open invitations · {summary.ceiling} member limit. Pending requests also reserve places. Showing the newest 100 invitations.</p>}
    {!summary && !loadError && !error && <p role="status">Checking member invitation status…</p>}
    {!admission && <p>New member invitations are paused or availability could not be confirmed. Existing invitation status remains available.</p>}
    <button type="button" disabled={busy || !summary || !!withdrawal || (!request && !admission)} onClick={() => void act("issue")}>{request ? "Retry same member invitation request" : "Prepare member invitation"}</button>
    <button type="button" disabled={busy} onClick={() => setTick(value => value + 1)}>Reload member status</button>
    {request && <p>A request key is retained in this tab. Retry it to confirm its result before preparing another invitation.</p>}
    {withdrawal && <p role="status">Withdrawal confirmation is pending. Its code is hidden. Review a retry only if fresh status still shows that invitation as open.</p>}
    {summary && summary.invitations.length === 0 && <p>No member invitations yet.</p>}
    <ul>{summary?.invitations.map(item => {
      const current = item.state === "invitation_ready" && !!item.expiresAt && Date.parse(item.expiresAt) > now;
      const actionable = current && !item.withdrawalRequested && (!withdrawal || withdrawal === item.id);
      return <li key={item.id}>
        <p>Invitation <code>{item.id}</code> — {item.state === "invitation_ready" && !current ? "expired" : item.state.replaceAll("_", " ")}</p>
        <p>{item.message}</p>
        {item.did && <p>Member: {item.handle} · <code>{item.did}</code></p>}
        {current && !withdrawal && !item.withdrawalRequested && <>
          <p>Single-use invitation code: <code>{item.code}</code></p>
          <p>Expires {new Date(item.expiresAt!).toLocaleString()}. Share the code privately with the member.</p>
          <a href={item.accountUrl} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">Open this PDS account page</a>
        </>}
        {actionable && <div>
          <button type="button" disabled={busy || !!withdrawal} onClick={() => void act("refresh", item.id)}>Check whether invitation was used</button>
          <button type="button" disabled={busy} ref={node => { if (node) launchers.current.set(item.id, node); else launchers.current.delete(item.id); }} onClick={() => { setReview(item.id); setUnderstood(false); }}>{withdrawal === item.id ? "Review withdrawal retry" : "Review invitation withdrawal"}</button>
        </div>}
        {review === item.id && actionable && <div className="enrollment-withdrawal-review">
          <h5 ref={reviewTitle} tabIndex={-1}>Withdraw this unused invitation?</h5>
          <p>The code will stop allowing a new account once the server confirms withdrawal. If the member used it first, their account is preserved. This does not remove a member account.</p>
          <label><input type="checkbox" checked={understood} onChange={event => setUnderstood(event.target.checked)} /> I understand an existing account will be preserved.</label>
          <div><button type="button" disabled={busy || !understood} onClick={() => void act("withdraw", item.id)}>Request invitation withdrawal</button>
          <button type="button" disabled={busy} onClick={() => { setReview(""); setUnderstood(false); window.setTimeout(() => launchers.current.get(item.id)?.focus(), 0); }}>Close withdrawal review</button></div>
        </div>}
      </li>;
    })}</ul>
    {notice && <p role="status">{notice}</p>}
    {(loadError || error) && <p role="alert" ref={failure} tabIndex={-1}>{loadError || error}</p>}
  </section>;
}
