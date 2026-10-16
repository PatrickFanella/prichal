import { useEffect, useState } from 'react';

function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null && !Array.isArray(value); }
function date(value: unknown): value is string { return typeof value === 'string' && Number.isFinite(Date.parse(value)); }
const digest = /^sha256:[a-f0-9]{64}$/;
type Release = { state: 'unknown' | 'observed' | 'stale'; checkedAt: string | null; runtime: null | { version?: string; tag: string; digest: string; imageRef: string; imageId: string } };
function release(value: unknown): value is Release {
  if (!object(value) || !['unknown', 'observed', 'stale'].includes(String(value.state)) || !(value.checkedAt === null || date(value.checkedAt))) return false;
  if (value.runtime === null) return value.state !== 'observed';
  const runtime = value.runtime;
  if (!object(runtime)) return false;
  return value.state !== 'unknown' && (runtime.version === undefined || runtime.version === '' || typeof runtime.version === 'string' && runtime.version.length<=128 && /^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$/.test(runtime.version)) && date(value.checkedAt) && typeof runtime.tag === 'string' && /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/.test(runtime.tag) && typeof runtime.digest === 'string' && digest.test(runtime.digest) && typeof runtime.imageId === 'string' && digest.test(runtime.imageId) && runtime.imageRef === `ghcr.io/bluesky-social/pds:${runtime.tag}@${runtime.digest}`;
}

export function PDSRelease({ value }: { value: unknown }) {
  const observation = release(value) ? value : null;
  return <p className="pds-release">PDS release: {observation?.runtime ? <><span>{observation.runtime.version || `image tag ${observation.runtime.tag}`}</span> (<code>{observation.runtime.digest.slice(0, 19)}</code>){observation.state === 'stale' ? '. Last observation is stale' : '. Observed running'}</> : 'unknown'}.
    {observation?.checkedAt && <> Last checked {new Date(observation.checkedAt).toLocaleString()}.</>}
    {observation?.runtime && <> {observation.runtime.version ? 'Version from the inspected image label.' : 'Exact package version is unknown.'}</>}
  </p>;
}

export function UpstreamPDSRelease() {
  const [view, setView] = useState<{ state: string; tag: string; checkedAt: string | null } | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const response = await fetch('/api/operator/pds-upstream-release', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(5000)]) });
        const value: unknown = await response.json();
        if (!response.ok || !object(value) || !['unknown', 'observed'].includes(String(value.state)) || !(value.checkedAt === null || date(value.checkedAt)) || typeof value.tag !== 'string' || (value.state === 'observed' ? !/^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$/.test(value.tag) || value.tag.length > 128 || !date(value.checkedAt) : value.tag !== '')) throw Error('Invalid release');
        if (!controller.signal.aborted) setView(value as { state: string; tag: string; checkedAt: string | null });
      } catch { if (!controller.signal.aborted) setView(null); }
    }
    void load(); const timer = window.setInterval(() => { void load(); }, 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, []);
  return <p className="pds-release">Latest upstream PDS release: {view?.state === 'observed' ? view.tag : 'unknown'}. Last checked {view?.checkedAt ? new Date(view.checkedAt).toLocaleString() : 'unknown'}.
    {' '}<a href="https://github.com/bluesky-social/pds/releases" target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">Upstream releases</a>. This does not indicate a qualified upgrade. No upgrade is started here.
  </p>;
}
