import { type ReactNode } from 'react'
import { Mono } from '../../components/ui'
import { formatCapacity } from '../../lib/format'
import type { NetInfo } from '../../lib/types'

export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{children}</dd>
    </div>
  )
}

export function Calculated({ info }: { info: NetInfo }) {
  return (
    <dl>
      <Detail label="Network"><Mono>{info.network}</Mono></Detail>
      {info.mask && <Detail label="Subnet mask"><Mono>{info.mask}</Mono></Detail>}
      {info.broadcast && <Detail label="Broadcast"><Mono>{info.broadcast}</Mono></Detail>}
      <Detail label="First usable"><Mono>{info.firstUsable}</Mono></Detail>
      <Detail label="Last usable"><Mono>{info.lastUsable}</Mono></Detail>
      <Detail label="Total addresses"><span className="tabular-nums">{formatCapacity(info.totalAddresses, info.family, info.prefixLength)}</span></Detail>
      <Detail label="Usable addresses"><span className="tabular-nums">{formatCapacity(info.usableAddresses, info.family, info.prefixLength)}</span></Detail>
    </dl>
  )
}
