import { useQuery } from '@tanstack/react-query'
import { Archive, ArchiveRestore, ArrowLeft, Pencil } from 'lucide-react'
import { Link, useMatch, useNavigate, useParams } from 'react-router'
import { Button, Card, EmptyState, ErrorState, Notice, PageHeader, SkeletonRows, StateBadge } from '../../components/ui'
import { ApiError, get } from '../../lib/api'
import { useCan } from '../../lib/session'
import type { LinkDetail } from '../../lib/types'
import { AttachmentsCard } from './AttachmentsCard'
import { HistoryCard } from './HistoryCard'
import { LinkFormModal } from './LinkFormModal'
import { LiveLinkCard } from '../firewall/LiveLinkCard'
import { isVpnType, linkStatuses, linkStatusTone, typeName } from './labels'
import { AddressesCard, CommercialCard, GeneralCard, RelatedCard, SupportCard, TechnicalCard, VpnCard } from './LinkDetailSections'
import { useLinkSettings, useProviders } from './queries'
import { useArchive } from './useArchive'

/** Everything about one link, with clickable references to the register. */
export function LinkDetailPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const editing = Boolean(useMatch('/network-links/:id/edit'))
  const q = useQuery({ queryKey: [`/network-links/${id}`], queryFn: () => get<LinkDetail>(`/network-links/${id}`) })
  const settings = useLinkSettings()
  const providers = useProviders()
  const can = { edit: useCan('links:edit'), archive: useCan('links:archive'), history: useCan('audit:view') }
  const archive = useArchive()
  const back = <Link to="/network-links/all" className="mb-2 inline-flex items-center gap-1 text-[12.5px] text-ink-3 hover:text-accent"><ArrowLeft size={13} aria-hidden />All network links</Link>

  if (q.isPending) return <>{back}<Card><SkeletonRows rows={10} /></Card></>
  if (q.isError) {
    return <>{back}<Card>{q.error instanceof ApiError && q.error.status === 404
      ? <EmptyState title="This link doesn't exist" body="Check the address, or find it in All network links." />
      : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Card></>
  }
  const d = q.data
  const l = d.link
  const vpn = isVpnType(settings.data, l.linkType) || Boolean(l.vpnTunnel || l.remotePeer || l.remoteSubnets.length)
  return (
    <>
      {back}
      <PageHeader title={`${l.code} · ${l.name}`} description={`${typeName(settings.data, l.linkType)}${l.providerName ? ` from ${l.providerName}` : ''}, ${l.sourceSite}${l.destination ? ` to ${l.destination}` : ''}.`}
        actions={<>
          <StateBadge tone={linkStatusTone[l.status]} title="Administrative status, not live availability">{linkStatuses[l.status]}</StateBadge>
          {can.archive && <Button onClick={() => void archive(l)}>{l.archivedAt ? <><ArchiveRestore size={14} aria-hidden />Restore</> : <><Archive size={14} aria-hidden />Archive</>}</Button>}
          {can.edit && !l.archivedAt && <Button variant="primary" onClick={() => navigate(`/network-links/${l.id}/edit`)}><Pencil size={14} aria-hidden />Edit</Button>}
        </>} />
      {l.archivedAt && <div className="mb-4"><Notice tone="warn">Archived. It is left out of lists, totals and contract reminders, and can't be edited until it is restored.</Notice></div>}
      <div className="grid gap-4 xl:grid-cols-2">
        <div className="xl:col-span-2 empty:hidden"><LiveLinkCard link={l} /></div>
        <GeneralCard d={d} settings={settings.data} />
        <TechnicalCard d={d} />
        {vpn && <VpnCard d={d} />}
        <CommercialCard d={d} />
        <SupportCard d={d} provider={providers.data?.find((p) => p.id === l.providerId)} />
        <AddressesCard d={d} />
        <RelatedCard d={d} settings={settings.data} />
        {l.remarks && <Card title="Remarks and technical notes"><p className="p-4 text-[13px] whitespace-pre-wrap text-ink">{l.remarks}</p></Card>}
        <div className="xl:col-span-2"><AttachmentsCard d={d} /></div>
        {can.history && <div className="xl:col-span-2"><HistoryCard id={l.id} /></div>}
      </div>
      <LinkFormModal open={editing} detail={d} onClose={() => navigate(`/network-links/${l.id}`)} onSaved={() => navigate(`/network-links/${l.id}`)} />
    </>
  )
}
