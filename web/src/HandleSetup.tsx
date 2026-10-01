import { useEffect, useState } from 'react';
type Configuration = { deploymentId: string; handle: string; did: string; endpoint: string; txtName: string; txtValue: string; state: string; message: string; checkedAt: string | null };
function valid(value: unknown, deploymentId: string): value is Configuration {
 if (!value || typeof value !== 'object') return false;
 const row = value as Record<string, unknown>;
 return row.deploymentId === deploymentId && ['handle','did','endpoint','txtName','txtValue','state','message'].every(key => typeof row[key] === 'string') && (row.checkedAt === null || typeof row.checkedAt === 'string');
}
export function HandleSetup({ deploymentId, csrfToken }: { deploymentId: string; csrfToken: string }) {
 const [handle,setHandle] = useState(''), [configuration,setConfiguration] = useState<Configuration|null>(null), [error,setError] = useState(''), [busy,setBusy] = useState(false);
 useEffect(() => {const controller = new AbortController();void fetch(`/api/deployments/${deploymentId}/handle`,{signal:controller.signal}).then(async response => { if(response.status===404) return; const value:unknown=await response.json();if(!response.ok || !valid(value,deploymentId)) throw new Error();setConfiguration(value);setHandle(value.handle); }).catch(()=>{if(!controller.signal.aborted)setError('Handle status is temporarily unavailable.');});return ()=>controller.abort();},[deploymentId]);
 async function action(check: boolean) {
  setBusy(true);setError('');
  try { const response=await fetch(`/api/deployments/${deploymentId}/handle${check?'/check':''}`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrfToken},body:JSON.stringify(check?{}:{handle})});const value:unknown=await response.json();if(!response.ok || !valid(value,deploymentId)){if(value && typeof value==='object' && 'error' in value && typeof value.error==='string')setError(value.error);else setError('Handle request could not be confirmed.');return;}setConfiguration(value);setHandle(value.handle); }catch{setError('Handle request could not be confirmed. Retry the same domain.');}finally{setBusy(false);}
 }
 return <section className="handle-setup" aria-label="Customer domain handle"><h4>Your domain handle</h4><p>Use a domain you own for your account handle. Your PDS address and stored data stay where they are.</p>
 <form onSubmit={event=>{event.preventDefault();void action(false);}}><label htmlFor={`handle-${deploymentId}`}>Domain hostname</label><input id={`handle-${deploymentId}`} value={handle} onChange={event=>setHandle(event.target.value)} maxLength={244} required placeholder="creator.yourdomain.com" autoCapitalize="none" autoCorrect="off" spellCheck={false}/><button type="submit" disabled={busy}>{busy?'Saving…':configuration?'Change domain instructions':'Prepare domain instructions'}</button></form>
 {configuration && <><p>Add this TXT record at your DNS provider. You do not need to transfer your domain.</p><dl><dt>Name</dt><dd><code>{configuration.txtName}</code></dd><dt>Value</dt><dd><code>{configuration.txtValue}</code></dd></dl><p>After DNS matches, sign in to your native AT account through a compatible client and change its handle to <code>{configuration.handle}</code>. That sign-in proves account control.</p><p role="status">{configuration.state}: {configuration.message}</p><button type="button" disabled={busy} onClick={()=>{void action(true);}}>Check DNS and account identity</button><p>Checks are limited to once every 30 seconds. To undo a handle change, change it back through your native account and configure that handle here. Domain proof can expire if you remove its TXT record.</p></>}
 {error && <p role="alert">{error}</p>}</section>;
}
