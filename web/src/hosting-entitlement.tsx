import { useEffect, useState } from 'react';
type Entitlement = { state: string; source: string; plan: string; instanceLimit: number; allocatedInstances: number; availableInstances: number; canDeploy: boolean; expiresAt: string | null };
function valid(v: unknown): v is Entitlement { if(typeof v !== 'object' || v===null) return false; const x=v as Record<string,unknown>; return ['unconfigured','active','expired','revoked'].includes(String(x.state)) && ['none','local_pilot'].includes(String(x.source)) && typeof x.plan==='string' && [x.instanceLimit,x.allocatedInstances,x.availableInstances].every(n=>typeof n==='number'&&Number.isSafeInteger(n)&&n>=0) && typeof x.canDeploy==='boolean' && (x.expiresAt===null || typeof x.expiresAt==='string'&&Number.isFinite(Date.parse(x.expiresAt))); }
export function HostingEntitlement({ organizationId }: { organizationId: string }) {
  const [status, setStatus] = useState<Entitlement | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try { const response=await fetch(`/api/organizations/${organizationId}/entitlement`, {cache:'no-store',signal:AbortSignal.any([controller.signal,AbortSignal.timeout(5000)])}); const result:unknown=await response.json(); if(!response.ok||!valid(result)) throw new Error(); if(!controller.signal.aborted){setStatus(result);setUnavailable(false);} }
      catch {if(!controller.signal.aborted) setUnavailable(true);}
    }
    void load();const timer=window.setInterval(()=>{void load();},10000);return()=>{controller.abort();window.clearInterval(timer);};
  },[organizationId]);
  if(unavailable) return <p role="status">Hosting allowance could not be confirmed.</p>;
  if(!status) return <p role="status">Checking hosting allowance...</p>;
  return <div>
    <p>Hosting access: {status.state === 'active' ? 'Local pilot' : status.state === 'unconfigured' ? 'Not configured' : status.state}.</p>
    {status.source==='local_pilot' && <p>{status.allocatedInstances} retained native deployment request{status.allocatedInstances===1?'':'s'}; allowance {status.instanceLimit}. {status.expiresAt && <>Expires {new Date(status.expiresAt).toLocaleString()}.</>}</p>}
    {!status.canDeploy && <p>New native deployments are blocked. Existing servers and data are retained. Contact support for hosting access or allowance changes.</p>}
  </div>;
}
