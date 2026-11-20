import { Navigate, useParams } from 'react-router'
import { PageHeader } from '../../components/ui'
import { useRouteTab } from '../../lib/tabs'
import { FirewallImport } from './FirewallImport'
import { LiveView } from './LiveView'

/**
 * The connected FortiGate, its own section: what it reports now (Live), and
 * bringing what it knows into DF-IPAM after review (Import). DF-IPAM only ever
 * reads the firewall.
 */
export function FirewallPage() {
  const tab = useRouteTab(['live', 'import'])
  const { tab: raw } = useParams()
  if (!raw) return <Navigate to="/firewall/live" replace />
  if (raw === 'review') return <Navigate to="/firewall/import" replace />
  if (tab === 'live') {
    return <>
      <PageHeader title="Firewall · Live" description="What the FortiGate reports now, read every 10 seconds and never stored. DF-IPAM never changes the firewall." />
      <LiveView />
    </>
  }
  return <>
    <PageHeader title="Firewall · Import" description="Bring networks, devices, servers and network links from the FortiGate into DF-IPAM. Nothing is saved until you confirm." />
    <FirewallImport />
  </>
}
