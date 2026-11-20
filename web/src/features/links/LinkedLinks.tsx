import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Card, StateBadge } from '../../components/ui'
import { list, queryString } from '../../lib/api'
import { useCan } from '../../lib/session'
import type { NetworkLink } from '../../lib/types'
import { linkStatuses, linkStatusTone } from './labels'

/**
 * The network links that use a subnet or an address, for the segment page and
 * register entry details. Shows nothing to people who may not see links.
 */
export function LinkedLinks({ segmentId, ip, title, card }: { segmentId?: string; ip?: string; title?: (n: number) => React.ReactNode; card?: (n: number) => string }) {
  const allowed = useCan('links:view')
  const qs = queryString({ segmentId, ip, pageSize: 20 })
  const q = useQuery({ queryKey: ['/network-links', 'linked', qs], queryFn: () => list<NetworkLink>(`/network-links${qs}`), enabled: allowed })
  if (!allowed || !q.data || q.data.data.length === 0) return null
  const rows = q.data.data
  const items = (
      <ul className="space-y-1 text-[13px]">
        {rows.map((l) => (
          <li key={l.id} className="flex flex-wrap items-center gap-2">
            <Link to={`/network-links/${l.id}`} className="text-accent hover:underline"><span className="font-mono">{l.code}</span> {l.name}</Link>
            <StateBadge tone={linkStatusTone[l.status]}>{linkStatuses[l.status]}</StateBadge>
          </li>
        ))}
      </ul>
  )
  if (card) return <Card className="mt-4" title={card(rows.length)}><div className="p-4">{items}</div></Card>
  return <div>{title?.(rows.length)}{items}</div>
}
