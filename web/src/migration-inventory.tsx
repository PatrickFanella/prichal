import { useEffect, useRef, useState } from 'react';

type Inventory = {
 state: 'archive_inventory_only'; direction: 'outbound_from_retained_export'; exportId: string; deploymentId: string; did: string;
 capturedAt: string; archiveVersion: 1 | 2; archiveBytes: number; archiveSha256: string; repositoryBytes: number;
 capturedRecordCount: number; blobCount: number; blobBytes: number; preferences: 'not_captured_v1' | 'captured_app_bsky_only';
 integrity: 'authenticated_archive_and_member_hashes'; signatureVerification: 'not_rechecked'; canTransfer: false; canSwitchIdentity: false; remainingGates: string[];
};
const gates = ['current_source_account_authority_and_recovery_options', 'independent_repository_signature_and_blob_reference_verification', 'authorized_target_compatibility_and_capacity', 'resumable_transfer_and_independent_import_inventory', 'explicit_identity_switch_authorization', 'post_switch_auth_signing_and_downstream_resolution', 'external_application_state_and_preferences'];
const labels = ['Current source account control and recovery options.', 'Independent repository signature and referenced media checks.', 'An authorized target with compatible software and sufficient capacity.', 'Resumable transfer and independently verified imported inventory.', 'Explicit approval to switch the account identity after transfer.', 'Authentication, signing and downstream identity checks after the switch.', 'Preferences and application state held outside this export.'];
const integer = (v: unknown, min: number, max: number): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max;
function valid(value: unknown, exportId: string, deploymentId: string, sha256: string, bytes: number, capturedAt: string): value is Inventory {
 if (typeof value !== 'object' || value === null) return false;
 const v = value as Record<string, unknown>;
 return v.state === 'archive_inventory_only' && v.direction === 'outbound_from_retained_export' && v.exportId === exportId && v.deploymentId === deploymentId && typeof v.did === 'string' && /^did:plc:[a-z2-7]{24}$/.test(v.did) && typeof v.capturedAt === 'string' && Number.isFinite(Date.parse(v.capturedAt)) && Date.parse(v.capturedAt) === Date.parse(capturedAt) && (v.archiveVersion === 1 || v.archiveVersion === 2) && v.archiveSha256 === sha256 && v.archiveBytes === bytes && integer(v.archiveBytes, 1, 20971520) && integer(v.repositoryBytes, 1, 8388608) && integer(v.capturedRecordCount, 0, 10000) && integer(v.blobCount, 0, 1000) && integer(v.blobBytes, 0, 8388608) && v.preferences === (v.archiveVersion === 1 ? 'not_captured_v1' : 'captured_app_bsky_only') && v.integrity === 'authenticated_archive_and_member_hashes' && v.signatureVerification === 'not_rechecked' && v.canTransfer === false && v.canSwitchIdentity === false && Array.isArray(v.remainingGates) && v.remainingGates.length === gates.length && v.remainingGates.every((gate, index) => gate === gates[index]);
}

export function MigrationInventory({ exportId, deploymentId, sha256, bytes, capturedAt, onAccessDenied }: { exportId: string; deploymentId: string; sha256: string; bytes: number; capturedAt: string; onAccessDenied: () => void }) {
 const [value, setValue] = useState<Inventory | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
 const active = useRef<AbortController | null>(null), result = useRef<HTMLParagraphElement | null>(null), failure = useRef<HTMLParagraphElement | null>(null);
 useEffect(() => { if (value) result.current?.focus(); }, [value]);
 useEffect(() => { if (error) failure.current?.focus(); }, [error]);
 useEffect(() => { active.current?.abort(); active.current = null; setValue(null); setError(''); setBusy(false); return () => { active.current?.abort(); active.current = null; }; }, [exportId, deploymentId, sha256, bytes, capturedAt]);
 async function inspect() {
  if (busy) return;
  const controller = new AbortController(); active.current = controller; setValue(null); setError(''); setBusy(true);
  try {
   const response = await fetch(`/api/exports/${exportId}/migration-preflight`, { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
   const body: unknown = await response.json();
   if (active.current !== controller || controller.signal.aborted) return;
   if ([401, 403, 404].includes(response.status)) { onAccessDenied(); return; }
   if (!response.ok || !valid(body, exportId, deploymentId, sha256, bytes, capturedAt)) { setError('Migration inventory could not be confirmed. Refresh export status before retrying.'); return; }
   setValue(body);
  } catch { if (active.current === controller && !controller.signal.aborted) setError('Migration inventory could not be confirmed. Refresh export status before retrying.'); }
  finally { if (active.current === controller && !controller.signal.aborted) setBusy(false); }
 }
 return <div className="migration-inventory" aria-busy={busy}>
  <button type="button" disabled={busy} onClick={() => { void inspect(); }}>{busy ? 'Reading migration inventory...' : 'Inspect migration inventory'}</button>
  <p>This read checks the retained export. It does not contact a target or move your account identity.</p>
  {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}
  {value && <>
   <p role="status" ref={result} tabIndex={-1}>Captured archive inventory checked. Transfer and identity switching are unavailable.</p>
   <dl><dt>Captured</dt><dd>{new Date(value.capturedAt).toLocaleString()}</dd><dt>Records in captured manifest</dt><dd>{value.capturedRecordCount.toLocaleString()}</dd><dt>Repository size</dt><dd>{value.repositoryBytes.toLocaleString()} bytes</dd><dt>Referenced media</dt><dd>{value.blobCount.toLocaleString()} blobs, {value.blobBytes.toLocaleString()} bytes</dd><dt>Preference coverage</dt><dd>{value.preferences === 'not_captured_v1' ? 'Not captured in this version 1 archive.' : 'Captured Bluesky settings only. External application settings are outside this export.'}</dd><dt>Integrity</dt><dd>Authenticated archive and member hashes match the retained checkpoint. Repository signatures were not rechecked by this read.</dd></dl>
   <p>Capture time is historical. Current source access and identity control still require qualification.</p>
   <p>Remaining migration checks</p><ul>{labels.map(label => <li key={label}>{label}</li>)}</ul>
  </>}
 </div>;
}
