// A deployment endpoint is empty until known, a private local endpoint, or the
// generated managed public endpoint once public readiness has passed.
export function deploymentEndpoint(value: unknown, hostname: unknown, id: unknown): boolean {
  if (value === '') return true;
  if (typeof value !== 'string' || typeof hostname !== 'string' || typeof id !== 'string') return false;
  try {
    const endpoint = new URL(value);
    if (endpoint.protocol !== 'https:' || endpoint.hostname !== hostname || endpoint.username || endpoint.password || endpoint.pathname !== '/' || endpoint.search || endpoint.hash) return false;
    if (/^[a-f0-9-]{36}\.localhost$/.test(hostname)) return Number(endpoint.port) >= 36000 && Number(endpoint.port) < 46000;
    return endpoint.port === '' && /^pds-[a-f0-9]{32}(\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/.test(hostname) && hostname.startsWith(`pds-${id.replaceAll('-', '')}.`);
  } catch { return false; }
}
