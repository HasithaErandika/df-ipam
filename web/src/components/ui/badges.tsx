import { CircleDot } from 'lucide-react'
import { formatAgo } from '../../lib/format'
import { deviceStatuses, deviceTone, healthTone, label, registerStatuses, registerTone, severities, severityTone, type Tone } from '../../lib/labels'
import { cx } from './cx'
import { Badge } from './layout'

const dot: Record<Tone, string> = { neutral: 'bg-ink-3', accent: 'bg-accent', ok: 'bg-ok', warn: 'bg-[var(--meter-warning)]', danger: 'bg-danger' }

/** A badge with a coloured dot, for states that change, such as health. */
export function StateBadge({ tone, children, title }: { tone: Tone; children: React.ReactNode; title?: string }) {
  return (
    <span title={title} className="inline-flex min-w-0 items-center gap-1.5 text-[12.5px] text-ink">
      <span aria-hidden className={cx('size-2 shrink-0 rounded-full', dot[tone])} />{children}
    </span>
  )
}

export function SeverityBadge({ severity }: { severity: string }) {
  return <Badge tone={severityTone[severity] ?? 'neutral'}>{label(severities, severity)}</Badge>
}

export function HealthBadge({ health, since }: { health: string; since?: string | null }) {
  const text = health === 'up' ? 'Up' : health === 'down' ? 'Down' : 'Unknown'
  return <StateBadge tone={healthTone[health] ?? 'neutral'} title={since ? `Since ${new Date(since).toLocaleString('en-GB')}` : undefined}>
    {text}{since ? <span className="text-ink-3"> · {formatAgo(since)}</span> : null}
  </StateBadge>
}

/** The register side of an address. Never mixed with what the scanner saw. */
/** attention marks an address that should be in the register but isn't. */
export function RegisterBadge({ status, attention }: { status: string | null | undefined; attention?: boolean }) {
  if (!status) return <Badge tone={attention ? 'warn' : 'neutral'}>Not registered</Badge>
  return <Badge tone={registerTone[status]}>{label(registerStatuses, status)}</Badge>
}

/** The evidence side of an address: whether the scanner saw it, and when. */
export function SeenBadge({ lastSeen }: { lastSeen: string | null | undefined }) {
  if (!lastSeen) return <StateBadge tone="neutral">Not seen</StateBadge>
  const recent = Date.now() - new Date(lastSeen).getTime() < 15 * 60_000
  return <StateBadge tone={recent ? 'ok' : 'neutral'} title={new Date(lastSeen).toLocaleString('en-GB')}>Seen {formatAgo(lastSeen)}</StateBadge>
}

export function DeviceBadge({ status }: { status: string | null | undefined }) {
  if (!status) return <Badge tone="warn">Unknown MAC</Badge>
  return <Badge tone={deviceTone[status]}>{label(deviceStatuses, status)}</Badge>
}

export function EnvironmentBadge({ name, colour }: { name: string | null; colour?: string | null }) {
  if (!name) return <span className="text-ink-3">—</span>
  return (
    <span className="inline-flex items-center gap-1.5 rounded-[3px] border border-line bg-surface px-1.5 text-[11.5px] leading-[18px] font-medium whitespace-nowrap text-ink-2">
      <span aria-hidden className="size-2 rounded-[2px]" style={{ backgroundColor: colour ?? '#64748b' }} />{name}
    </span>
  )
}

/** Marks the router of a network: the segment's gateway or the scanner's default route. */
export function GatewayBadge() {
  return <Badge tone="neutral"><span className="inline-flex items-center gap-1 text-ink"><span aria-hidden className="size-1.5 rounded-full bg-[var(--addr-gateway)]" />Gateway</span></Badge>
}

export function PrivateMacBadge() {
  return <span title="A randomised (private) MAC address, as phones and laptops use on Wi-Fi. It is not raised as an unknown device." className="inline-flex items-center gap-1 text-[11.5px] text-ink-3"><CircleDot size={11} aria-hidden />Private MAC</span>
}
