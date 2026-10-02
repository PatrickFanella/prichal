import { useEffect, useRef, useState } from 'react';
import { checkoutIntent, checkoutPlan, checkoutReceipt, checkoutNavigation } from './checkout-contract';
import type { CheckoutIntent, CheckoutPlan, CheckoutReceipt, CheckoutNavigation } from './checkout-contract';

export function CheckoutWorkspace({ownerId,organizationId,csrfToken}:{ownerId:string;organizationId:string;csrfToken:string}) {
 const storageKey=`pds-checkout-intent:${ownerId}:${organizationId}`;
 const base=`/api/organizations/${organizationId}/billing/checkout`;
 const [intent,setIntent]=useState<CheckoutIntent|null>(null),[blocked,setBlocked]=useState(false),[plan,setPlan]=useState<CheckoutPlan|null>(null),[receipt,setReceipt]=useState<CheckoutReceipt|null>(null);
 const [known,setKnown]=useState(false),[busy,setBusy]=useState(false),[reviewed,setReviewed]=useState(false),[cleanup,setCleanup]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState(''),[navigation,setNavigation]=useState<CheckoutNavigation|null>(null);
 const request=useRef<AbortController|null>(null),heading=useRef<HTMLHeadingElement|null>(null),failure=useRef<HTMLParagraphElement|null>(null),result=useRef<HTMLParagraphElement|null>(null);
 useEffect(()=>{if(error)failure.current?.focus();},[error]);
 useEffect(()=>{if(notice)result.current?.focus();},[notice]);
 useEffect(()=>{if(intent||blocked){const panel=heading.current?.closest('details');if(panel)panel.open=true;heading.current?.focus();}},[intent,blocked]);
 useEffect(()=>{if(navigation?.status!=='open')return;const timer=window.setTimeout(()=>setNavigation({status:'expired',expiresAt:navigation.expiresAt}),Math.max(0,Date.parse(navigation.expiresAt)-Date.now()));return()=>window.clearTimeout(timer);},[navigation]);
 async function refresh(retained:CheckoutIntent|null=intent) {
  request.current?.abort();const controller=new AbortController();request.current=controller;
  setBusy(true);setKnown(false);setPlan(null);setReceipt(null);setNavigation(null);setReviewed(false);setCleanup(false);setError('');setNotice('');
  try {
   const options={cache:'no-store',signal:AbortSignal.any([controller.signal,AbortSignal.timeout(8000)])} satisfies RequestInit;
   const [catalog,original]=await Promise.all([fetch(`${base}-plan`,options),retained?fetch(`${base}?key=${retained.key}`,options):Promise.resolve(null)]);
   const catalogValue:unknown=await catalog.json();
   let found:CheckoutReceipt|null=null;
   if(original&&original.status!==404){const value:unknown=await original.json();if(!original.ok||!retained||!checkoutReceipt(value,organizationId,retained))throw new Error();found=value;}
   if(!catalog.ok||!checkoutPlan(catalogValue)||retained&&catalogValue.priceId!==retained.priceId||retained&&!found&&!catalogValue.canPrepare)throw new Error();
   if(request.current!==controller||controller.signal.aborted)return;
   setPlan(catalogValue);setReceipt(found);setKnown(true);
  } catch {if(request.current===controller&&!controller.signal.aborted)setError('Checkout could not be confirmed. Keep the original request and refresh current access. Billing may be unavailable.');}
  finally {if(request.current===controller&&!controller.signal.aborted)setBusy(false);}
 }
 useEffect(()=>{
  let retained:CheckoutIntent|null=null;
  try{const raw=sessionStorage.getItem(storageKey);if(raw){const value:unknown=JSON.parse(raw);if(!checkoutIntent(value))throw new Error();retained=value;setIntent(value);}}
  catch{setBlocked(true);}
  void refresh(retained);
  return()=>{request.current?.abort();request.current=null;};
 },[storageKey]);
 function stage(){
  if(!known||busy||blocked||intent||!plan?.canPrepare)return;
  try{const value={key:crypto.randomUUID(),priceId:plan.priceId};sessionStorage.setItem(storageKey,JSON.stringify(value));setIntent(value);setReviewed(false);setNotice('Review this fixture plan before preparing it. No payment request has been sent.');}
  catch{setError('Session storage is required to retain the original checkout request.');}
 }
 async function submit(){
  if(!known||busy||blocked||!intent||!reviewed||!plan||plan.priceId!==intent.priceId||receipt&&receipt.state!=='prepared'||!receipt&&!plan.canPrepare)return;
  const retained=intent,prior=receipt,controller=new AbortController();request.current=controller;
  setBusy(true);setKnown(false);setReviewed(false);setCleanup(false);setNavigation(null);setError('');setNotice('');
  try{
   sessionStorage.setItem(storageKey,JSON.stringify(retained));
   const response=await fetch(prior?`${base}/${prior.id}/start`:base,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrfToken},body:JSON.stringify(prior?{confirmed:true}:{idempotencyKey:retained.key,confirmed:true}),signal:AbortSignal.any([controller.signal,AbortSignal.timeout(8000)])});
   const value:unknown=await response.json();
   if(request.current!==controller||controller.signal.aborted)return;
   if(!response.ok||!checkoutReceipt(value,organizationId,retained))throw new Error();
   setReceipt(value);setNotice(prior?'The original dispatch receipt was returned. Refresh it before continuing.':'The configured plan was prepared. Refresh and review it again before dispatch.');
  }catch{if(request.current===controller&&!controller.signal.aborted){setReceipt(null);setError('The result could not be confirmed. Refresh the original receipt before retrying.');}}
  finally{if(request.current===controller&&!controller.signal.aborted)setBusy(false);}
 }
 async function recover(){
  if(!known||busy||!receipt||receipt.state!=='session_ready')return;
  const controller=new AbortController();request.current=controller;setBusy(true);setNavigation(null);setReviewed(false);setCleanup(false);setError('');setNotice('');
  try{
   const response=await fetch(`${base}/${receipt.id}/session`,{cache:'no-store',signal:AbortSignal.any([controller.signal,AbortSignal.timeout(8000)])});const value:unknown=await response.json();
   if(request.current!==controller||controller.signal.aborted)return;
   if(!response.ok||!checkoutNavigation(value))throw new Error();
   setNavigation(value);setNotice(value.status==='open'?'The original fixture session is open. Its temporary link expires at the time shown.':'The original session is closed. No replacement checkout was created.');
  }catch{if(request.current===controller&&!controller.signal.aborted){setKnown(false);setReceipt(null);setError('The original hosted session could not be confirmed. Refresh the receipt before trying another read.');}}
  finally{if(request.current===controller&&!controller.signal.aborted)setBusy(false);}
 }
 function clear(){
  if(!known||busy||!cleanup||blocked||!receipt||!['session_ready','manual_review'].includes(receipt.state))return;
  try{sessionStorage.removeItem(storageKey);setIntent(null);setReviewed(false);setCleanup(false);setNavigation(null);setNotice('The browser request was cleared. The server receipt and any remote resources remain retained.');}
  catch{setError('The browser request could not be cleared.');}
 }
 return <details className="checkout-workspace"><summary>Checkout review</summary><section aria-labelledby={`checkout-title-${organizationId}`} aria-busy={busy}>
  <h4 id={`checkout-title-${organizationId}`} ref={heading} tabIndex={-1}>Original checkout request</h4>
  <p>Local fixture checkout is separate from hosting access. A return page or completed session does not grant hosting.</p>
  {error&&<p role="alert" ref={failure} tabIndex={-1}>{error}</p>}{notice&&<p role="status" ref={result} tabIndex={-1}>{notice}</p>}
  <button type="button" disabled={busy} onClick={()=>{void refresh();}}>Refresh original checkout receipt</button>
  {blocked&&<p>Retained browser metadata could not be validated. Keep it and contact support before creating another request.</p>}
  {known&&plan&&<p>Configured fixture Price: <code>{plan.priceId}</code>. No commercial price or paid hosting allowance is offered here.</p>}
  {known&&!intent&&!blocked&&plan?.canPrepare&&<button type="button" disabled={busy} onClick={stage}>Review configured fixture plan</button>}
  {known&&!intent&&!blocked&&plan&&!plan.canPrepare&&<p>This organization already has retained checkout, billing or handover work. Recover its original request or contact support.</p>}
  {intent&&<p>Retained fixture Price: <code>{intent.priceId}</code>. Refresh always reads this original request.</p>}
  {known&&receipt&&<p>Original checkout state: {receipt.state}. {receipt.state==='manual_review'?'Support review is required. No replacement or renewed request is available.':receipt.state==='queued'||receipt.state==='customer_ready'?'Work is pending. Refresh its receipt; do not create another request.':''}</p>}
  {known&&intent&&!blocked&&(!receipt||receipt.state==='prepared')&&<div>
   <label><input type="checkbox" checked={reviewed} disabled={busy} onChange={event=>setReviewed(event.target.checked)}/>I reviewed this original fixture Price and {receipt?'authorize dispatch to create its customer and hosted checkout':'want to prepare it without dispatch'}.</label>
   <button type="button" disabled={busy||!reviewed} onClick={()=>{void submit();}}>{receipt?'Dispatch original checkout request':'Prepare original fixture plan'}</button>
  </div>}
  {known&&receipt?.state==='session_ready'&&<button type="button" disabled={busy} onClick={()=>{void recover();}}>Read original hosted session</button>}
  {navigation&&<p>Session: {navigation.status}. Expires {new Date(navigation.expiresAt).toLocaleString()}. {navigation.status==='open'&&navigation.url&&Date.parse(navigation.expiresAt)>Date.now()&&<a href={navigation.url} rel="noreferrer" referrerPolicy="no-referrer">Open original fixture checkout</a>}</p>}
  {known&&intent&&receipt&&['session_ready','manual_review'].includes(receipt.state)&&<div><label><input type="checkbox" checked={cleanup} disabled={busy} onChange={event=>setCleanup(event.target.checked)}/>I checked this receipt and understand clearing affects only browser metadata.</label><button type="button" disabled={busy||!cleanup} onClick={clear}>Clear confirmed browser request</button></div>}
 </section></details>;
}
