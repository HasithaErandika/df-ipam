import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Card, ErrorState, PageHeader, SkeletonRows } from '../../components/ui'
import { list } from '../../lib/api'
import { formatCount } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { IPRecord, Subnet } from '../../lib/types'

const link = 'block rounded-md border border-line p-3 transition-colors hover:border-accent'

function Scope() {
  const subnets = useQuery({ queryKey: ['subnets', 'scope-count'], queryFn: () => list<Subnet>('/subnets?pageSize=1') })
  const addresses = useQuery({ queryKey: ['ip', 'scope-count'], queryFn: () => list<IPRecord>('/ip-addresses?pageSize=1') })
  if (subnets.isPending || addresses.isPending) return <SkeletonRows rows={2} />
  if (subnets.error) return <ErrorState error={subnets.error} onRetry={subnets.refetch} />
  if (addresses.error) return <ErrorState error={addresses.error} onRetry={addresses.refetch} />
  const count = subnets.data.meta.total
  if (count === 0) return <p className="px-4 py-3 text-[13px] text-ink-2">No business services are assigned to you yet. Ask an administrator to assign them.</p>
  return <p className="px-4 py-3 text-[13px] text-ink-2">Your business services cover {formatCount(count)} subnets and {formatCount(addresses.data.meta.total)} tracked addresses.</p>
}

/** A role without the whole inventory, such as Business Viewer. */
export function LimitedDashboard() {
  const canReport = useCan('reports:export') || useCan('reports:export_scoped')
  const scoped = useCan('inventory:view_scoped')
  return (
    <>
      <PageHeader title="Dashboard" description="What your role gives you." />
      <Card title="Your access">
        {scoped && <Scope />}
        <div className="grid gap-2.5 border-t border-line p-3 sm:grid-cols-3">
          {scoped && <Link to="/ip-addresses" className={link}><span className="block font-medium text-ink">IP Address Explorer</span><span className="block text-[12.5px] text-ink-2">Subnets and addresses of your business services.</span></Link>}
          {canReport && <Link to="/reports" className={link}><span className="block font-medium text-ink">Reports</span><span className="block text-[12.5px] text-ink-2">Run and download the reports open to your role.</span></Link>}
          <Link to="/alerts" className={link}><span className="block font-medium text-ink">Alerts and Notifications</span><span className="block text-[12.5px] text-ink-2">Your alerts and how you receive them.</span></Link>
        </div>
      </Card>
    </>
  )
}
