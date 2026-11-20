import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { Card, DataTable, PageHeader, Segmented } from '../../components/ui'
import { renewalColumns } from './renewalColumns'
import { get } from '../../lib/api'
import type { LinkRenewal } from '../../lib/types'

const windows = [{ value: '30', text: '30 days' }, { value: '60', text: '60 days' }, { value: '90', text: '90 days' }, { value: '180', text: '6 months' }, { value: '365', text: '1 year' }]

/** Contracts ending soon, and those already ended on links still in use. */
export function RenewalsPage() {
  const [params, setParams] = useSearchParams()
  const days = params.get('days') ?? '90'
  const q = useQuery({ queryKey: ['/network-links', 'renewals', days], queryFn: () => get<LinkRenewal[]>(`/network-links/renewals?days=${days}`) })
  return (
    <>
      <PageHeader title="Contract renewals" description="Reminders go by email to each link's responsible officer and to people who manage link costs, on the reminder days set in Network setup. Terminated and archived links are left out." />
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2.5">
          <Segmented label="Ending within" value={days} onChange={(v) => setParams({ days: v }, { replace: true })} options={windows} />
        </div>
        <DataTable columns={renewalColumns} rows={q.data ?? []} query={q} rowKey={(r) => r.id}
          empty={{ title: 'No contracts end in this window', body: 'Links show here once their contract expiry date is set.' }} />
      </Card>
    </>
  )
}
