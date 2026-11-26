import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Badge, Button, Card, DataTable, FilterBar, FilterSelect, Input, none, PageHeader, PathTabs, SearchInput, SeverityBadge, Stack, StateBadge, type Column } from '../../components/ui'
import { get, post } from '../../lib/api'
import { formatAgo, formatDateTime } from '../../lib/format'
import { alertStatuses, alertTypes, label, options, severities } from '../../lib/labels'
import { useFilters, usePagedList } from '../../lib/listing'
import { useInvalidate, useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { Alert } from '../../lib/types'
import { EmailPreferences } from './EmailPreferences'

export function AlertsPage() {
  const tab = useRouteTab(['alerts', 'history'])
  const history = tab === 'history'
  const canManage = useCan('alerts:manage')
  const segments = useSegments()
  const f = useFilters(['severity', 'type', 'segmentId', 'q', 'from', 'to', 'status'] as const)
  const status = history ? f.values.status || 'all' : 'active'
  const { query, rows, meta, paging } = usePagedList<Alert>('/alerts', { ...f.values, status }, { refetchInterval: history ? undefined : 60_000 })
  const summary = useQuery({ queryKey: ['/alerts/summary'], queryFn: () => get<Record<string, number>>('/alerts/summary'), refetchInterval: 60_000 })
  const refresh = useInvalidate('/alerts', '/dashboard')
  const ack = useMutation({ mutationFn: (id: string) => post(`/alerts/${id}/acknowledge`), meta: { success: 'Alert acknowledged.' }, onSuccess: () => void refresh() })
  const active = summary.data ? ['critical', 'high', 'medium', 'low'].reduce((n, k) => n + (summary.data[k] ?? 0), 0) : undefined

  const columns: Column<Alert>[] = [
    { key: 'severity', header: 'Severity', cell: (a) => <SeverityBadge severity={a.severity} /> },
    { key: 'alert', header: 'Alert', cell: (a) => <Stack main={<span className="font-medium">{label(alertTypes, a.type)}</span>} sub={<span className="whitespace-normal">{a.message}</span>} /> },
    { key: 'where', header: 'Where', cell: (a) => (
      <div className="min-w-0">
        <div className="font-mono text-[12.5px]">{a.ip ?? a.mac ?? '—'}</div>
        {a.segmentId ? <Link className="text-[12px] text-ink-3 hover:text-accent hover:underline" to={`/segments/${a.segmentId}`}>{a.segmentName}</Link> : null}
      </div>
    ) },
    { key: 'status', header: 'Status', hideBelow: 'md', cell: (a) => (
      <Stack main={<StateBadge tone={a.status === 'open' ? 'danger' : a.status === 'acknowledged' ? 'warn' : 'neutral'}>{label(alertStatuses, a.status)}</StateBadge>}
        sub={a.status === 'acknowledged' && a.acknowledgedBy ? `by ${a.acknowledgedBy}` : a.closedAt ? `closed ${formatAgo(a.closedAt)}` : undefined} />
    ) },
    { key: 'when', header: 'Opened', hideBelow: 'lg', cell: (a) => <Stack main={<span title={formatDateTime(a.openedAt)}>{formatAgo(a.openedAt)}</span>} sub={a.status !== 'closed' ? `still seen ${formatAgo(a.lastSeenAt)}` : undefined} /> },
    { key: 'act', header: '', className: 'text-right', cell: (a) => (canManage && a.status === 'open'
      ? <Button busy={ack.isPending && ack.variables === a.id} onClick={() => ack.mutate(a.id)}>Acknowledge</Button>
      : none) },
  ]

  return (
    <>
      <PageHeader title="Alerts" description="DF-Atlas only reports: it never blocks a device or changes DHCP. Alerts close by themselves when the cause is gone."
        actions={<><EmailPreferences /><Link to="/reports" className="text-[13px] font-medium text-accent hover:underline">Alert history report</Link></>} />
      {summary.data && (
        <div className="mb-3 flex flex-wrap gap-2">
          {(['critical', 'high', 'medium', 'low'] as const).map((s) => (
            <button key={s} type="button" onClick={() => f.set('severity', f.values.severity === s ? '' : s)}
              className={`inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-1.5 text-[12.5px] ${f.values.severity === s ? 'border-accent bg-accent-soft' : 'border-line bg-surface hover:border-line-strong'}`}>
              <SeverityBadge severity={s} /><span className="font-semibold text-ink tabular-nums">{summary.data[s] ?? 0}</span>
            </button>
          ))}
        </div>
      )}
      <PathTabs base="/alerts" tabs={[{ key: 'alerts', label: 'Active', count: active }, { key: 'history', label: 'History' }]} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search message, IP, MAC" />
          <FilterSelect label="Severity" value={f.values.severity} onChange={(v) => f.set('severity', v)} options={options(severities)} />
          <FilterSelect label="Type" value={f.values.type} onChange={(v) => f.set('type', v)} anyLabel="Any type" options={options(alertTypes)} />
          <FilterSelect label="Segment" value={f.values.segmentId} onChange={(v) => f.set('segmentId', v)} options={(segments.data ?? []).map((s) => ({ value: s.id, text: s.name }))} />
          {history && <>
            <FilterSelect label="Status" value={f.values.status} onChange={(v) => f.set('status', v)} anyLabel="Any status" options={options(alertStatuses)} />
            <label className="flex items-center gap-1.5 text-[12.5px] text-ink-3">From<Input type="date" className="h-8 w-36" value={f.values.from} onChange={(e) => f.set('from', e.target.value)} /></label>
            <label className="flex items-center gap-1.5 text-[12.5px] text-ink-3">To<Input type="date" className="h-8 w-36" value={f.values.to} min={f.values.from || undefined} onChange={(e) => f.set('to', e.target.value)} /></label>
          </>}
        </FilterBar>
        <DataTable columns={columns} rows={rows} query={query} rowKey={(a) => a.id} meta={meta} paging={paging} filtered={f}
          rowClassName={(a) => (a.status === 'open' && a.severity === 'critical' ? 'bg-danger-soft/40' : undefined)}
          empty={history ? { title: 'No alerts in this period' } : { title: 'All clear', body: 'No open alerts. New ones appear here after each scan or server check.' }} />
      </Card>
      {!canManage && <p className="mt-2 text-[12px] text-ink-3"><Badge>Read only</Badge> Admins acknowledge alerts.</p>}
    </>
  )
}
