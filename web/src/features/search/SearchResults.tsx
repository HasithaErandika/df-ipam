import { useQuery } from '@tanstack/react-query'
import { Boxes, Network, Server, TableProperties, type LucideIcon } from 'lucide-react'
import { Link, useSearchParams } from 'react-router'
import { Card, EmptyState, ErrorState, Mono, PageHeader, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'

interface Hit { kind: string; id: string; title: string; detail: string; link: string }
interface Results { query: string; subnets: Hit[]; ipAddresses: Hit[]; devices: Hit[]; vlans: Hit[] }

const groups: { key: keyof Omit<Results, 'query'>; label: string; icon: LucideIcon; mono: boolean }[] = [
  { key: 'subnets', label: 'Subnets', icon: TableProperties, mono: true },
  { key: 'ipAddresses', label: 'IP addresses', icon: Boxes, mono: true },
  { key: 'devices', label: 'Devices', icon: Server, mono: false },
  { key: 'vlans', label: 'VLANs', icon: Network, mono: false },
]

export function SearchResults() {
  const [params] = useSearchParams()
  const q = params.get('q')?.trim() ?? ''
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['search', q], enabled: q.length >= 2, queryFn: () => get<Results>(`/search?q=${encodeURIComponent(q)}`),
  })
  const empty = data && groups.every((g) => data[g.key].length === 0)
  return (
    <>
      <PageHeader title="Search" description={q ? `Results for “${q}”. An address finds the subnets holding it in every routing domain.` : 'Search subnets, addresses, hostnames, MACs, devices and VLANs.'} />
      {q.length < 2 && <Card><EmptyState title="Type at least two characters" body="Use the search box at the top of any page." /></Card>}
      {q.length >= 2 && isPending && <Card><SkeletonRows rows={6} /></Card>}
      {error && <Card><ErrorState error={error} onRetry={refetch} /></Card>}
      {empty && <Card><EmptyState title="Nothing matches" body="Try a CIDR, an address, part of a hostname, a MAC or a VLAN number." /></Card>}
      {data && !empty && (
        <div className="grid gap-4 lg:grid-cols-2">
          {groups.filter((g) => data[g.key].length > 0).map((g) => (
            <Card key={g.key} title={`${g.label} (${data[g.key].length}${data[g.key].length === 8 ? '+' : ''})`}>
              <ul>
                {data[g.key].map((h) => (
                  <li key={h.id} className="border-b border-line last:border-0">
                    <Link to={h.link} className="flex items-center gap-3 px-4 py-2 hover:bg-surface-2">
                      <g.icon size={15} className="shrink-0 text-accent" aria-hidden />
                      <span className="min-w-0">
                        <span className="block truncate text-ink">{g.mono ? <Mono>{h.title}</Mono> : h.title}</span>
                        <span className="block truncate text-[12px] text-ink-3">{h.detail}</span>
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </Card>
          ))}
        </div>
      )}
    </>
  )
}
