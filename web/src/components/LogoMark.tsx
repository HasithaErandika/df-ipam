import { useId } from 'react'

/** The DF-Atlas mark. Each copy has its own gradient id, so two on a page don't clash. */
export function LogoMark({ size = 30 }: { size?: number }) {
  const id = `dfipam-logo-${useId().replace(/:/g, '')}`
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden className="shrink-0">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="var(--brand-deep)" />
          <stop offset="1" stopColor="var(--brand-night)" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="8" fill={`url(#${id})`} />
      <rect x="7" y="7" width="8" height="8" rx="1.5" fill="var(--brand-light)" />
      <rect x="17" y="17" width="8" height="8" rx="1.5" fill="var(--brand-light)" />
      <rect x="17" y="7" width="8" height="8" rx="1.5" fill="#fff" opacity=".6" />
      <rect x="7" y="17" width="8" height="8" rx="1.5" fill="#fff" opacity=".6" />
    </svg>
  )
}
