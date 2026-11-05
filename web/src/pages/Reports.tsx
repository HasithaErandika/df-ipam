import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, FileBarChart } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { download, get, list, listAll, post } from '../lib/api'
import { environments, formatCount, formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { ReportJob, ReportSummary, ReportType, RoutingDomain } from '../lib/types'

const formatNames = { xlsx: 'Excel', csv: 'CSV', pdf: 'PDF' }
const statusTone = { queued: 'neutral', running: 'accent', completed: 'ok', failed: 'danger' } as const

function size(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function Figure({ label, value, alert }: { label: string; value: number; alert?: boolean }) {
  return (
    <div className="min-w-0 px-4 py-3">
      <div className="text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">{label}</div>
      <div className={`mt-1 text-[20px] leading-none font-semibold tracking-tight tabular-nums ${alert && value > 0 ? 'text-danger' : 'text-ink'}`}>{formatCount(value)}</div>
    </div>
  )
}

function Summary() {
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['report-jobs', 'summary'], queryFn: () => get<ReportSummary>('/reports/summary'),
    refetchInterval: (q) => ((q.state.data?.inProgress ?? 0) > 0 ? 4000 : false),
  })
  if (isPending) return <Card className="mb-3"><SkeletonRows rows={2} /></Card>
  if (error) return <Card className="mb-3"><ErrorState error={error} onRetry={refetch} /></Card>
  const v = data.inventory
  return (
    <Card title="Summary" className="mb-3" action={<span className="text-xs text-ink-3">{data.lastReadyAt ? `Last report ${formatDateTime(data.lastReadyAt)}` : 'No reports finished yet'}</span>}>
      <div className="grid grid-cols-2 divide-x divide-y divide-line md:grid-cols-4 md:divide-y-0">
        <Figure label="Your reports" value={data.total} />
        <Figure label="Ready to download" value={data.ready} />
        <Figure label="In progress" value={data.inProgress} />
        <Figure label="Failed" value={data.failed} alert />
      </div>
      {v && (
        <div className="grid grid-cols-2 divide-x divide-y divide-line border-t border-line md:grid-cols-3 xl:grid-cols-6 xl:divide-y-0">
          <Figure label="Subnets" value={v.subnets} />
          <Figure label="Tracked IPs" value={v.ipRecords} />
          <Figure label="Allocated" value={v.allocated} />
          <Figure label="Reserved" value={v.reserved} />
          <Figure label="Devices" value={v.devices} />
          <Figure label="Open findings" value={v.openFindings} alert />
        </div>
      )}
    </Card>
  )
}

