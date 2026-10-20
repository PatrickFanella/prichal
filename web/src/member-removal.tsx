import { useEffect, useRef, useState } from "react";
import { validRemoval, type RemovalStatus, type RemovalReceipt, type RemovalTarget } from "./member-removal-contract";

export function MemberRemoval({ target, handle, csrfToken, onClose }: { target: RemovalTarget; handle: string; csrfToken: string; onClose: () => void }) {
  const storageKey = `pds-member-removal:${target.deploymentId}:${target.invitationId}`;
  const path = `/api/member-invitations/${target.invitationId}/removal`;
  const [pending, setPending] = useState(() => { try { const v = sessionStorage.getItem(storageKey); return v && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v) ? v : ""; } catch { return ""; } });
  const [status, setStatus] = useState<RemovalStatus | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [review, setReview] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tick, setTick] = useState(0);
  const active = useRef<AbortController | null>(null);
  const sequence = useRef(0);
  const busyRef = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const failure = useRef<HTMLParagraphElement>(null);
  const launch = useRef<HTMLButtonElement>(null);
  useEffect(() => { heading.current?.focus(); return () => { sequence.current++; active.current?.abort(); }; }, []);
  useEffect(() => { if (error) failure.current?.focus(); }, [error]);
  useEffect(() => { if (review) heading.current?.focus(); }, [review]);
  async function read(signal: AbortSignal): Promise<RemovalStatus> {
    const response = await fetch(path, { cache: "no-store", signal });
    const value: unknown = await response.json();
    if (!response.ok || !validRemoval(value, target)) throw Error("Member removal status could not be verified. Controls are paused; reload status or sign in again.");
    if (pending && value.state !== "not_requested" && (value as RemovalReceipt).idempotencyKey !== pending) throw Error("The retained request key differs from the server receipt. Contact support before another removal request.");
    return value;
  }
  useEffect(() => {
    const controller = new AbortController(); let loading = false;
    async function load() {
      if (loading || busyRef.current) return;
      loading = true; const current = ++sequence.current;
      try {
        const value = await read(AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]));
        if (controller.signal.aborted || current !== sequence.current) return;
        setStatus(value); setError("");
        if (pending && value.state !== "not_requested") {
          try { sessionStorage.removeItem(storageKey); setPending(""); } catch { /* The server receipt retains its original retry key. */ }
        }
        if (value.state !== "not_requested" && value.state !== "review") { setReview(false); setUnderstood(false); }
      } catch (cause) {
        if (!controller.signal.aborted && current === sequence.current) { setStatus(null); setReview(false); setUnderstood(false); setError(cause instanceof Error ? cause.message : "Member removal status unavailable."); }
      } finally { loading = false; }
    }
    void load(); const timer = window.setInterval(() => void load(), 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [path, target.deploymentId, target.invitationId, target.did, pending, tick]);
  async function submit() {
    if (busyRef.current || !review || !understood || !status || !["not_requested", "review"].includes(status.state)) return;
    const key = pending || (status.state === "review" ? (status as RemovalReceipt).idempotencyKey : crypto.randomUUID());
    try { sessionStorage.setItem(storageKey, key); setPending(key); } catch { setError("Enable session storage so an uncertain response can be retried with the same request key."); return; }
    busyRef.current = true; setBusy(true); sequence.current++; setReview(false); setUnderstood(false); setError(""); setNotice("");
    const controller = new AbortController(); active.current = controller;
    const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]);
    try {
      const fresh = await read(signal);
      if (fresh.state !== "not_requested" && (fresh as RemovalReceipt).idempotencyKey !== key) throw Error("The server has a different removal request. Reload status before continuing.");
      if (fresh.state !== "not_requested" && fresh.state !== "review") { setStatus(fresh); setNotice("The existing removal request is already recorded. Its original window is retained."); return; }
      const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken }, body: JSON.stringify({ idempotencyKey: key }), signal });
      const value: unknown = await response.json();
      if (!response.ok || !validRemoval(value, target) || value.state === "not_requested" || (value as RemovalReceipt).idempotencyKey !== key) throw Error("The removal response could not be confirmed. Reload status and retry the retained request after review.");
      if (controller.signal.aborted) return;
      setStatus(value); sessionStorage.removeItem(storageKey); setPending(""); setNotice("Member removal request recorded. The server's confirmed deadline remains authoritative.");
    } catch (cause) {
      if (!controller.signal.aborted) { setStatus(null); setError(cause instanceof Error ? cause.message : "Member removal could not be confirmed. Retry the retained request."); }
    } finally {
      if (active.current === controller) { active.current = null; busyRef.current = false; if (!controller.signal.aborted) { setBusy(false); setTick(v => v + 1); } }
    }
  }
  const receipt = status && status.state !== "not_requested" ? status as RemovalReceipt : null;
  return <section className="enrollment-withdrawal-review" aria-label={`Removal for ${handle}`}>
    <h5 ref={heading} tabIndex={-1}>{review ? "Review member removal" : "Member removal status"}</h5>
    <p>{handle} · <code>{target.did}</code></p>
    {!status && !error && <p role="status">Checking removal status…</p>}
    {status?.state === "not_requested" && <p>No removal has been requested for this accepted member.</p>}
    {receipt && <><p>Status: {receipt.state.replaceAll("_", " ")}. {receipt.message}</p>
      {receipt.deactivateAfter && <p>Confirmed migration deadline: {new Date(receipt.deactivateAfter).toLocaleString()}.</p>}
      <details><summary>Removal history (newest 100 events)</summary><ul>{receipt.events.map((event, index) => <li key={index}>{new Date(event.createdAt).toLocaleString()} · {event.state.replaceAll("_", " ")} · {event.actorId ? `Manager ${event.actorId}` : "Worker observation"}: {event.message}</li>)}</ul></details>
    </>}
    {pending && <p>A request key is retained in this tab. Confirm its result before another removal request.</p>}
    {status && ["not_requested", "review"].includes(status.state) && !review && <button type="button" ref={launch} disabled={busy} onClick={() => { setReview(true); setUnderstood(false); }}>Review {pending || receipt ? "removal retry" : "member removal"}</button>}
    {review && <div><p>The server will confirm a fixed 30-day migration window. This removal will not deactivate the account before that deadline. Existing account restrictions remain in effect. After the window, it requests deactivation, not deletion. The account and its data remain retained and count toward the member limit. This flow does not change the member's handle or offer cancellation.</p>
      <label><input type="checkbox" checked={understood} disabled={busy} onChange={event => setUnderstood(event.target.checked)}/> I checked this member and understand the migration window and retained data.</label>
      <button type="button" disabled={busy || !understood} onClick={() => void submit()}>Confirm {pending || receipt ? "removal retry" : "member removal"}</button>
      <button type="button" disabled={busy} onClick={() => { setReview(false); setUnderstood(false); window.setTimeout(() => launch.current?.focus(), 0); }}>Close removal review</button>
    </div>}
    <button type="button" disabled={busy} onClick={() => { setReview(false); setUnderstood(false); setTick(v => v + 1); }}>Reload removal status</button>
    <button type="button" disabled={busy} onClick={onClose}>Close removal status</button>
    {notice && <p role="status">{notice}</p>}
    {error && <p ref={failure} tabIndex={-1} role="alert">{error}</p>}
  </section>;
}
