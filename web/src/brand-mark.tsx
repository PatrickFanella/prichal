// Prichal docking-node mark. Geometry matches brand/build.py; the red flange is the occupied port.
const port = (angle: number, fill: string) => <g key={angle} transform={`rotate(${angle} 256 256)`} fill={fill}><rect x="218" y="62" width="76" height="70" fill="var(--ink)"/><rect x="196" y="36" width="120" height="34"/></g>;

export function BrandMark({ size = 40 }: { size?: number }) {
  return <svg className="brand-mark" width={size} height={size} viewBox="0 0 512 512" aria-hidden="true" focusable="false">
    {[0, 180, 270].map((a) => port(a, 'var(--ink)'))}{port(90, 'var(--signal)')}
    <circle cx="256" cy="256" r="134" fill="var(--paper)" stroke="var(--ink)" strokeWidth="26"/>
    <path d="M 122 256 A 134 44 0 0 0 390 256" fill="none" stroke="var(--ink)" strokeWidth="14"/>
    <path d="M 122 256 A 134 44 0 0 1 390 256" fill="none" stroke="var(--ink)" strokeWidth="6" strokeDasharray="14 12"/>
  </svg>;
}
