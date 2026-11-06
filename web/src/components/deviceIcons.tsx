import type { ReactNode, SVGProps } from 'react'

// Network symbols in the usual diagram style, drawn for this project on the same grid
// and stroke as the rest of the app's icons.
export type DeviceGlyphProps = SVGProps<SVGSVGElement> & { size?: number }

const glyph = (body: ReactNode) => function DeviceGlyph({ size = 24, ...rest }: DeviceGlyphProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" {...rest}>
      {body}
    </svg>
  )
}

/** A disc with paths crossing it: traffic in, traffic out. */
export const RouterGlyph = glyph(<>
  <circle cx="12" cy="12" r="10.25" />
  <path d="M13.5 10.5l4-4M14 6.5h3.5V10" /><path d="M10.5 13.5l-4 4M10 17.5H6.5V14" />
  <path d="M6.5 6.5l4 4M10.5 7v3.5H7" /><path d="M17.5 17.5l-4-4M13.5 17v-3.5H17" />
</>)

/** A box with traffic passing both ways. */
export const SwitchGlyph = glyph(<>
  <rect x="1.75" y="5" width="20.5" height="14" rx="2" />
  <path d="M6 9.5h11.5M15 7.25l2.5 2.25-2.5 2.25" /><path d="M18 14.5H6.5M9 12.25L6.5 14.5 9 16.75" />
</>)

/** A brick wall. */
export const FirewallGlyph = glyph(<>
  <rect x="2.5" y="3.5" width="19" height="17" rx="1.5" />
  <path d="M2.5 9.17h19M2.5 14.83h19M8.83 3.5v5.67M15.17 3.5v5.67M12 9.17v5.66M8.83 14.83v5.67M15.17 14.83v5.67" />
</>)

/** Radio waves above a ceiling unit. */
export const AccessPointGlyph = glyph(<>
  <path d="M5 7.5a10 10 0 0 1 14 0" /><path d="M8.25 10.75a5.4 5.4 0 0 1 7.5 0" />
  <circle cx="12" cy="13.75" r="1" fill="currentColor" /><path d="M12 14.75V17" /><rect x="5.5" y="17" width="13" height="4" rx="1.5" />
</>)

/** A rack unit that manages radios. */
export const WirelessControllerGlyph = glyph(<>
  <path d="M6.5 6.5a8 8 0 0 1 11 0" /><path d="M9.5 9.75a3.7 3.7 0 0 1 5 0" />
  <rect x="2" y="13" width="20" height="7.5" rx="1.75" /><path d="M6 16.75h.01M9.5 16.75h.01M14 16.75h4" />
</>)

/** One flow shared across three. */
export const LoadBalancerGlyph = glyph(<>
  <circle cx="4.5" cy="12" r="2.5" />
  <path d="M7 12h14M18.75 9.75L21 12l-2.25 2.25" />
  <path d="M6.5 10.5L11 5.5h10M18.75 3.25L21 5.5l-2.25 2.25" /><path d="M6.5 13.5l4.5 5h10M18.75 16.25L21 18.5l-2.25 2.25" />
</>)

/** A one-unit box with ports. */
export const ApplianceGlyph = glyph(<>
  <rect x="1.75" y="7.5" width="20.5" height="9" rx="1.75" />
  <path d="M5.5 12h.01M8.5 12h.01M11.5 12h.01M14.5 12h.01M18.5 10.5v3" />
</>)

/** A server drawn in dashes: it exists only in software. */
export const VirtualMachineGlyph = glyph(<>
  <rect x="2.5" y="2.5" width="19" height="19" rx="2.5" strokeDasharray="3 3.1" />
  <rect x="7" y="7" width="10" height="4" rx="1" /><rect x="7" y="13" width="10" height="4" rx="1" /><path d="M9.5 9h.01M9.5 15h.01" />
</>)
