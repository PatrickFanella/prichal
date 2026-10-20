import { useEffect, useRef, useState } from 'react';
type Configuration = { deploymentId: string; handle: string; did: string; endpoint: string; txtName: string; txtValue: string; state: string; message: string; checkedAt: string | null };
function valid(value: unknown, deploymentId: string): value is Configuration {
 if (!value || typeof value !== 'object') return false;
 const row = value as Record<string, unknown>;
 return row.deploymentId === deploymentId && ['handle','did','endpoint','txtName','txtValue','state','message'].every(key => typeof row[key] === 'string') && (row.checkedAt === null || typeof row.checkedAt === 'string');
}
export function HandleSetup({ deploymentId, csrfToken }: { deploymentId: string; csrfToken: string }) {
 const [handle,setHandle] = useState(''), [configuration,setConfiguration] = useState<Configuration|null>(null), [error,setError] = useState(''), [busy,setBusy] = useState(false);
 const [loadError,setLoadError] = useState(''), [loading,setLoading] = useState(true), [readVersion,setReadVersion] = useState(0);
 const read = useRef<AbortController|null>(null), mutation = useRef<AbortController|null>(null);
 const failure = useRef<HTMLParagraphElement|null>(null), result = useRef<HTMLParagraphElement|null>(null), focusResult = useRef(false);
 useEffect(()=>{if(error)failure.current?.focus();},[error]);
 useEffect(()=>{if(focusResult.current&&configuration){result.current?.focus();focusResult.current=false;}},[configuration]);
 useEffect(() => {
  const controller = new AbortController();read.current=controller;
  setConfiguration(null);setHandle('');setError('');setLoadError('');setLoading(true);setBusy(false);focusResult.current=false;
  void(async()=>{
   try{
    const response=await fetch(`/api/deployments/${deploymentId}/handle`,{cache:'no-store',signal:AbortSignal.any([controller.signal,AbortSignal.timeout(8000)])});
    if(response.status===404)return;
    const value:unknown=await response.json();if(controller.signal.aborted)return;
    if(!response.ok||!valid(value,deploymentId))throw new Error('Invalid handle status');
    setConfiguration(value);setHandle(value.handle);
   }catch{if(!controller.signal.aborted)setLoadError('Handle status is temporarily unavailable.');}
   finally{if(!controller.signal.aborted)setLoading(false);}
  })();
  return()=>{controller.abort();if(read.current===controller)read.current=null;mutation.current?.abort();mutation.current=null;focusResult.current=false;};
 },[deploymentId,csrfToken,readVersion]);
 function edit(value:string){read.current?.abort();setLoading(false);setLoadError('');setHandle(value);}
 async function action(check: boolean) {
  if(mutation.current||(check&&(!configuration||configuration.deploymentId!==deploymentId||handle!==configuration.handle)))return;
  read.current?.abort();setLoading(false);setLoadError('');
  const controller=new AbortController();mutation.current=controller;setBusy(true);setError('');
  try {
   const response=await fetch(`/api/deployments/${deploymentId}/handle${check?'/check':''}`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrfToken},body:JSON.stringify(check?{}:{handle}),signal:AbortSignal.any([controller.signal,AbortSignal.timeout(8000)])});
   const value:unknown=await response.json();if(controller.signal.aborted)return;
   if(!response.ok || !valid(value,deploymentId)){if(value && typeof value==='object' && 'error' in value && typeof value.error==='string')setError(value.error);else setError('Handle request could not be confirmed.');return;}
   focusResult.current=true;setConfiguration(value);setHandle(value.handle);
  }catch{if(!controller.signal.aborted)setError('Handle request could not be confirmed. Retry the same domain.');}
  finally{if(mutation.current===controller){mutation.current=null;setBusy(false);}}
 }
 const saved=configuration?.deploymentId===deploymentId&&handle===configuration.handle;
 return <section className="handle-setup" aria-label="Customer domain handle" aria-busy={busy}><h4>Your name as your username</h4><p>Use a domain you own as your username (your handle), such as <code>@yourband.com</code>. Your server address and stored data stay where they are.</p>
 {loading&&<p role="status">Loading saved domain instructions…</p>}
 {loadError&&<><p role="alert">{loadError}</p><button type="button" disabled={busy} onClick={()=>setReadVersion(value=>value+1)}>Retry domain status</button></>}
 <form onSubmit={event=>{event.preventDefault();void action(false);}}><label htmlFor={`handle-${deploymentId}`}>Domain hostname</label><input id={`handle-${deploymentId}`} value={handle} onChange={event=>edit(event.target.value)} disabled={busy} maxLength={244} required placeholder="creator.yourdomain.com" autoCapitalize="none" autoCorrect="off" spellCheck={false}/><button type="submit" disabled={busy}>{busy?'Saving…':configuration?'Change domain instructions':'Prepare domain instructions'}</button></form>
 {configuration?.deploymentId===deploymentId&&!saved&&<p role="status">Your edited domain is not saved. Save it to prepare its own DNS instructions before changing DNS.</p>}
 {saved&&configuration && <><p>Add this TXT record at your DNS provider. You do not need to transfer your domain.</p><dl><dt>Name</dt><dd><code>{configuration.txtName}</code></dd><dt>Value</dt><dd><code>{configuration.txtValue}</code></dd></dl><p>After DNS matches, sign in to your native AT account through a compatible client and change its handle to <code>{configuration.handle}</code>. That sign-in proves account control.</p><p role="status" ref={result} tabIndex={-1}>{configuration.state}: {configuration.message}</p><button type="button" disabled={busy} onClick={()=>{void action(true);}}>Check DNS and account identity</button><p>Checks are limited to once every 30 seconds. To undo a handle change, change it back through your native account and configure that handle here. Domain proof can expire if you remove its TXT record.</p></>}
 {error && <p role="alert" ref={failure} tabIndex={-1}>{error}</p>}</section>;
}
