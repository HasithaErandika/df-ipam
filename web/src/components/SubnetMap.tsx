// Decorative address map: a field of cells, like a subnet grid seen from afar.
// The pattern is fixed, so it never suggests real data.
const cols = 22
const rows = 26

function shade(i: number): string {
  let h = Math.imul(i ^ 0x9e3779b9, 0x85ebca6b)
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35)
  const n = ((h ^ (h >>> 16)) >>> 0) % 97
  if (n < 9) return 'var(--map-a)'
  if (n < 13) return 'var(--map-b)'
  if (n < 20) return 'var(--map-c)'
  return n < 52 ? 'var(--map-1)' : 'var(--map-2)'
}

export function SubnetMap() {
  return (
    <svg aria-hidden viewBox={`0 0 ${cols * 22} ${rows * 22}`} preserveAspectRatio="xMidYMid slice" className="absolute inset-0 h-full w-full">
      {Array.from({ length: cols * rows }, (_, i) => (
        <rect key={i} x={(i % cols) * 22 + 2} y={Math.floor(i / cols) * 22 + 2} width="18" height="18" rx="3" style={{ fill: shade(i + 1) }} />
      ))}
    </svg>
  )
}
