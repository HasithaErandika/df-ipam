import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import { Card, DataTable, ErrorState, SkeletonRows, StateBadge, type Column } from '../../components/ui'
import { formatAgo, formatDateTime, formatIn } from '../../lib/format'
import { useScanStatus } from '../../lib/scan'
import type { ScanStatus } from '../../lib/types'
import { ScanButton } from '../shared/ScanButton'
import { DiscoveryMethods } from './DiscoveryMethods'


const method: Record<string, string> = { ping: 'Ping sweep with reverse DNS and NetBIOS', arp: 'ARP scan per VLAN', snmp: 'Switch SNMP tables' }

/** One line about the scanner, with Scan now for Admins. */
export function ScanSummary({ compact }: { compact?: boolean }) {
  const q = useScanStatus()
  if (!q.data) return compact ? null : <SkeletonRows rows={1} />
  const s = q.data
  return (
    <div className="flex flex-wrap items-center gap-3 text-[12.5px] text-ink-2">
      {s.running
        ? <StateBadge tone="accent">Scanning now{s.startedAt ? ` · started ${formatAgo(s.startedAt)}` : ''}</StateBadge>
        : <StateBadge tone={s.finishedAt ? 'ok' : 'neutral'}>{s.finishedAt ? `Last scan ${formatAgo(s.finishedAt)}` : 'No scan since the server started'}</StateBadge>}
      {!s.running && s.nextScanAt && <span className="text-ink-3">Next {formatIn(s.nextScanAt)}</span>}
      <ScanButton running={s.running} />
    </div>
  )
}

type Row = ScanStatus['segments'][number]

const columns: Column<Row>[] = [
  { key: 'name', header: 'Segment', cell: (r) => <span className="font-medium text-ink">{r.name}</span> },
  { key: 'result', header: 'Result', cell: (r) => r.error
    ? <span className="flex items-start gap-1.5 text-danger"><AlertTriangle size={14} className="mt-0.5 shrink-0" aria-hidden />{r.error}</span>
    : <span className="flex items-center gap-1.5 text-ok"><CheckCircle2 size={14} aria-hidden />Completed</span> },
  { key: 'found', header: 'Answering', className: 'text-right tabular-nums', cell: (r) => r.found },
  { key: 'time', header: 'Took', className: 'text-right tabular-nums', hideBelow: 'md', cell: (r) => `${(r.durationMs / 1000).toFixed(1)} s` },
  { key: 'when', header: 'Scanned', hideBelow: 'md', cell: (r) => <span title={formatDateTime(r.scannedAt)}>{formatAgo(r.scannedAt)}</span> },
  { key: 'scan', header: '', className: 'text-right', cell: (r) => <ScanButton segmentId={r.id} label="Rescan" /> },
]

/** How each segment's last sweep went. Kept in memory, so it restarts with the server. */
export function ScanStatusPanel() {
  const q = useScanStatus()
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  const s = q.data
  return (
    <div className="space-y-4">
      <Card title="Scanner" subtitle={s ? `${method[s.method] ?? s.method} · every ${s.intervalMinutes} minutes · read-only, nothing on the network is changed` : undefined}>
        <div className="px-4 py-3"><ScanSummary /></div>
      </Card>
      <Card title="Last result per segment" subtitle="Cloud segments are not swept from the building. Segments larger than 4,096 addresses are skipped; split them to scan.">
        <DataTable columns={columns} rows={s?.segments ?? []} query={q} rowKey={(r) => r.id}
          empty={{ title: 'No segments scanned yet', body: 'Results appear here as each segment finishes. Add segments in Settings first.' }} />
      </Card>
      <DiscoveryMethods />
    </div>
  )
}
