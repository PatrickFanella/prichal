import { useEffect, useRef, useState } from 'react';

type HostingNode = { id: string; region: string; state: string; operatorSlots: number; slots: number; reserved: number; available: number; fresh: boolean; cpus: number; memoryBytes: number; checkedAt: string | null };
function object(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null && !Array.isArray(value); }
function valid(value: unknown): value is HostingNode {
  return object(value) && typeof value.id === 'string' && /^[a-f0-9-]{36}$/.test(value.id)
    && typeof value.region === 'string' && ['pending', 'active', 'draining'].includes(String(value.state))
    && typeof value.fresh === 'boolean' && ['operatorSlots', 'slots', 'reserved', 'available', 'cpus', 'memoryBytes'].every(key => Number.isSafeInteger(value[key]) && Number(value[key]) >= 0)
    && (value.checkedAt === null || typeof value.checkedAt === 'string' && Number.isFinite(Date.parse(value.checkedAt)));
}

export function HostingNodes({ ownerId }: { ownerId: string }) {
  const [nodes, setNodes] = useState<HostingNode[]>([]), [error, setError] = useState(''), [busy, setBusy] = useState(false), [more, setMore] = useState(false);
  const request = useRef<AbortController | null>(null);
  async function refresh() {
    if (request.current) return;
    const controller = new AbortController(); request.current = controller;
    setNodes([]); setMore(false); setError(''); setBusy(true);
    try {
      const response = await fetch('/api/operator/nodes', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]) });
      const value: unknown = await response.json();
      if (request.current !== controller || controller.signal.aborted) return;
      if (!response.ok || !object(value) || !Array.isArray(value.items) || value.items.length > 100 || !value.items.every(valid) || typeof value.hasMore !== 'boolean') throw Error('Inventory unavailable');
      setNodes(value.items); setMore(value.hasMore);
    } catch {
      if (request.current === controller && !controller.signal.aborted) { setNodes([]); setMore(false); setError('Hosting inventory could not be verified. Refresh to check current operator access and measurements.'); }
    } finally { if (request.current === controller) { request.current = null; if (!controller.signal.aborted) setBusy(false); } }
  }
  useEffect(() => { setNodes([]); setError(''); setMore(false); void refresh(); const timer = window.setInterval(() => { void refresh(); },15000); return () => { window.clearInterval(timer); request.current?.abort(); request.current = null; }; }, [ownerId]);
  return <section aria-labelledby="hosting-nodes-heading"><h4 id="hosting-nodes-heading">Hosting nodes</h4>
    <p>Capacity is measured by the private host observer. Stale measurements do not admit new placements.</p>
    <button type="button" disabled={busy} onClick={() => { void refresh(); }}>Refresh hosting nodes</button>
    {busy && <p role="status">Checking hosting inventory…</p>}
    {error && <p role="alert">{error}</p>}
    {!busy && !error && nodes.length === 0 && <p>No hosting nodes are registered.</p>}
    {nodes.map(node => <article key={node.id}><h5>Node {node.id}</h5><p>{node.region} · {node.state} · {node.fresh ? 'Verified measurement' : 'Measurement unavailable or stale'}</p>
      <dl><div><dt>Available slots</dt><dd>{node.available}</dd></div><div><dt>Reserved slots</dt><dd>{node.reserved}</dd></div><div><dt>Operator ceiling</dt><dd>{node.operatorSlots}</dd></div><div><dt>Measured resources</dt><dd>{node.cpus || 'Unknown'} CPUs · {node.memoryBytes ? `${(node.memoryBytes / 1073741824).toFixed(1)} GiB RAM` : 'Unknown RAM'}</dd></div><div><dt>Last verified</dt><dd>{node.checkedAt ? new Date(node.checkedAt).toLocaleString() : 'Not yet verified'}</dd></div></dl>
    </article>)}{more && <p>Only the first 100 nodes are shown.</p>}
  </section>;
}
