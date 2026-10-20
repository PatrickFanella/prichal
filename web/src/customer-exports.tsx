import { useEffect, useRef, useState } from 'react';
import { HostingExit } from './hosting-exit';
import { MigrationInventory } from './migration-inventory';

type Export = {id:string; deploymentId:string; state:'queued'|'ready'|'failed'; bytes:number|null; sha256:string|null; capturedAt:string|null; expiresAt:string|null; downloadAvailable:boolean};
const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
function valid(value:unknown, deployment:string):value is Export {
 if(typeof value!=='object'||value===null)return false;
 const v=value as Record<string,unknown>;
 return typeof v.id==='string'&&uuid.test(v.id)&&v.deploymentId===deployment&&['queued','ready','failed'].includes(String(v.state))&&typeof v.downloadAvailable==='boolean'&&(v.bytes===null||typeof v.bytes==='number'&&Number.isSafeInteger(v.bytes)&&v.bytes>0&&v.bytes<=20971520)&&(v.sha256===null||typeof v.sha256==='string'&&/^[a-f0-9]{64}$/.test(v.sha256))&&[v.capturedAt,v.expiresAt].every(x=>x===null||typeof x==='string'&&Number.isFinite(Date.parse(x)));
}
export function CustomerExports({deploymentId,csrfToken}:{deploymentId:string;csrfToken:string}) {
 const storageKey=`pds-portable-export:${deploymentId}`;
 const [key,setKey]=useState(()=>{try{const value=sessionStorage.getItem(storageKey);if(value&&uuid.test(value))return value;}catch{/* Storage failure is reported before submission. */}return '';});
 const failure=useRef<HTMLParagraphElement|null>(null);
 const sequence=useRef(0),busyRef=useRef(false),mutation=useRef<AbortController|null>(null);
 const [items,setItems]=useState<Export[]>([]),[busy,setBusy]=useState(false),[error,setError]=useState(''),[loadError,setLoadError]=useState(''),[loaded,setLoaded]=useState(false);
 useEffect(()=>{if(error||loadError)failure.current?.focus();},[error,loadError]);
 useEffect(()=>{
  const controller=new AbortController();let timer:ReturnType<typeof setTimeout>;
  setItems([]);setLoaded(false);setError('');setLoadError('');setBusy(false);busyRef.current=false;
  try{const retained=sessionStorage.getItem(storageKey);setKey(retained&&uuid.test(retained)?retained:'');}catch{setKey('');}
  async function load(){
   if(!busyRef.current){const current=++sequence.current;try{const response=await fetch(`/api/deployments/${deploymentId}/exports`,{cache:'no-store',signal:AbortSignal.any([controller.signal,AbortSignal.timeout(5000)])});const body:unknown=await response.json();if(!response.ok||!Array.isArray(body)||!body.every(x=>valid(x,deploymentId)))throw new Error('Invalid export list');if(!controller.signal.aborted&&sequence.current===current){setItems(body);setLoaded(true);setLoadError('');}}catch{if(!controller.signal.aborted&&sequence.current===current){setItems([]);setLoaded(false);setLoadError('Export status is temporarily unavailable.');}}}
   if(!controller.signal.aborted)timer=setTimeout(()=>{void load();},3000);
  }
  void load();return()=>{controller.abort();clearTimeout(timer);sequence.current++;mutation.current?.abort();mutation.current=null;busyRef.current=false;};
 },[deploymentId,csrfToken]);
 async function submit(){
  if(busyRef.current||!loaded||(!key&&items.length>=3))return;
  const requestKey=key||crypto.randomUUID();setError('');
  try{sessionStorage.setItem(storageKey,requestKey);}catch{setError('This browser cannot retain a retry key. Enable session storage before requesting an export.');return;}
  const controller=new AbortController();mutation.current=controller;busyRef.current=true;sequence.current++;setKey(requestKey);setBusy(true);
  try{const response=await fetch(`/api/deployments/${deploymentId}/exports`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrfToken},body:JSON.stringify({idempotencyKey:requestKey}),signal:AbortSignal.any([controller.signal,AbortSignal.timeout(5000)])});const body:unknown=await response.json();if(controller.signal.aborted)return;if(!response.ok||!valid(body,deploymentId)){setError('The export request could not be confirmed. Retry uses the same request.');return;}setItems(current=>[body,...current.filter(x=>x.id!==body.id)]);sessionStorage.removeItem(storageKey);setKey('');}catch{if(!controller.signal.aborted)setError('The export request could not be confirmed. Retry uses the same request.');}finally{if(mutation.current===controller){mutation.current=null;busyRef.current=false;setBusy(false);}}
 }
 return <><section className="customer-exports" aria-labelledby={`exports-${deploymentId}`}>
  <h5 id={`exports-${deploymentId}`}>Download your data</h5>
  <p>Create a portable snapshot of your signed records, referenced media and private Bluesky settings. Protect the downloaded archive as private account data. Downloads are available for 24 hours. Local qualification supports three retained requests per server. Account holds preserve export access; a stopped service must resume before capture.</p>
  {(error||loadError)&&<p role="alert" ref={failure} tabIndex={-1}>{error||loadError}</p>}
  {!loaded&&!loadError&&<p role="status">Loading exports...</p>}
  <button type="button" disabled={busy||!loaded||(!key&&items.length>=3)} onClick={()=>{void submit();}}>{busy?'Requesting...':key?'Retry export request':'Create export'}</button>
  {key&&<p role="status">An unconfirmed request is retained in this browser tab. Retry to retrieve the same export.</p>}
  <ul>{items.map(item=><li key={item.id}><p role="status">Export {item.state}{item.capturedAt&&<>. Captured {new Date(item.capturedAt).toLocaleString()}</>}</p>{item.bytes!==null&&<p>{item.bytes.toLocaleString()} bytes. SHA-256: <code style={{overflowWrap:'anywhere'}}>{item.sha256}</code></p>}{item.state==='ready'&&item.expiresAt&&<p>Download window ends {new Date(item.expiresAt).toLocaleString()}.</p>}{item.downloadAvailable&&<a href={`/api/exports/${item.id}/download`}>Download portable archive</a>}{item.downloadAvailable&&item.sha256&&item.bytes&&item.capturedAt&&<MigrationInventory key={`${deploymentId}:${item.id}:${item.sha256}:${item.bytes}:${item.capturedAt}`} exportId={item.id} deploymentId={deploymentId} sha256={item.sha256} bytes={item.bytes} capturedAt={item.capturedAt} onAccessDenied={()=>{setItems([]);setLoaded(false);setLoadError('Export access could not be confirmed. Refresh status before retrying.');}}/>}{item.state==='ready'&&!item.downloadAvailable&&<p>The download window has ended. The retained snapshot remains available for support review.</p>}{item.state==='failed'&&<p>Capture failed. The data on your server is retained. Contact support before requesting another snapshot.</p>}</li>)}</ul>
 </section><HostingExit deploymentId={deploymentId} csrfToken={csrfToken}/></>;
}