function Generate({ report, onClose }: { report: ReportType | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [format, setFormat] = useState<keyof typeof formatNames>('xlsx')
  const [filters, setFilters] = useState<Record<string, string>>({})
  const uses = (f: string) => report?.filters.includes(f) ?? false
  const domains = useQuery({ queryKey: ['routing-domains', 'all'], enabled: uses('routingDomainId'), queryFn: () => listAll<RoutingDomain>('/routing-domains') })
  const create = useMutation({
    mutationFn: () => post('/reports', { reportType: report?.key, format, filters }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['report-jobs'] }); setFilters({}); onClose() },
  })
  if (!report) return null
  const set = (key: string, value: string) => setFilters((f) => ({ ...f, [key]: value }))
  return (
    <Modal open onClose={onClose} title={report.title} subtitle={report.description}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => create.mutate()}>Generate</Button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Format" htmlFor="report-format">
          <Select id="report-format" value={format} onChange={(e) => setFormat(e.target.value as keyof typeof formatNames)}>
            {Object.entries(formatNames).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        {uses('routingDomainId') && (
          <Field label="Routing domain" htmlFor="report-domain">
            <Select id="report-domain" value={filters.routingDomainId ?? ''} onChange={(e) => set('routingDomainId', e.target.value)}>
              <option value="">All</option>
              {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
            </Select>
          </Field>
        )}
        {uses('environment') && (
          <Field label="Environment" htmlFor="report-env">
            <Select id="report-env" value={filters.environment ?? ''} onChange={(e) => set('environment', e.target.value)}>
              <option value="">All</option>
              {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </Select>
          </Field>
        )}
        {uses('from') && <Field label="From" htmlFor="report-from"><Input id="report-from" type="date" value={filters.from ?? ''} onChange={(e) => set('from', e.target.value)} /></Field>}
        {uses('to') && <Field label="To" htmlFor="report-to"><Input id="report-to" type="date" value={filters.to ?? ''} onChange={(e) => set('to', e.target.value)} /></Field>}
      </div>
      {format === 'pdf' && <p className="mt-3 text-[13px] text-ink-2">PDF reports stop at 5,000 rows. Use Excel or CSV for full registers.</p>}
      <div className="mt-3"><FormError error={create.error} /></div>
      <p className="mt-3 text-[12px] text-ink-3">Reports describe the network, so generating and downloading one is recorded in the audit log.</p>
    </Modal>
  )
}

function Jobs() {
  const paging = usePaging()
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['report-jobs', paging.page, paging.pageSize], placeholderData: keepPreviousData,
    queryFn: () => list<ReportJob>(`/reports?page=${paging.page}&pageSize=${paging.pageSize}`),
    refetchInterval: (q) => (q.state.data?.data.some((j) => j.status === 'queued' || j.status === 'running') ? 4000 : false),
  })
  useClampPage(paging, data?.meta)
  const save = useMutation({ mutationFn: (j: ReportJob) => download(`/reports/${j.id}/download`, `${j.reportType}.${j.format}`) })
  return (
    <Card title="Your reports">
      {save.error ? <div className="p-3"><FormError error={save.error} /></div> : null}
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No reports yet" body="Choose a report above to generate one." />}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Report</Th><Th>Format</Th><Th>Requested</Th><Th>Status</Th><Th className="text-right">Rows</Th><Th className="text-right">Size</Th><Th /></tr></thead>
              <tbody>
                {data.data.map((j) => (
                  <tr key={j.id} className="hover:bg-surface-2">
                    <Td className="font-medium text-ink">{j.title || j.reportType}{j.error && <div className="text-xs font-normal text-danger">{j.error}</div>}{j.truncated && <div className="text-xs font-normal text-warn">Stopped at the row limit, so this isn't the whole register.</div>}</Td>
                    <Td className="text-ink-2">{formatNames[j.format]}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(j.createdAt)}</Td>
                    <Td><Badge tone={statusTone[j.status]}>{j.status}</Badge></Td>
                    <Td className="text-right text-ink-2 tabular-nums">{j.status === 'completed' ? formatCount(j.rowCount) : '—'}</Td>
                    <Td className="text-right text-ink-2 tabular-nums">{j.status === 'completed' ? size(j.sizeBytes) : '—'}</Td>
                    <Td className="text-right">{j.status === 'completed' && <Button busy={save.isPending && save.variables?.id === j.id} onClick={() => save.mutate(j)}><Download size={13} aria-hidden /> Download</Button>}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}

export function Reports() {
  const canExport = useCan('reports:export')
  const canScoped = useCan('reports:export_scoped')
  const allowed = canExport || canScoped
  const [selected, setSelected] = useState<ReportType | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['report-types'], enabled: allowed, queryFn: () => get<ReportType[]>('/reports/types') })
  if (!allowed) return <><PageHeader title="Reports" /><Card><EmptyState title="You don't have access to reports" /></Card></>
  const groups = [...new Set(data?.map((r) => r.group))]
  return (
    <>
      <PageHeader title="Reports" description="Registers and evidence in Excel, CSV and PDF. Reports run in the background, then appear below." />
      <Summary />
      {isPending && <Card><SkeletonRows rows={6} /></Card>}
      {error && <Card><ErrorState error={error} onRetry={refetch} /></Card>}
      {!canExport && data && <p className="mb-3 text-[13px] text-ink-2">Your reports cover only the business services assigned to you.</p>}
      <div className="mb-3 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {groups.map((g) => (
          <Card key={g} title={g}>
            <ul className="divide-y divide-line">
              {data?.filter((r) => r.group === g).map((r) => (
                <li key={r.key}>
                  <button type="button" onClick={() => setSelected(r)} className="flex w-full cursor-pointer items-start gap-2 px-4 py-2 text-left hover:bg-surface-2">
                    <FileBarChart size={15} className="mt-0.5 shrink-0 text-ink-3" aria-hidden />
                    <span><span className="block text-[13px] font-medium text-ink">{r.title}</span><span className="block text-xs text-ink-2">{r.description}</span></span>
                  </button>
                </li>
              ))}
            </ul>
          </Card>
        ))}
      </div>
      <Jobs />
      <Generate report={selected} onClose={() => setSelected(null)} />
    </>
  )
}
