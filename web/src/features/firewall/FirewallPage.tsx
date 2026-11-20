import { Navigate, useParams, useSearchParams } from 'react-router'
import { EmptyState, Card, PageHeader, Segmented } from '../../components/ui'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import { InventoryReview, type InventoryKind } from './InventoryReview'
import { LiveView } from './LiveView'
import { ReviewView } from './ReviewView'

/**
 * The connected FortiGate, its own section: what it reports now, and
 * everything it knows (links, networks, devices, servers) brought into
 * DF-IPAM after review. DF-IPAM only ever reads the firewall.
 */
export function FirewallPage() {
  const tab = useRouteTab(['live', 'review'])
  const hasTab = Boolean(useParams().tab)
  const [params, setParams] = useSearchParams()
  const canAddLinks = useCan('links:create')
  const canEditLinks = useCan('links:edit')
  const can = { links: canAddLinks || canEditLinks, networks: useCan('network:manage'), inventory: useCan('inventory:edit') }
  const shows = [
    ...(can.networks ? [{ value: 'networks', text: 'Networks' }] : []),
    ...(can.inventory ? [{ value: 'devices', text: 'Devices' }, { value: 'servers', text: 'Servers' }] : []),
    ...(can.links ? [{ value: 'links', text: 'Network links' }] : []),
  ]
  const show = shows.some((s) => s.value === params.get('show')) ? params.get('show')! : shows[0]?.value
  if (!hasTab) return <Navigate to="/firewall/live" replace />
  if (tab === 'live') {
    return <>
      <PageHeader title="Firewall · Live" description="What the FortiGate reports now, read every 10 seconds and never stored. DF-IPAM never changes the firewall." />
      <LiveView />
    </>
  }
  return (
    <>
      <PageHeader title="Firewall · Review and import" description="Fetch what the FortiGate knows, see how it compares with DF-IPAM, then tick and confirm what to add. Nothing is saved until you confirm." />
      {shows.length === 0 ? <Card><EmptyState title="Nothing you may import" body="Importing needs permission to change the register, devices, the network setup or network links." /></Card> : (
        <>
          <div className="mb-4"><Segmented label="What to review" value={show} onChange={(v) => setParams({ show: v }, { replace: true })} options={shows} /></div>
          {show === 'links' ? <ReviewView /> : <InventoryReview kind={show as InventoryKind} />}
        </>
      )}
    </>
  )
}
