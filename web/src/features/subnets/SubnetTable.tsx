import type { Column } from './columns'
import { Badge, Meter, Mono, Td, Th } from '../../components/ui'
import { environments, formatCapacity } from '../../lib/format'
import type { Subnet } from '../../lib/types'

interface Props {
  rows: Subnet[]
  columns: Set<Column>
  selectable: boolean
  picked: Set<string>
  onPick: (picked: Set<string>) => void
  onOpen: (subnet: Subnet) => void
}

/** Ticks or unticks a whole page. Archived subnets can't be selected. */
function pickPage(rows: Subnet[], picked: Set<string>, on: boolean) {
  if (on) return new Set([...picked, ...rows.filter((s) => !s.archived).map((s) => s.id)])
  return new Set([...picked].filter((id) => !rows.some((s) => s.id === id)))
}

function pickOne(picked: Set<string>, id: string, on: boolean) {
  const next = new Set(picked)
  if (on) next.add(id)
  else next.delete(id)
  return next
}

function SubnetRow({ s, columns, selectable, picked, onPick, onOpen }: Omit<Props, 'rows'> & { s: Subnet }) {
  const has = (c: Column) => columns.has(c)
  return (
    <tr className="hover:bg-surface-2">
      {selectable && <Td><input type="checkbox" aria-label={`Select ${s.name}`} disabled={s.archived} checked={picked.has(s.id)}
        onChange={(e) => onPick(pickOne(picked, s.id, e.target.checked))} /></Td>}
      <Td><button type="button" className="cursor-pointer text-left font-medium text-ink hover:text-accent" onClick={() => onOpen(s)}>{s.name}</button></Td>
      <Td><Mono>{s.calculated.cidr}</Mono></Td>
      {has('routingDomain') && <Td className="text-ink-2">{s.routingDomainName}</Td>}
      {has('environment') && <Td><Badge tone={s.environment === 'production' ? 'accent' : 'neutral'}>{environments[s.environment]}</Badge></Td>}
      {has('site') && <Td className="text-ink-2">{s.site?.name ?? '—'}</Td>}
      {has('vlan') && <Td className="text-ink-2">{s.vlan?.name ?? '—'}</Td>}
      {has('gateway') && <Td className="text-ink-2">{s.gateway ? <Mono>{s.gateway}</Mono> : '—'}</Td>}
      {has('usable') && <Td className="text-right text-ink-2 tabular-nums">{formatCapacity(s.calculated.usableAddresses, s.calculated.family, s.calculated.prefixLength)}</Td>}
      {has('utilisation') && <Td><Meter pct={Number(s.utilisationPct)} level={s.utilisationLevel} label={`${s.name} utilisation`} /></Td>}
    </tr>
  )
}

export function SubnetTable({ rows, columns, selectable, picked, onPick, onOpen }: Props) {
  const has = (c: Column) => columns.has(c)
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse">
        <thead>
          <tr>
            {selectable && <Th className="w-8"><input type="checkbox" aria-label="Select all on this page"
              checked={rows.length > 0 && rows.every((s) => picked.has(s.id))}
              onChange={(e) => onPick(pickPage(rows, picked, e.target.checked))} /></Th>}
            <Th>Name</Th><Th>CIDR</Th>
            {has('routingDomain') && <Th>Routing domain</Th>}
            {has('environment') && <Th>Environment</Th>}
            {has('site') && <Th>Site</Th>}
            {has('vlan') && <Th>VLAN</Th>}
            {has('gateway') && <Th>Gateway</Th>}
            {has('usable') && <Th className="text-right">Usable</Th>}
            {has('utilisation') && <Th>Utilisation</Th>}
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => <SubnetRow key={s.id} s={s} columns={columns} selectable={selectable} picked={picked} onPick={onPick} onOpen={onOpen} />)}
        </tbody>
      </table>
    </div>
  )
}
