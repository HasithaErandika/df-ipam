import { ArrowLeft } from 'lucide-react'
import { type ReactNode } from 'react'
import { Badge, Button, Card, Meter, Mono } from '../../components/ui'
import { environments } from '../../lib/format'
import type { Subnet } from '../../lib/types'

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="min-w-0"><dt className="text-[11.5px] text-ink-3">{label}</dt><dd className="text-[13px] break-words text-ink">{children}</dd></div>
}

/** The open subnet at a glance, above its addresses. */
export function SubnetSummary({ subnet, onBack }: { subnet: Subnet; onBack: () => void }) {
  const net = subnet.calculated
  return (
    <Card className="mb-3">
      <div className="flex flex-wrap items-start justify-between gap-3 p-3">
        <div className="min-w-0">
          <Button variant="ghost" className="-ml-2 h-7 px-2" onClick={onBack}><ArrowLeft size={14} aria-hidden /> All subnets</Button>
          <p className="mt-1 flex flex-wrap items-center gap-2 font-semibold break-all text-ink"><Mono>{net.cidr}</Mono><Badge>{environments[subnet.environment] ?? subnet.environment}</Badge></p>
          <p className="text-ink-2">{subnet.name}</p>
        </div>
        <div className="shrink-0">
          <Meter pct={Number(subnet.utilisationPct)} level={subnet.utilisationLevel} label={`${subnet.name} utilisation`} />
          <p className="mt-1 text-right text-[12px] text-ink-3 tabular-nums">{subnet.allocated} allocated · {subnet.reserved} reserved · {subnet.quarantined} quarantined</p>
        </div>
      </div>
      <dl className="grid grid-cols-2 gap-3 border-t border-line p-3 md:grid-cols-3 xl:grid-cols-[1fr_1fr_1fr_1fr_2fr_1fr]">
        <Fact label="Routing domain">{subnet.routingDomainName}</Fact>
        <Fact label="Site">{subnet.site?.name ?? '—'}</Fact>
        <Fact label="VLAN">{subnet.vlan?.name ?? '—'}</Fact>
        <Fact label="Gateway">{subnet.gateway ? <Mono>{subnet.gateway}</Mono> : '—'}</Fact>
        <Fact label="Usable range"><Mono>{net.firstUsable} – {net.lastUsable}</Mono></Fact>
        <Fact label="Usable addresses"><span className="tabular-nums">{net.usableAddresses}</span></Fact>
      </dl>
    </Card>
  )
}
